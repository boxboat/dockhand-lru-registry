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

package lru

import (
	"fmt"
	"github.com/boxboat/lru-registry/pkg/common"
	bolt "go.etcd.io/bbolt"
	"strings"
	"time"
)

var (
	ImageBucket  = []byte("images")
	AccessBucket = []byte("access")
)

type Cache struct {
	Db *bolt.DB
}

type Image struct {
	Repo       string
	Tag        string
	AccessTime time.Time
}

func (image *Image) Name() string {
	return fmt.Sprintf("%s:%s", image.Repo, image.Tag)
}
func (image *Image) CanonicalName(registryAddress string) string {
	if strings.HasPrefix(registryAddress, "http") {
		registryAddress = strings.Split(registryAddress, "://")[1]
	}
	canonicalName := fmt.Sprintf("%s/%s", registryAddress, image.Name())
	return canonicalName
}

func (cache *Cache) Init() error {
	if err := cache.Db.Update(cache.createBucket(ImageBucket)); err != nil {
		return err
	}
	if err := cache.Db.Update(cache.createBucket(AccessBucket)); err != nil {
		return err
	}
	return nil
}

func (cache *Cache) createBucket(bucket []byte) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return fmt.Errorf("create bucket: %v", err)
		}
		return nil
	}
}

func (cache *Cache) getAccessTime(image *Image) *time.Time {
	accessTime := &time.Time{}
	_ = cache.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(ImageBucket)
		if v := b.Get([]byte(image.Name())); v != nil {
			if err := accessTime.UnmarshalText(v); err != nil {
				common.LogIfError(err)
				return err
			}
		}
		return nil
	})
	return accessTime
}

func (cache *Cache) updateAccessTime(image *Image, oldAccessTime *time.Time) error {
	_ = cache.Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(AccessBucket)
		if err := b.Delete([]byte(oldAccessTime.Format(time.RFC3339))); err != nil {
			return err
		}
		if err := b.Put([]byte(image.AccessTime.Format(time.RFC3339)), []byte(image.Name())); err != nil {
			return err
		}
		b = tx.Bucket(ImageBucket)
		if err := b.Put([]byte(image.Name()), []byte(image.AccessTime.Format(time.RFC3339))); err != nil {
			return err
		}
		return nil
	})
	return nil
}

func (cache *Cache) addImage(image *Image) error {
	_ = cache.Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(AccessBucket)
		if err := b.Put([]byte(image.AccessTime.Format(time.RFC3339)), []byte(image.Name())); err != nil {
			return err
		}
		b = tx.Bucket(ImageBucket)
		if err := b.Put([]byte(image.Name()), []byte(image.AccessTime.Format(time.RFC3339))); err != nil {
			return err
		}
		return nil
	})
	return nil
}

func (cache *Cache) AddOrUpdate(image *Image) {
	lastAccessTime := cache.getAccessTime(image)
	if lastAccessTime != nil && !lastAccessTime.Equal(image.AccessTime) {
		common.Log.Debugf("updating access time: %s", image.AccessTime.Format(time.RFC3339))
		cache.updateAccessTime(image, lastAccessTime)
	} else {
		cache.addImage(image)
	}
}

func (cache *Cache) Remove(image *Image) {
	_ = cache.Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(AccessBucket)
		if err := b.Delete([]byte(image.AccessTime.Format(time.RFC3339))); err != nil {
			common.LogIfError(err)
			return err
		}

		b = tx.Bucket(ImageBucket)
		if err := b.Delete([]byte(image.Name())); err != nil {
			common.LogIfError(err)
			return err
		}
		return nil
	})
}

func (cache *Cache) GetLruList() []Image {
	var images []Image
	cache.Db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(AccessBucket).Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			access := time.Time{}
			_ = access.UnmarshalText(k)
			images = append(images, Image{
				Repo:       strings.Split(string(v), ":")[0],
				Tag:        strings.Split(string(v), ":")[1],
				AccessTime: access,
			})
		}
		return nil
	})
	return images
}
