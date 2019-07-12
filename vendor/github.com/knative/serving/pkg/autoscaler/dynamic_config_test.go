/*
Copyright 2018 The Knative Authors

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

package autoscaler_test

import (
	"testing"
	"time"

	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/resources"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

var configMap = map[string]string{"max-scale-up-rate": "1",
	"container-concurrency-target-percentage": "0.5",
	"container-concurrency-target-default":    "10.0",
	"stable-window":                           "4s",
	"panic-window":                            "5s",
	"scale-to-zero-grace-period":              "30s", // min
	"tick-interval":                           "8s"}

func TestNewDynamicConfigFromMap(t *testing.T) {
	dc, err := autoscaler.NewDynamicConfigFromMap(configMap, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("Failed to create dynamic configuration: %v", err)
	}

	if dc.Current().StableWindow != 4*time.Second {
		t.Fatalf("Unexpected configuration value: %v", dc.Current().StableWindow)
	}
}

func TestNewDynamicConfigFromMapError(t *testing.T) {
	invalidConfigMap := resources.CopyMap(configMap)
	invalidConfigMap["stable-window"] = "4"
	_, err := autoscaler.NewDynamicConfigFromMap(invalidConfigMap, zap.NewNop().Sugar())
	if err == nil {
		t.Fatal("Failed to detect configuration error")
	}
}

func TestUpdateDynamicConfig(t *testing.T) {
	dc, err := autoscaler.NewDynamicConfigFromMap(configMap, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("Failed to create dynamic configuration: %v", err)
	}

	updatedConfigMap := resources.CopyMap(configMap)
	updatedConfigMap["stable-window"] = "4m"

	dc.Update(&corev1.ConfigMap{
		Data: updatedConfigMap,
	})

	if dc.Current().StableWindow != 4*time.Minute {
		t.Fatalf("Unexpected configuration value: %v", dc.Current().StableWindow)
	}
}

func TestCurrentDynamicConfigIsSnapshot(t *testing.T) {
	dc, err := autoscaler.NewDynamicConfigFromMap(configMap, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("Failed to create dynamic configuration: %v", err)
	}

	initial := dc.Current()
	if initial.StableWindow != 4*time.Second {
		t.Fatalf("Unexpected configuration value: %v", dc.Current().StableWindow)
	}

	updatedConfigMap := resources.CopyMap(configMap)
	updatedConfigMap["stable-window"] = "4m"

	dc.Update(&corev1.ConfigMap{
		Data: updatedConfigMap,
	})

	if initial.StableWindow != 4*time.Second {
		t.Fatalf("Configuration snapshot has mutated unexpectedly: %v", dc.Current().StableWindow)
	}
}

func TestCurrentDynamicConfigIsImmutable(t *testing.T) {
	dc, err := autoscaler.NewDynamicConfigFromMap(configMap, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("Failed to create dynamic configuration: %v", err)
	}

	initial := dc.Current()
	if initial.StableWindow != 4*time.Second {
		t.Fatalf("Unexpected configuration value: %v", dc.Current().StableWindow)
	}

	initial.StableWindow = 4 * time.Minute

	if dc.Current().StableWindow != 4*time.Second {
		t.Fatalf("Configuration value mutated: %v", dc.Current().StableWindow)
	}
}

func TestUpdateDynamicConfigError(t *testing.T) {
	dc, err := autoscaler.NewDynamicConfigFromMap(configMap, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("Failed to create dynamic configuration: %v", err)
	}

	invalidConfigMap := resources.CopyMap(configMap)
	invalidConfigMap["stable-window"] = "4"
	invalidConfigMap["panic-window"] = "5m"

	dc.Update(&corev1.ConfigMap{
		Data: invalidConfigMap,
	})

	if dc.Current().PanicWindow != 5*time.Second {
		t.Fatalf("Configuration value modified on error: %v", dc.Current().PanicWindow)
	}
}
