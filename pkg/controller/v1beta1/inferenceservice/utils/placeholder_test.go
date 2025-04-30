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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReplacePlaceholders(t *testing.T) {
	scenarios := map[string]struct {
		container           *corev1.Container
		meta                metav1.ObjectMeta
		expected            *corev1.Container
		expectErrorContains string
	}{
		"invalid-template-in-args": {
			container: &corev1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--test=dummy",
					"--new-arg=baz",
					"--foo={{.Name invalid - invalid}}",
				},
			},
			expectErrorContains: `failed to replace placeholder at ".args[2]"`,
		},
		"invalid-template-in-env": {
			container: &corev1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []corev1.EnvVar{
					{Name: "MODELS_DIR", Value: "{{.Labels.modelDir}}"},
					{Name: "MODEL_NAME", Value: "{{index .Annotations model-name}}"},
				},
			},
			expectErrorContains: `failed to replace placeholder at ".env[1].value"`,
		},
		"ReplaceArgsAndEnvPlaceholders": {
			container: &corev1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--foo={{.Name}}",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []corev1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "{{.Labels.modelDir}}"},
					{Name: "MODEL_NAME", Value: "{{index .Annotations \"model-name\"}}"},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			meta: metav1.ObjectMeta{
				Name: "bar",
				Labels: map[string]string{
					"modelDir": "/mnt/models",
				},
				Annotations: map[string]string{
					"model-name": "a-useful-model",
				},
			},
			expected: &corev1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []corev1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
					{Name: "MODEL_NAME", Value: "a-useful-model"},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := ReplacePlaceholders(scenario.container, scenario.meta)
			if scenario.expectErrorContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, scenario.expectErrorContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, scenario.expected, scenario.container)
		})
	}
}
