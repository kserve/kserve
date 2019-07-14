/*
Copyright 2019 The Knative Authors.

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

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/system"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/knative/pkg/configmap/testing"
	_ "github.com/knative/pkg/system/testing"
)

func TestDefaultsConfigurationFromFile(t *testing.T) {
	cm, example := ConfigMapsFromTestFile(t, DefaultsConfigName)

	if _, err := NewDefaultsConfigFromConfigMap(cm); err != nil {
		t.Errorf("NewDefaultsConfigFromConfigMap(actual) = %v", err)
	}

	if _, err := NewDefaultsConfigFromConfigMap(example); err != nil {
		t.Errorf("NewDefaultsConfigFromConfigMap(example) = %v", err)
	}
}

func TestDefaultsConfiguration(t *testing.T) {
	configTests := []struct {
		name         string
		wantErr      bool
		wantDefaults interface{}
		config       *corev1.ConfigMap
	}{{
		name:    "defaults configuration",
		wantErr: false,
		wantDefaults: &Defaults{
			RevisionTimeoutSeconds: DefaultRevisionTimeoutSeconds,
			RevisionCPURequest:     DefaultRevisionCPURequest,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      DefaultsConfigName,
			},
			Data: map[string]string{},
		},
	}, {
		name:    "specified values",
		wantErr: false,
		wantDefaults: &Defaults{
			RevisionTimeoutSeconds: 123,
			RevisionCPURequest:     resource.MustParse("123m"),
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      DefaultsConfigName,
			},
			Data: map[string]string{
				"revision-timeout-seconds": "123",
				"revision-cpu-request":     "123m",
			},
		},
	}, {
		name:         "bad revision timeout",
		wantErr:      true,
		wantDefaults: (*Defaults)(nil),
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      DefaultsConfigName,
			},
			Data: map[string]string{
				"revision-timeout-seconds": "asdf",
			},
		},
	}, {
		name:         "bad resource",
		wantErr:      true,
		wantDefaults: (*Defaults)(nil),
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      DefaultsConfigName,
			},
			Data: map[string]string{
				"revision-cpu-request": "bad",
			},
		},
	}}

	for _, tt := range configTests {
		actualDefaults, err := NewDefaultsConfigFromConfigMap(tt.config)

		if (err != nil) != tt.wantErr {
			t.Fatalf("Test: %q; NewDefaultsConfigFromConfigMap() error = %v, WantErr %v", tt.name, err, tt.wantErr)
		}

		if diff := cmp.Diff(actualDefaults, tt.wantDefaults, ignoreResourceQuantity); diff != "" {
			t.Fatalf("Test: %q; want %v, but got %v", tt.name, tt.wantDefaults, actualDefaults)
		}
	}
}
