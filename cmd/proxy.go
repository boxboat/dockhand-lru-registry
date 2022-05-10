/*
Copyright © 2021 BoxBoat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/boxboat/dockhand-lru-registry/pkg/common"
	"github.com/boxboat/dockhand-lru-registry/pkg/lru"
	"github.com/boxboat/dockhand-lru-registry/pkg/proxy"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

type ProxyArgs struct {
	serverPort               int
	serverCert               string
	serverKey                string
	databaseDir              string
	registryHost             string
	registryScheme           string
	CleanupArgs              proxy.CleanSettings
	TargetDiskSizeByteString string
	UseForwardedHeaders      bool
}

var (
	proxyArgs ProxyArgs
)

func startProxy(ctx context.Context) {
	db, err := bolt.Open(fmt.Sprintf("%s/%s", proxyArgs.databaseDir, "usage.db"), 0600, nil)
	common.ExitIfError(err)
	defer db.Close()

	registryTarget, err := url.Parse(fmt.Sprintf("%s://%s", proxyArgs.registryScheme, proxyArgs.registryHost))
	common.ExitIfError(err)

	tlsSetting := config.TLSDisabled
	if proxyArgs.registryScheme == "https" {
		tlsSetting = config.TLSEnabled
	}

	registryProxy := &proxy.Proxy{
		Server: &http.Server{
			Addr: fmt.Sprintf(":%v", proxyArgs.serverPort),
		},
		RegistryHost:  proxyArgs.registryHost,
		RegistryProxy: httputil.NewSingleHostReverseProxy(registryTarget),
		Cache:         &lru.Cache{Db: db},
		RegClient: regclient.New(
			regclient.WithConfigHost(
				config.Host{
					Name: proxyArgs.registryHost,
					TLS:  tlsSetting,
				})),
		CleanSettings:       proxyArgs.CleanupArgs,
		UseForwardedHeaders: proxyArgs.UseForwardedHeaders,
	}

	if proxyArgs.serverCert != "" && proxyArgs.serverKey != "" {
		tlsPair, err := tls.LoadX509KeyPair(proxyArgs.serverCert, proxyArgs.serverKey)
		common.ExitIfError(err)
		registryProxy.Server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{tlsPair},
		}
	}

	registryProxy.RunProxy(ctx)

}

var startProxyCmd = &cobra.Command{
	Use:   "start",
	Short: "ci registry proxy",
	Long:  `start the proxy with the provided settings`,
	Run: func(cmd *cobra.Command, args []string) {
		startProxy(cmd.Context())
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {

		if proxyArgs.CleanupArgs.CleanTagsPercentage > 100 ||
			proxyArgs.CleanupArgs.CleanTagsPercentage < 0 {
			common.Log.Warnf("clean-tags-percentage invalid range - will be overridden ")
		}
		proxyArgs.CleanupArgs.CleanTagsPercentage = math.Max(0, math.Min(proxyArgs.CleanupArgs.CleanTagsPercentage/100, 1.0))

		if bytes, err := common.ParseByteString(proxyArgs.TargetDiskSizeByteString); err == nil {
			common.Log.Debugf("target usage bytes: %d", bytes)
			proxyArgs.CleanupArgs.TargetUsageBytes = bytes
		} else {
			common.ExitIfError(err)
		}

		return nil
	},
}

// setup command
func init() {
	rootCmd.AddCommand(startProxyCmd)

	startProxyCmd.Flags().IntVar(
		&proxyArgs.serverPort,
		"port",
		3000,
		"")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.serverCert,
		"cert",
		"",
		"x509 server certificate")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.serverKey,
		"key",
		"",
		"x509 server key")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.databaseDir,
		"db-dir",
		"/var/lib/registry",
		"db directory")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.CleanupArgs.RegistryBinary,
		"registry-bin",
		"/registry/bin/registry",
		"registry binary")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.CleanupArgs.RegistryConfig,
		"registry-conf",
		"/etc/docker/registry/config.yml",
		"registry config")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.registryHost,
		"registry-host",
		"127.0.0.1:5000",
		"registry host")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.registryScheme,
		"registry-scheme",
		"http",
		"registry scheme")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.CleanupArgs.RegistryDir,
		"registry-dir",
		"/var/lib/registry",
		"registry directory")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.TargetDiskSizeByteString,
		"target-disk-usage",
		"50Gi",
		"target usage of disk for a clean cycle, a scheduled clean cycle will clean tags until this threshold is met")

	startProxyCmd.Flags().Float64Var(
		&proxyArgs.CleanupArgs.CleanTagsPercentage,
		"clean-tags-percentage",
		10.0,
		"percentage of least recently used tags to remove each iteration of a clean cycle until the target-percentage is achieved")

	startProxyCmd.Flags().BoolVar(
		&proxyArgs.UseForwardedHeaders,
		"use-forwarded-headers",
		false,
		"use x-forwarded headers")

	startProxyCmd.Flags().BoolVar(
		&proxyArgs.CleanupArgs.UseOptimizedDiskCalculation,
		"separate-disk",
		false,
		"registry on separate disk or mount - use optimized disk size calculation")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.CleanupArgs.TimeZone,
		"timezone",
		"Local",
		"timezone string to use for scheduling based on the cron-string")

	startProxyCmd.Flags().StringVar(
		&proxyArgs.CleanupArgs.CronSchedule,
		"cleanup-cron",
		"0 0 * * *",
		"cron schedule for cleaning up the least recently used tags default is 0:00:00")

	_ = viper.BindPFlags(startProxyCmd.Flags())
}
