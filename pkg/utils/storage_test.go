/*
Copyright 2021 The KServe Authors.

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

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestFindCommonParentPath(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		paths    []string
		expected string
	}{
		"EmptyPaths": {
			paths:    []string{},
			expected: "",
		},
		"SinglePath": {
			paths:    []string{"/mnt/models"},
			expected: "/mnt/models",
		},
		"CommonParent": {
			paths: []string{
				"/mnt/models/model1",
				"/mnt/models/model2",
			},
			expected: "/mnt/models",
		},
		"DifferentRoots": {
			paths: []string{
				"/mnt/models",
				"/opt/models",
			},
			expected: "/",
		},
		"NestedCommonParent": {
			paths: []string{
				"/mnt/models/pytorch/model1",
				"/mnt/models/pytorch/model2",
				"/mnt/models/pytorch/",
			},
			expected: "/mnt/models/pytorch",
		},
		"NoCommonParent": {
			paths: []string{
				"/mnt/models",
				"/opt/data",
				"/var/cache",
			},
			expected: "/",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := FindCommonParentPath(scenario.paths)
			g.Expect(result).To(gomega.Equal(scenario.expected))
		})
	}
}

func TestGetVolumeNameFromPath(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		path     string
		expected string
	}{
		"RootPath": {
			path:     "/",
			expected: "",
		},
		"SimplePath": {
			path:     "/mnt/models",
			expected: "mnt-models",
		},
		"NestedPath": {
			path:     "/mnt/models/subdir",
			expected: "mnt-models-subdir",
		},
		"PathWithTrailingSlash": {
			path:     "/mnt/models/",
			expected: "mnt-models",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := GetVolumeNameFromPath(scenario.path)
			g.Expect(result).To(gomega.Equal(scenario.expected))
		})
	}
}

func TestAddDefaultHuggingFaceEnvVars(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		initialEnvVars  []corev1.EnvVar
		expectedEnvVars []corev1.EnvVar
		description     string
	}{
		"EmptyContainer": {
			initialEnvVars: []corev1.EnvVar{},
			expectedEnvVars: []corev1.EnvVar{
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "1"},
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
				{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "8"},
			},
			description: "Should add all HF env vars when container has no env vars",
		},
		"UserOverridesOneEnvVar": {
			initialEnvVars: []corev1.EnvVar{
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "0"},
			},
			expectedEnvVars: []corev1.EnvVar{
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "0"}, // User value preserved
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
				{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "8"},
			},
			description: "Should preserve user-defined HF env var and add missing ones",
		},
		"UserDefinesWithValueFrom": {
			initialEnvVars: []corev1.EnvVar{
				{
					Name: "HF_HUB_ENABLE_HF_TRANSFER",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "hf-config"},
							Key:                  "transfer-enabled",
						},
					},
				},
			},
			expectedEnvVars: []corev1.EnvVar{
				{
					Name: "HF_HUB_ENABLE_HF_TRANSFER",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "hf-config"},
							Key:                  "transfer-enabled",
						},
					},
				},
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
				{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "8"},
			},
			description: "Should preserve user-defined HF env var with ValueFrom",
		},
		"UserDefinesAllEnvVars": {
			initialEnvVars: []corev1.EnvVar{
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "0"},
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "0"},
				{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "4"},
			},
			expectedEnvVars: []corev1.EnvVar{
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "0"},
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "0"},
				{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "4"},
			},
			description: "Should not add any env vars when user defines all of them",
		},
		"ContainerWithOtherEnvVars": {
			initialEnvVars: []corev1.EnvVar{
				{Name: "SOME_OTHER_VAR", Value: "value"},
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "0"},
			},
			expectedEnvVars: []corev1.EnvVar{
				{Name: "SOME_OTHER_VAR", Value: "value"},
				{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "0"},
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
				{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "8"},
			},
			description: "Should add missing HF env vars without affecting other env vars",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			container := &corev1.Container{
				Env: make([]corev1.EnvVar, len(scenario.initialEnvVars)),
			}
			copy(container.Env, scenario.initialEnvVars)

			AddDefaultHuggingFaceEnvVars(container)

			// Verify the number of env vars
			g.Expect(container.Env).To(gomega.HaveLen(len(scenario.expectedEnvVars)), scenario.description)

			// Verify each expected env var exists with correct value
			for _, expectedEnv := range scenario.expectedEnvVars {
				found := false
				for _, actualEnv := range container.Env {
					if actualEnv.Name == expectedEnv.Name {
						found = true
						if expectedEnv.Value != "" {
							g.Expect(actualEnv.Value).To(gomega.Equal(expectedEnv.Value),
								"Env var %s should have value %s", expectedEnv.Name, expectedEnv.Value)
							g.Expect(actualEnv.ValueFrom).To(gomega.BeNil(),
								"Env var %s should not have ValueFrom when Value is set", expectedEnv.Name)
						} else if expectedEnv.ValueFrom != nil {
							g.Expect(actualEnv.ValueFrom).To(gomega.Equal(expectedEnv.ValueFrom),
								"Env var %s should have correct ValueFrom", expectedEnv.Name)
							g.Expect(actualEnv.Value).To(gomega.BeEmpty(),
								"Env var %s should not have Value when ValueFrom is set", expectedEnv.Name)
						}
						break
					}
				}
				g.Expect(found).To(gomega.BeTrue(), "Expected env var %s not found", expectedEnv.Name)
			}
		})
	}
}
