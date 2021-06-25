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
	"github.com/boxboat/lru-registry/pkg/common"
	"github.com/boxboat/lru-registry/pkg/lru"
	"github.com/boxboat/lru-registry/pkg/proxy"
	"github.com/regclient/regclient/regclient"
	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
)

type ProxyArgs struct {
	serverPort     int
	serverCert     string
	serverKey      string
	registryHost   string
	registryScheme string
	CleanupArgs    proxy.CleanSettings
}

var (
	proxyArgs ProxyArgs
)

func runProxy(ctx context.Context) {
	db, err := bolt.Open("usage.db", 0600, nil)
	common.ExitIfError(err)
	defer db.Close()

	registryTarget, err := url.Parse(fmt.Sprintf("%s://%s", proxyArgs.registryScheme, proxyArgs.registryHost))
	common.ExitIfError(err)

	tlsSetting := regclient.TLSDisabled
	if proxyArgs.registryScheme == "https" {
		tlsSetting = regclient.TLSEnabled
	}

	registryProxy := &proxy.Proxy{
		Server: &http.Server{
			Addr: fmt.Sprintf(":%v", proxyArgs.serverPort),
		},
		RegistryHost:  proxyArgs.registryHost,
		RegistryProxy: httputil.NewSingleHostReverseProxy(registryTarget),
		Cache:         &lru.Cache{Db: db},
		RegClient: regclient.NewRegClient(
			regclient.WithConfigHost(
				regclient.ConfigHost{
					Name: proxyArgs.registryHost,
					TLS:  tlsSetting,
				})),
		CleanSettings: proxyArgs.CleanupArgs,
	}

	registryProxy.Cache.Init()

	if proxyArgs.serverCert != "" && proxyArgs.serverKey != "" {
		tlsPair, err := tls.LoadX509KeyPair(proxyArgs.serverCert, proxyArgs.serverKey)
		common.ExitIfError(err)
		registryProxy.Server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{tlsPair},
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", registryProxy.Proxy)
	registryProxy.Server.Handler = mux

	go registryProxy.CleanCycle(ctx)

	go func() {
		if registryProxy.Server.TLSConfig != nil {
			if err := registryProxy.Server.ListenAndServeTLS("", ""); err != nil {
				common.ExitIfError(err)
			}
		} else {
			if err := registryProxy.Server.ListenAndServe(); err != nil {
				common.ExitIfError(err)
			}
		}
	}()

	// listen for shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	common.Log.Infof("received shutdown signal, shutting down proxy")
	if err := registryProxy.Server.Shutdown(context.Background()); err != nil {
		common.Log.Infof("proxy shutdown: %v", err)
	}
}

var startProxyCmd = &cobra.Command{
	Use:   "start",
	Short: "ci registry proxy",
	Long:  `start the proxy with the provided settings`,
	Run: func(cmd *cobra.Command, args []string) {
		runProxy(cmd.Context())
	},
}

// setup command
func init() {
	rootCmd.AddCommand(startProxyCmd)

	startProxyCmd.Flags().IntVar(
		&proxyArgs.serverPort,
		"port",
		443,
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
		"/registry-data",
		"registry directory")

	startProxyCmd.Flags().Float64Var(
		&proxyArgs.CleanupArgs.MaxUsage,
		"max-percentage",
		.9,
		"maximum usage of disk size")

	startProxyCmd.Flags().Float64Var(
		&proxyArgs.CleanupArgs.MinReserveUsage,
		"min-percentage",
		.5,
		"minimum reserve of disk size")

	startProxyCmd.Flags().Uint64Var(
		&proxyArgs.CleanupArgs.MinReserveGigs,
		"min-reserve-gigabytes",
		20,
		"maximum usage of disk size")

	startProxyCmd.Flags().Float64Var(
		&proxyArgs.CleanupArgs.CleanTagsPercentage,
		"clean-tags-percentage",
		10.0,
		"clean percentage of least recently used tags")
}
