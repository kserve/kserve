/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// DefaultsConfigName is the name of config map for the defaults.
	DefaultsConfigName = "config-defaults"
)

// NewDefaultsConfigFromMap creates a Defaults from the supplied Map
func NewDefaultsConfigFromMap(data map[string]string) (*Defaults, error) {
	nc := &Defaults{}

	// Process int64 fields
	for _, i64 := range []struct {
		key   string
		field *int64
		// specified exactly when optional
		defaultValue int64
	}{{
		key:          "revision-timeout-seconds",
		field:        &nc.RevisionTimeoutSeconds,
		defaultValue: DefaultRevisionTimeoutSeconds,
	}} {
		if raw, ok := data[i64.key]; !ok {
			*i64.field = i64.defaultValue
		} else if val, err := strconv.ParseInt(raw, 10, 64); err != nil {
			return nil, err
		} else {
			*i64.field = val
		}
	}

	// Process resource quantity fields
	for _, rsrc := range []struct {
		key   string
		field *resource.Quantity
		// specified exactly when optional
		defaultValue resource.Quantity
	}{{
		key:          "revision-cpu-request",
		field:        &nc.RevisionCPURequest,
		defaultValue: DefaultRevisionCPURequest,
	}} {
		if raw, ok := data[rsrc.key]; !ok {
			*rsrc.field = rsrc.defaultValue
		} else if val, err := resource.ParseQuantity(raw); err != nil {
			return nil, err
		} else {
			*rsrc.field = val
		}
	}

	return nc, nil
}

// NewDefaultsConfigFromConfigMap creates a Defaults from the supplied configMap
func NewDefaultsConfigFromConfigMap(config *corev1.ConfigMap) (*Defaults, error) {
	return NewDefaultsConfigFromMap(config.Data)
}

const (
	// DefaultRevisionTimeoutSeconds will be set if timeoutSeconds not specified.
	DefaultRevisionTimeoutSeconds = 5 * 60
)

// Pseudo-constants
var (
	// DefaultRevisionCPURequest will be set if resources.requests.cpu is not specified.
	DefaultRevisionCPURequest = resource.MustParse("400m")
)

// Defaults includes the default values to be populated by the webhook.
type Defaults struct {
	RevisionTimeoutSeconds int64

	RevisionCPURequest resource.Quantity
}
