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
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/boxboat/dockhand-lru-registry/pkg/common"
	"github.com/boxboat/dockhand-lru-registry/pkg/lru"
	"github.com/go-co-op/gocron"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sys/unix"
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
	RegClient            *regclient.RegClient
	CleanSettings        CleanSettings
	MaintenanceSemaphore *semaphore.Weighted
	MaintenanceScheduler *gocron.Scheduler
}

type CleanSettings struct {
	RegistryBinary              string
	RegistryDir                 string
	RegistryConfig              string
	TargetUsageBytes            uint64
	CleanTagsPercentage         float64
	TimeZone                    string
	CronSchedule                string
	UseOptimizedDiskCalculation bool
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

func (proxy *Proxy) runGarbageCollection(ctx context.Context) {
	if err := proxy.MaintenanceSemaphore.Acquire(ctx, writers); err != nil {
		common.Log.Warnf("unable to acquire lock skipping garbage collection: %v", err)
		return
	}
	defer proxy.MaintenanceSemaphore.Release(writers)
	err := proxy.executeGarbageCollection(ctx)
	common.LogIfError(err)
}

func (proxy *Proxy) cleanup(ctx context.Context) {
	common.Log.Debugf(
		"executing scheduled cleanup based on TZ=%s '%s'",
		proxy.CleanSettings.TimeZone,
		proxy.CleanSettings.CronSchedule)

	proxy.runGarbageCollection(ctx)
	remove, _ := proxy.shouldRemoveTags()
	iteration := 0

	for remove {
		lruImages := proxy.Cache.GetLruList()
		common.Log.Infof("total tags: %d", len(lruImages))
		minTagRemoval := math.Min(float64(iteration), 1)
		percentageTagRemoval := math.Round(float64(len(lruImages)) * proxy.CleanSettings.CleanTagsPercentage)
		removalTags := int(math.Max(percentageTagRemoval, minTagRemoval))
		common.Log.Infof("iteration %d: removing %d tags", iteration, removalTags)

		for idx, image := range lruImages {
			if idx < removalTags {
				if ref, err := ref.New(image.CanonicalName(proxy.RegistryHost)); err == nil {
					common.Log.Infof("Removing %s", ref.CommonName())
					if err = proxy.RegClient.TagDelete(ctx, ref); err == nil {
						proxy.Cache.Remove(&image)
					} else if errors.Is(err, types.ErrNotFound) {
						proxy.Cache.Remove(&image)
					} else {
						common.LogIfError(err)
						if _, err := proxy.RegClient.ManifestGet(ctx, ref); err != nil && errors.Is(err, types.ErrNotFound) {
							proxy.Cache.Remove(&image)
						}
					}
				} else {
					common.LogIfError(err)
				}
			} else {
				break
			}
		}
		proxy.runGarbageCollection(ctx)
		tryAgain, currentBytes := proxy.shouldRemoveTags()
		if tryAgain && (len(lruImages)-removalTags) <= 0 {
			// we have reached a state where we can't remove anymore tags
			remove = false
			common.Log.Warnf("unable to reach regisry target %d bytes  - exiting cleanup with %d bytes", proxy.CleanSettings.TargetUsageBytes, currentBytes)
		} else {
			remove = tryAgain
		}
		iteration++
	}
}

func (proxy *Proxy) executeGarbageCollection(ctx context.Context) error {
	gc := exec.CommandContext(
		ctx,
		proxy.CleanSettings.RegistryBinary,
		"garbage-collect",
		"--delete-untagged",
		proxy.CleanSettings.RegistryConfig)

	var combinedOutput bytes.Buffer
	gc.Stdout = &combinedOutput
	gc.Stderr = &combinedOutput

	if err := gc.Run(); err != nil {
		common.Log.Warnf("gc: %s", combinedOutput.String())
		return err
	}
	common.Log.Infof("gc: %s", combinedOutput.String())

	return nil
}

func (proxy *Proxy) shouldRemoveTags() (bool, uint64) {
	var usedBytes uint64 = 0
	var err error = nil
	if proxy.CleanSettings.UseOptimizedDiskCalculation {
		usedBytes, err = sizeOfDisk(proxy.CleanSettings.RegistryDir)
	} else {
		usedBytes, err = sizeOfDir(proxy.CleanSettings.RegistryDir)
	}
	common.LogIfError(err)

	common.Log.Debugf("registry using %d bytes", usedBytes)
	common.Log.Debugf("registry target %d bytes", proxy.CleanSettings.TargetUsageBytes)

	return usedBytes > proxy.CleanSettings.TargetUsageBytes, usedBytes
}

func sizeOfDisk(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		common.LogIfError(err)
		return 0, err
	}

	usedBytes := (stat.Blocks * uint64(stat.Bsize)) - (stat.Bfree * uint64(stat.Bsize))
	return usedBytes, nil
}

func sizeOfDir(path string) (uint64, error) {
	var size int64 = 0

	readSize := func(path string, file os.FileInfo, err error) error {
		if !file.IsDir() {
			size += file.Size()
		}
		return nil
	}

	err := filepath.Walk(path, readSize)

	return uint64(size), err
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
	proxyCtx, cancel := context.WithCancel(ctx)

	proxy.MaintenanceSemaphore = semaphore.NewWeighted(writers)
	location, err := time.LoadLocation(proxy.CleanSettings.TimeZone)
	if err != nil {
		common.Log.Warnf("unable to parse timezone %s, falling back to UTC", proxy.CleanSettings.TimeZone)
		location = time.UTC
	}
	proxy.MaintenanceScheduler = gocron.NewScheduler(location)
	proxy.MaintenanceScheduler.Cron(proxy.CleanSettings.CronSchedule).SingletonMode().Do(func() { proxy.cleanup(proxyCtx) })
	proxy.MaintenanceScheduler.StartAsync()

	mux := http.NewServeMux()
	mux.HandleFunc("/", proxy.serveProxy)
	mux.HandleFunc("/healthz", proxy.healthz)
	proxy.Server.Handler = mux

	err = proxy.Cache.Init()
	common.ExitIfError(err)

	go proxy.listenAndServe()

	// listen for shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	cancel()
	common.Log.Infof("received shutdown signal, shutting down proxy")
	if err := proxy.Server.Shutdown(context.Background()); err != nil {
		common.Log.Infof("proxy shutdown: %v", err)
	}
}
