/*
Copyright 2025 The KServe Authors.

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

package kernelcachecommon

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kserve/kserve/pkg/constants"
)

func TestLoadKernelCacheConfig(t *testing.T) {
	t.Run("loads config from ConfigMap", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"kernelcache": `{
					"jobNamespace": "custom-namespace",
					"extractImage": "custom-image:v1",
					"jobTTLSecondsAfterFinished": 7200,
					"reconcileIntervalSeconds": 120
				}`,
			},
		}

		clientset := fake.NewSimpleClientset(configMap)
		ctx := context.Background()

		config, err := LoadKernelCacheConfig(ctx, clientset)
		if err != nil {
			t.Fatalf("LoadKernelCacheConfig failed: %v", err)
		}

		if config.JobNamespace != "custom-namespace" {
			t.Errorf("expected JobNamespace=custom-namespace, got %s", config.JobNamespace)
		}
		if config.ExtractImage != "custom-image:v1" {
			t.Errorf("expected ExtractImage=custom-image:v1, got %s", config.ExtractImage)
		}
		if *config.JobTTLSecondsAfterFinished != 7200 {
			t.Errorf("expected JobTTLSecondsAfterFinished=7200, got %d", *config.JobTTLSecondsAfterFinished)
		}
		if *config.ReconcileIntervalSeconds != 120 {
			t.Errorf("expected ReconcileIntervalSeconds=120, got %d", *config.ReconcileIntervalSeconds)
		}
	})

	t.Run("applies defaults when ConfigMap is empty", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{},
		}

		clientset := fake.NewSimpleClientset(configMap)
		ctx := context.Background()

		config, err := LoadKernelCacheConfig(ctx, clientset)
		if err != nil {
			t.Fatalf("LoadKernelCacheConfig failed: %v", err)
		}

		if config.JobNamespace != DefaultJobNamespace {
			t.Errorf("expected default JobNamespace=%s, got %s", DefaultJobNamespace, config.JobNamespace)
		}
		if config.ExtractImage != DefaultExtractImage {
			t.Errorf("expected default ExtractImage=%s, got %s", DefaultExtractImage, config.ExtractImage)
		}
		if *config.JobTTLSecondsAfterFinished != DefaultJobTTLSecondsAfterFinished {
			t.Errorf("expected default JobTTLSecondsAfterFinished=%d, got %d",
				DefaultJobTTLSecondsAfterFinished, *config.JobTTLSecondsAfterFinished)
		}
		if *config.ReconcileIntervalSeconds != DefaultReconcileIntervalSeconds {
			t.Errorf("expected default ReconcileIntervalSeconds=%d, got %d",
				DefaultReconcileIntervalSeconds, *config.ReconcileIntervalSeconds)
		}
	})

	t.Run("applies defaults for unset fields in partial config", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"kernelcache": `{
					"jobNamespace": "partial-namespace"
				}`,
			},
		}

		clientset := fake.NewSimpleClientset(configMap)
		ctx := context.Background()

		config, err := LoadKernelCacheConfig(ctx, clientset)
		if err != nil {
			t.Fatalf("LoadKernelCacheConfig failed: %v", err)
		}

		if config.JobNamespace != "partial-namespace" {
			t.Errorf("expected JobNamespace=partial-namespace, got %s", config.JobNamespace)
		}
		if config.ExtractImage != DefaultExtractImage {
			t.Errorf("expected default ExtractImage=%s, got %s", DefaultExtractImage, config.ExtractImage)
		}
		if *config.JobTTLSecondsAfterFinished != DefaultJobTTLSecondsAfterFinished {
			t.Errorf("expected default JobTTLSecondsAfterFinished=%d, got %d",
				DefaultJobTTLSecondsAfterFinished, *config.JobTTLSecondsAfterFinished)
		}
	})
}

func TestReplaceUrlTag(t *testing.T) {
	tests := []struct {
		name     string
		imageURL string
		digest   string
		expected string
	}{
		{
			name:     "replaces tag with digest",
			imageURL: "registry.io/image:v1.0",
			digest:   "sha256:abc123",
			expected: "registry.io/image@sha256:abc123",
		},
		{
			name:     "replaces existing digest",
			imageURL: "registry.io/image:v1.0@sha256:old123",
			digest:   "sha256:new456",
			expected: "registry.io/image:v1.0@sha256:new456",
		},
		{
			name:     "keeps same digest if already present",
			imageURL: "registry.io/image:v1.0@sha256:abc123",
			digest:   "sha256:abc123",
			expected: "registry.io/image:v1.0@sha256:abc123",
		},
		{
			name:     "adds digest when no tag present",
			imageURL: "registry.io/image",
			digest:   "sha256:abc123",
			expected: "registry.io/image@sha256:abc123",
		},
		{
			name:     "handles registry with port",
			imageURL: "registry.io:5000/image:v1.0",
			digest:   "sha256:abc123",
			expected: "registry.io:5000/image@sha256:abc123",
		},
		{
			name:     "handles registry with port and no tag",
			imageURL: "registry.io:5000/image",
			digest:   "sha256:abc123",
			expected: "registry.io:5000/image@sha256:abc123",
		},
		{
			name:     "returns empty string for empty imageURL",
			imageURL: "",
			digest:   "sha256:abc123",
			expected: "",
		},
		{
			name:     "returns empty string for empty digest",
			imageURL: "registry.io/image:v1.0",
			digest:   "",
			expected: "",
		},
		{
			name:     "handles multi-level path",
			imageURL: "registry.io/org/repo/image:v1.0",
			digest:   "sha256:abc123",
			expected: "registry.io/org/repo/image@sha256:abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceUrlTag(tt.imageURL, tt.digest)
			if result != tt.expected {
				t.Errorf("ReplaceUrlTag(%q, %q) = %q, expected %q",
					tt.imageURL, tt.digest, result, tt.expected)
			}
		})
	}
}
