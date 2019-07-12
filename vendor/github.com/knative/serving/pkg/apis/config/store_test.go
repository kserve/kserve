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
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	logtesting "github.com/knative/pkg/logging/testing"
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/knative/pkg/configmap/testing"
)

var ignoreResourceQuantity = cmpopts.IgnoreUnexported(resource.Quantity{})

func TestStoreLoadWithContext(t *testing.T) {
	defer logtesting.ClearAll()
	store := NewStore(logtesting.TestLogger(t))

	defaultsConfig := ConfigMapFromTestFile(t, DefaultsConfigName)

	store.OnConfigChanged(defaultsConfig)

	config := FromContextOrDefaults(store.ToContext(context.Background()))

	t.Run("defaults", func(t *testing.T) {
		expected, _ := NewDefaultsConfigFromConfigMap(defaultsConfig)
		if diff := cmp.Diff(expected, config.Defaults, ignoreResourceQuantity); diff != "" {
			t.Errorf("Unexpected defaults config (-want, +got): %v", diff)
		}
	})
}

func TestStoreLoadWithContextOrDefaults(t *testing.T) {
	defer logtesting.ClearAll()

	defaultsConfig := ConfigMapFromTestFile(t, DefaultsConfigName)
	config := FromContextOrDefaults(context.Background())

	t.Run("defaults", func(t *testing.T) {
		expected, _ := NewDefaultsConfigFromConfigMap(defaultsConfig)
		if diff := cmp.Diff(expected, config.Defaults, ignoreResourceQuantity); diff != "" {
			t.Errorf("Unexpected defaults config (-want, +got): %v", diff)
		}
	})
}

func TestStoreImmutableConfig(t *testing.T) {
	defer logtesting.ClearAll()
	store := NewStore(logtesting.TestLogger(t))

	store.OnConfigChanged(ConfigMapFromTestFile(t, DefaultsConfigName))

	config := store.Load()

	config.Defaults.RevisionTimeoutSeconds = 1234

	newConfig := store.Load()

	if newConfig.Defaults.RevisionTimeoutSeconds == 1234 {
		t.Error("Defaults config is not immutable")
	}
}
