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
	"github.com/boxboat/lru-registry/pkg/common"
	"github.com/boxboat/lru-registry/pkg/lru"
	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/types"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"regexp"
	"time"
)

var (
	manifestMatch = regexp.MustCompile(`/v2/(.*)/manifests/(.*)`)
)

type Proxy struct {
	Server              *http.Server
	UseForwardedHeaders bool
	RegistryHost        string
	RegistryProxy       *httputil.ReverseProxy
	Cache               *lru.Cache
	RegClient           regclient.RegClient
	CleanSettings       CleanSettings
}

type CleanSettings struct {
	RegistryBinary      string
	RegistryDir         string
	RegistryConfig      string
	MaxUsage            float64
	MinReserveUsage     float64
	MinReserveGigs      uint64
	CleanTagsPercentage float64
}

func (proxy *Proxy) Proxy(res http.ResponseWriter, req *http.Request) {

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
		req.Header.Del("x-forwarded-for")
		req.Header.Del("x-forwarded-host")
		req.Header.Del("x-forwarded-port")
		req.Header.Del("x-real-ip")

		req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Forwarded-Port", req.URL.Port())
	}

	proxy.RegistryProxy.ServeHTTP(res, req)

}

func (proxy *Proxy) CleanCycle(ctx context.Context) {

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
			if proxy.removeTags() {
				lruImages := proxy.Cache.GetLruList()
				common.Log.Infof("total tags: %d", len(lruImages))
				removalTags := int(float64(len(lruImages)) * .5)
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
				err := proxy.registryGarbageCollect()
				common.LogIfError(err)
			}
		}
	}

}

func (proxy *Proxy) registryGarbageCollect() error {
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

	return usage > proxy.CleanSettings.MaxUsage || remainingGi < proxy.CleanSettings.MinReserveGigs || free < proxy.CleanSettings.MinReserveUsage
}
