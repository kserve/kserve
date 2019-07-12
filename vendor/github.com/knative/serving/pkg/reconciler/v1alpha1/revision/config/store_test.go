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
	"context"
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	logtesting "github.com/knative/pkg/logging/testing"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/logging"
	"github.com/knative/serving/pkg/network"

	. "github.com/knative/pkg/configmap/testing"
)

func TestStoreLoadWithContext(t *testing.T) {
	defer logtesting.ClearAll()
	store := NewStore(logtesting.TestLogger(t))

	controllerConfig := ConfigMapFromTestFile(t, ControllerConfigName, queueSidecarImageKey)
	networkConfig := ConfigMapFromTestFile(t, network.ConfigName)
	observabilityConfig := ConfigMapFromTestFile(t, ObservabilityConfigName)
	loggingConfig := ConfigMapFromTestFile(t, logging.ConfigMapName())
	autoscalerConfig := ConfigMapFromTestFile(t, autoscaler.ConfigName)

	store.OnConfigChanged(controllerConfig)
	store.OnConfigChanged(networkConfig)
	store.OnConfigChanged(observabilityConfig)
	store.OnConfigChanged(loggingConfig)
	store.OnConfigChanged(autoscalerConfig)

	config := FromContext(store.ToContext(context.Background()))

	t.Run("controller", func(t *testing.T) {
		expected, _ := NewControllerConfigFromConfigMap(controllerConfig)
		if diff := cmp.Diff(expected, config.Controller); diff != "" {
			t.Errorf("Unexpected controller config (-want, +got): %v", diff)
		}
	})

	t.Run("network", func(t *testing.T) {
		expected, _ := network.NewConfigFromConfigMap(networkConfig)
		ignoreDT := cmpopts.IgnoreFields(network.Config{}, "DomainTemplate")

		if diff := cmp.Diff(expected, config.Network, ignoreDT); diff != "" {
			t.Errorf("Unexpected controller config (-want, +got): %v", diff)
		}
	})

	t.Run("observability", func(t *testing.T) {
		expected, _ := NewObservabilityFromConfigMap(observabilityConfig)
		if diff := cmp.Diff(expected, config.Observability); diff != "" {
			t.Errorf("Unexpected observability config (-want, +got): %v", diff)
		}
	})

	t.Run("logging", func(t *testing.T) {
		expected, _ := logging.NewConfigFromConfigMap(loggingConfig)
		if diff := cmp.Diff(expected, config.Logging); diff != "" {
			t.Errorf("Unexpected logging config (-want, +got): %v", diff)
		}
	})

	t.Run("autoscaler", func(t *testing.T) {
		expected, _ := autoscaler.NewConfigFromConfigMap(autoscalerConfig)
		if diff := cmp.Diff(expected, config.Autoscaler); diff != "" {
			t.Errorf("Unexpected autoscaler config (-want, +got): %v", diff)
		}
	})
}

func TestStoreImmutableConfig(t *testing.T) {
	defer logtesting.ClearAll()
	store := NewStore(logtesting.TestLogger(t))

	store.OnConfigChanged(ConfigMapFromTestFile(t, ControllerConfigName, queueSidecarImageKey))
	store.OnConfigChanged(ConfigMapFromTestFile(t, network.ConfigName))
	store.OnConfigChanged(ConfigMapFromTestFile(t, ObservabilityConfigName))
	store.OnConfigChanged(ConfigMapFromTestFile(t, logging.ConfigMapName()))
	store.OnConfigChanged(ConfigMapFromTestFile(t, autoscaler.ConfigName))

	config := store.Load()

	config.Controller.QueueSidecarImage = "mutated"
	config.Network.IstioOutboundIPRanges = "mutated"
	config.Observability.FluentdSidecarImage = "mutated"
	config.Logging.LoggingConfig = "mutated"
	config.Autoscaler.MaxScaleUpRate = rand.Float64()

	newConfig := store.Load()

	if newConfig.Controller.QueueSidecarImage == "mutated" {
		t.Error("Controller config is not immutable")
	}
	if newConfig.Network.IstioOutboundIPRanges == "mutated" {
		t.Error("Network config is not immutable")
	}
	if newConfig.Observability.FluentdSidecarImage == "mutated" {
		t.Error("Observability config is not immutable")
	}
	if newConfig.Logging.LoggingConfig == "mutated" {
		t.Error("Logging config is not immutable")
	}
	if newConfig.Autoscaler.MaxScaleUpRate == config.Autoscaler.MaxScaleUpRate {
		t.Error("Autoscaler config is not immutable")
	}
}
