/*
Copyright 2026 The KServe Authors.

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

package metrics

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	defaultCertName = "tls.crt"
	defaultKeyName  = "tls.key"
)

// ConfigureServerOptions configures the controller-runtime metrics server and
// validates any user-provided certificate directory.
func ConfigureServerOptions(options metricsserver.Options) (metricsserver.Options, error) {
	if options.CertDir != "" && !options.SecureServing {
		return metricsserver.Options{}, errors.New("metrics certificate path requires secure serving")
	}

	if !options.SecureServing {
		return options, nil
	}

	options.FilterProvider = filters.WithAuthenticationAndAuthorization
	if options.CertDir == "" {
		return options, nil
	}

	for _, fileName := range []string{cmp.Or(options.CertName, defaultCertName), cmp.Or(options.KeyName, defaultKeyName)} {
		path := filepath.Join(options.CertDir, fileName)
		fileInfo, err := os.Stat(path)
		if err != nil {
			return metricsserver.Options{}, fmt.Errorf("validate metrics certificate file %q: %w", path, err)
		}
		if !fileInfo.Mode().IsRegular() {
			return metricsserver.Options{}, fmt.Errorf("metrics certificate file %q is not a regular file", path)
		}
	}

	return options, nil
}
