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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestAttachStorageInitializerConfidential(t *testing.T) {
	baseConfig := &v1alpha1.StorageContainerSpec{
		Container: corev1.Container{
			Name:  "storage-initializer",
			Image: "kserve/storage-initializer:latest",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}

	tests := []struct {
		name            string
		confidential    *v1alpha2.ConfidentialSpec
		expectedImage   string
		expectedEnvVars map[string]string
	}{
		{
			name:          "nil confidential spec uses standard image",
			confidential:  nil,
			expectedImage: "kserve/storage-initializer:latest",
		},
		{
			name:          "disabled confidential spec uses standard image",
			confidential:  &v1alpha2.ConfidentialSpec{Enabled: false},
			expectedImage: "kserve/storage-initializer:latest",
		},
		{
			name: "enabled sets env vars without swapping image",
			confidential: &v1alpha2.ConfidentialSpec{
				Enabled:    true,
				ResourceId: ptr.To("kbs:///default/key/model-key"),
			},
			expectedImage: "kserve/storage-initializer:latest",
			expectedEnvVars: map[string]string{
				constants.ConfidentialEnabledEnvVar:    "true",
				constants.ConfidentialResourceIdEnvVar: "kbs:///default/key/model-key",
			},
		},
		{
			name: "enabled without resourceId",
			confidential: &v1alpha2.ConfidentialSpec{
				Enabled: true,
			},
			expectedImage: "kserve/storage-initializer:latest",
			expectedEnvVars: map[string]string{
				constants.ConfidentialEnabledEnvVar: "true",
			},
		},
		{
			name: "restart avoidance preserves existing image",
			confidential: &v1alpha2.ConfidentialSpec{
				Enabled: true,
			},
			expectedImage: "kserve/storage-initializer:v1-existing",
			expectedEnvVars: map[string]string{
				constants.ConfidentialEnabledEnvVar: "true",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}

			// For restart-avoidance test, simulate an existing deployment
			curr := corev1.PodSpec{}
			if tc.name == "restart avoidance preserves existing image" {
				curr.InitContainers = []corev1.Container{
					{
						Name:  constants.StorageInitializerContainerName,
						Image: "kserve/storage-initializer:v1-existing",
					},
				}
			}

			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "kserve-container"},
				},
			}

			err := r.attachStorageInitializer(
				"s3://bucket/model",
				curr,
				podSpec,
				baseConfig,
				tc.confidential,
				"kserve-container",
				"/mnt/models",
			)
			require.NoError(t, err)

			// Find the storage-initializer init container
			var initContainer *corev1.Container
			for i := range podSpec.InitContainers {
				if podSpec.InitContainers[i].Name == constants.StorageInitializerContainerName {
					initContainer = &podSpec.InitContainers[i]
					break
				}
			}
			require.NotNil(t, initContainer, "storage-initializer init container should exist")

			assert.Equal(t, tc.expectedImage, initContainer.Image)

			if tc.expectedEnvVars != nil {
				envMap := make(map[string]string)
				for _, env := range initContainer.Env {
					envMap[env.Name] = env.Value
				}
				for key, expected := range tc.expectedEnvVars {
					assert.Equal(t, expected, envMap[key], "env var %s", key)
				}
			} else {
				// No confidential env vars should be present
				for _, env := range initContainer.Env {
					assert.NotEqual(t, constants.ConfidentialEnabledEnvVar, env.Name)
					assert.NotEqual(t, constants.ConfidentialResourceIdEnvVar, env.Name)
				}
			}
		})
	}
}
