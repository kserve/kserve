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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strconv"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis"
)

const (
	// DefaultsConfigName is the name of config map for the defaults.
	DefaultsConfigName = "config-defaults"

	// DefaultRevisionTimeoutSeconds will be set if timeoutSeconds not specified.
	DefaultRevisionTimeoutSeconds = 5 * 60

	// DefaultMaxRevisionTimeoutSeconds will be set if MaxRevisionTimeoutSeconds is not specified.
	DefaultMaxRevisionTimeoutSeconds = 10 * 60

	// DefaultUserContainerName is the default name we give to the container
	// specified by the user, if `name:` is omitted.
	DefaultUserContainerName = "user-container"
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
	}, {
		key:          "max-revision-timeout-seconds",
		field:        &nc.MaxRevisionTimeoutSeconds,
		defaultValue: DefaultMaxRevisionTimeoutSeconds,
	}} {
		if raw, ok := data[i64.key]; !ok {
			*i64.field = i64.defaultValue
		} else if val, err := strconv.ParseInt(raw, 10, 64); err != nil {
			return nil, err
		} else {
			*i64.field = val
		}
	}

	if nc.RevisionTimeoutSeconds > nc.MaxRevisionTimeoutSeconds {
		return nil, fmt.Errorf("revision-timeout-seconds (%d) cannot be greater than max-revision-timeout-seconds (%d)", nc.RevisionTimeoutSeconds, nc.MaxRevisionTimeoutSeconds)
	}

	// Process resource quantity fields
	for _, rsrc := range []struct {
		key   string
		field **resource.Quantity
	}{{
		key:   "revision-cpu-request",
		field: &nc.RevisionCPURequest,
	}, {
		key:   "revision-memory-request",
		field: &nc.RevisionMemoryRequest,
	}, {
		key:   "revision-cpu-limit",
		field: &nc.RevisionCPULimit,
	}, {
		key:   "revision-memory-limit",
		field: &nc.RevisionMemoryLimit,
	}} {
		if raw, ok := data[rsrc.key]; !ok {
			*rsrc.field = nil
		} else if val, err := resource.ParseQuantity(raw); err != nil {
			return nil, err
		} else {
			*rsrc.field = &val
		}
	}

	if raw, ok := data["container-name-template"]; !ok {
		nc.UserContainerNameTemplate = DefaultUserContainerName
	} else {
		tmpl, err := template.New("user-container").Parse(raw)
		if err != nil {
			return nil, err
		}
		// Check that the template properly applies to ObjectMeta.
		if err := tmpl.Execute(ioutil.Discard, metav1.ObjectMeta{}); err != nil {
			return nil, fmt.Errorf("error executing template: %v", err)
		}
		// We store the raw template because we run deepcopy-gen on the
		// config and that doesn't copy nicely.
		nc.UserContainerNameTemplate = raw
	}

	return nc, nil
}

// NewDefaultsConfigFromConfigMap creates a Defaults from the supplied configMap
func NewDefaultsConfigFromConfigMap(config *corev1.ConfigMap) (*Defaults, error) {
	return NewDefaultsConfigFromMap(config.Data)
}

// Defaults includes the default values to be populated by the webhook.
type Defaults struct {
	RevisionTimeoutSeconds int64
	// This is the timeout set for cluster ingress.
	// RevisionTimeoutSeconds must be less than this value.
	MaxRevisionTimeoutSeconds int64

	UserContainerNameTemplate string

	RevisionCPURequest    *resource.Quantity
	RevisionCPULimit      *resource.Quantity
	RevisionMemoryRequest *resource.Quantity
	RevisionMemoryLimit   *resource.Quantity
}

// UserContainerName returns the name of the user container based on the context.
func (d *Defaults) UserContainerName(ctx context.Context) string {
	tmpl := template.Must(
		template.New("user-container").Parse(d.UserContainerNameTemplate))
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, apis.ParentMeta(ctx)); err != nil {
		return ""
	}
	return buf.String()
}
