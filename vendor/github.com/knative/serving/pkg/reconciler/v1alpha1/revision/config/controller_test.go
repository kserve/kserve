/*
Copyright 2018 The Knative Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	. "github.com/knative/pkg/configmap/testing"
	_ "github.com/knative/pkg/system/testing"
)

var noSidecarImage = ""

func TestControllerConfigurationFromFile(t *testing.T) {
	cm, example := ConfigMapsFromTestFile(t, ControllerConfigName, queueSidecarImageKey)

	if _, err := NewControllerConfigFromConfigMap(cm); err != nil {
		t.Errorf("NewControllerConfigFromConfigMap(actual) = %v", err)
	}

	if _, err := NewControllerConfigFromConfigMap(example); err != nil {
		t.Errorf("NewControllerConfigFromConfigMap(example) = %v", err)
	}
}

func TestControllerConfiguration(t *testing.T) {
	configTests := []struct {
		name           string
		wantErr        bool
		wantController interface{}
		config         *corev1.ConfigMap
	}{{
		name:    "controller configuration with bad registries",
		wantErr: false,
		wantController: &Controller{
			RegistriesSkippingTagResolving: sets.NewString("ko.local", ""),
			QueueSidecarImage:              noSidecarImage,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ControllerConfigName,
			},
			Data: map[string]string{
				queueSidecarImageKey:           noSidecarImage,
				registriesSkippingTagResolving: "ko.local,,",
			},
		}}, {
		name:    "controller configuration with registries",
		wantErr: false,
		wantController: &Controller{
			RegistriesSkippingTagResolving: sets.NewString("ko.local", "ko.dev"),
			QueueSidecarImage:              noSidecarImage,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ControllerConfigName,
			},
			Data: map[string]string{
				queueSidecarImageKey:           noSidecarImage,
				registriesSkippingTagResolving: "ko.local,ko.dev",
			},
		},
	}, {
		name:           "controller with no side car image",
		wantErr:        true,
		wantController: (*Controller)(nil),
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ControllerConfigName,
			},
			Data: map[string]string{},
		},
	}}

	for _, tt := range configTests {
		actualController, err := NewControllerConfigFromConfigMap(tt.config)

		if (err != nil) != tt.wantErr {
			t.Fatalf("Test: %q; NewControllerConfigFromConfigMap() error = %v, WantErr %v", tt.name, err, tt.wantErr)
		}

		if diff := cmp.Diff(actualController, tt.wantController); diff != "" {
			t.Fatalf("Test: %q; want %v, but got %v", tt.name, tt.wantController, actualController)
		}
	}
}
