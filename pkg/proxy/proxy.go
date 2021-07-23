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

package proxy

import (
	"bytes"
	"context"
	"fmt"
	"github.com/boxboat/dockhand-lru-registry/pkg/common"
	"github.com/boxboat/dockhand-lru-registry/pkg/lru"
	"github.com/go-co-op/gocron"
	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/types"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
	"time"
)

var (
	manifestMatch = regexp.MustCompile(`/v2/(.*)/manifests/(.*)`)
)

const (
	writers int64 = 10000
)

type Proxy struct {
	Server               *http.Server
	UseForwardedHeaders  bool
	RegistryHost         string
	RegistryProxy        *httputil.ReverseProxy
	Cache                *lru.Cache
	RegClient            regclient.RegClient
	CleanSettings        CleanSettings
	MaintenanceSemaphore *semaphore.Weighted
	MaintenanceScheduler *gocron.Scheduler
}

type CleanSettings struct {
	RegistryBinary        string
	RegistryDir           string
	RegistryConfig        string
	TargetUsagePercentage float64
	CleanTagsPercentage   float64
	TimeZone              string
	CronSchedule          string
}

func (proxy *Proxy) healthz(res http.ResponseWriter, _ *http.Request) {
	res.WriteHeader(http.StatusOK)
	_, err := res.Write([]byte(fmt.Sprint("OK")))
	common.LogIfError(err)
}

func (proxy *Proxy) serveProxy(res http.ResponseWriter, req *http.Request) {

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		if proxy.MaintenanceSemaphore.TryAcquire(1) {
			defer proxy.MaintenanceSemaphore.Release(1)
		} else {
			res.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}

	common.Log.Debugf(`%s %s`, req.Method, req.URL)
	if req.Method == http.MethodHead || req.Method == http.MethodPut {
		if match := manifestMatch.MatchString(req.URL.Path); match {
			matches := manifestMatch.FindStringSubmatch(req.URL.Path)
			repo := matches[1]
			tag := matches[2]
			image := fmt.Sprintf(`%s:%s`, repo, tag)
			if req.Method == http.MethodHead {
				common.Log.Infof(`pulling %s`, image)
			} else if req.Method == http.MethodPut {
				common.Log.Infof(`pushing %s`, image)
			}

			proxy.Cache.AddOrUpdate(&lru.Image{
				Repo:       repo,
				Tag:        tag,
				AccessTime: time.Now(),
			})
		}
	}

	if !proxy.UseForwardedHeaders {
		common.Log.Debugf("use-forwarded-headers set to false deleting x-forwarded headers")
		req.Header.Del("x-forwarded-for")
		req.Header.Del("x-forwarded-host")
		req.Header.Del("x-forwarded-port")
		req.Header.Del("x-real-ip")
	}

	if value := req.Header.Get("x-forwarded-proto"); value == "" {
		if proxy.Server.TLSConfig != nil {
			common.Log.Debugf("set x-forwarded-proto: %s", "https")
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			common.Log.Debugf("set x-forwarded-proto: %s", "http")
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	} else {
		common.Log.Debugf("using x-forwarded-proto: %s", value)
	}

	proxy.RegistryProxy.ServeHTTP(res, req)
}

func (proxy *Proxy) runGarbageCollection(ctx context.Context){
	if err := proxy.MaintenanceSemaphore.Acquire(ctx, writers); err != nil {
		common.Log.Warnf("unable to acquire lock skipping garbage collection: %v", err)
		return
	}
	defer proxy.MaintenanceSemaphore.Release(writers)
	err := proxy.executeGarbageCollection()
	common.LogIfError(err)
}

func (proxy *Proxy) cleanup(ctx context.Context) {
	proxy.runGarbageCollection(ctx)
	if proxy.removeTags() {
		for proxy.removeTags() {
			lruImages := proxy.Cache.GetLruList()
			common.Log.Infof("total tags: %d", len(lruImages))
			removalTags := int(float64(len(lruImages)) * proxy.CleanSettings.CleanTagsPercentage)
			common.Log.Infof("Removing %d tags", removalTags)

			for idx, image := range lruImages {
				if idx < removalTags {
					if ref, err := types.NewRef(image.CanonicalName(proxy.RegistryHost)); err == nil {
						common.Log.Infof("Removing %s", ref.CommonName())
						if err = proxy.RegClient.TagDelete(context.Background(), ref); err == nil {
							proxy.Cache.Remove(&image)
						} else {
							common.LogIfError(err)
						}
					} else {
						common.LogIfError(err)
					}
				} else {
					break
				}
			}
			proxy.runGarbageCollection(ctx)
		}
	}
}

func (proxy *Proxy) executeGarbageCollection() error {
	gc := exec.Command(
		proxy.CleanSettings.RegistryBinary,
		"garbage-collect",
		"--delete-untagged",
		proxy.CleanSettings.RegistryConfig)

	var combinedOutout bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &combinedOutout)
	gc.Stdout = mw
	gc.Stderr = mw

	if err := gc.Run(); err != nil {
		return err
	}
	fmt.Print(combinedOutout.String())

	return nil
}

func (proxy *Proxy) removeTags() bool {
	var stat unix.Statfs_t
	if err := unix.Statfs(proxy.CleanSettings.RegistryDir, &stat); err != nil {
		common.LogIfError(err)
		return false
	}

	free := (float64(stat.Bavail) / float64(stat.Blocks)) * 100.0
	usage := 100.0 - free
	common.Log.Infof("%s using %f of disk", proxy.CleanSettings.RegistryDir, usage)
	remainingGi := (stat.Bavail * uint64(stat.Bsize) / 1024) / 1024 / 1024
	common.Log.Infof("Remaining %d Gi", remainingGi)

	return usage > proxy.CleanSettings.TargetUsagePercentage
}

func (proxy *Proxy) listenAndServe() {
	if proxy.Server.TLSConfig != nil {
		if err := proxy.Server.ListenAndServeTLS("", ""); err != nil {
			common.ExitIfError(err)
		}
	} else {
		if err := proxy.Server.ListenAndServe(); err != nil {
			common.ExitIfError(err)
		}
	}
}

func (proxy *Proxy) RunProxy(ctx context.Context) {
	proxy.MaintenanceSemaphore = semaphore.NewWeighted(writers)
	location, err := time.LoadLocation(proxy.CleanSettings.TimeZone)
	if err != nil {
		common.Log.Warnf("unable to parse timezone %s, falling back to UTC", proxy.CleanSettings.TimeZone)
		location = time.UTC
	}
	proxy.MaintenanceScheduler = gocron.NewScheduler(location)
	proxy.MaintenanceScheduler.Cron(proxy.CleanSettings.CronSchedule).SingletonMode().Do(func() { proxy.cleanup(ctx) })
	proxy.MaintenanceScheduler.StartAsync()

	mux := http.NewServeMux()
	mux.HandleFunc("/", proxy.serveProxy)
	mux.HandleFunc("/healthz", proxy.healthz)
	proxy.Server.Handler = mux

	proxy.Cache.Init()

	go proxy.listenAndServe()

	// listen for shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	common.Log.Infof("received shutdown signal, shutting down proxy")
	if err := proxy.Server.Shutdown(context.Background()); err != nil {
		common.Log.Infof("proxy shutdown: %v", err)
	}

}
