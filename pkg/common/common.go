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

package common

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
	"strings"
)

var (
	// Log for global use
	Log = log.New()
)

const (
	// GiB - GibiByte size
	GiB       = 1024 * 1024 * 1024
	GiBSuffix = "Gi"
	// MiB - MebiByte size
	MiB       = 1024 * 1024
	MiBSuffix = "Mi"
	// KiB - KibiByte size
	KiB       = 1024
	KiBSuffix = "Ki"
)

// ExitIfError will generically handle an error by logging its contents
// and exiting with a return code of 1.
func ExitIfError(err error) {
	if err != nil {
		Log.Errorf("%v", err)
		os.Exit(1)
	}
}

func LogIfError(err error) {
	if err != nil {
		Log.Warnf("%v", err)
	}
}

func ParseByteString(b string) (uint64, error) {
	var bytes uint64 = 0

	if strings.HasSuffix(b, GiBSuffix) {
		if raw, err := strconv.Atoi(strings.ReplaceAll(b, GiBSuffix, "")); err == nil {
			bytes = uint64(raw) * GiB
		} else {
			return bytes, err
		}
	} else if strings.HasSuffix(b, MiBSuffix) {
		if raw, err := strconv.Atoi(strings.ReplaceAll(b, MiBSuffix, "")); err == nil {
			bytes = uint64(raw) * MiB
		} else {
			return bytes, err
		}
	} else if strings.HasSuffix(b, KiBSuffix) {
		if raw, err := strconv.Atoi(strings.ReplaceAll(b, KiBSuffix, "")); err == nil {
			bytes = uint64(raw) * KiB
		} else {
			return bytes, err
		}
	} else {
		return bytes, fmt.Errorf("no valid suffix, must be Gi, Mi or Ki")
	}

	return bytes, nil
}
