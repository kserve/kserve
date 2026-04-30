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

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestModelcarNames(t *testing.T) {
	t.Run("Index 0 returns original constants", func(t *testing.T) {
		sidecar, init, volume := ModelcarNames(0)
		assert.Equal(t, constants.ModelcarContainerName, sidecar)
		assert.Equal(t, constants.ModelcarInitContainerName, init)
		assert.Equal(t, constants.StorageInitializerVolumeName, volume)
	})

	t.Run("Index 1 returns suffixed names", func(t *testing.T) {
		sidecar, init, volume := ModelcarNames(1)
		assert.Equal(t, constants.ModelcarContainerName+"-1", sidecar)
		assert.Equal(t, constants.ModelcarInitContainerName+"-1", init)
		assert.Equal(t, constants.StorageInitializerVolumeName+"-1", volume)
	})

	t.Run("Index 2 returns suffixed names", func(t *testing.T) {
		sidecar, init, volume := ModelcarNames(2)
		assert.Equal(t, constants.ModelcarContainerName+"-2", sidecar)
		assert.Equal(t, constants.ModelcarInitContainerName+"-2", init)
		assert.Equal(t, constants.StorageInitializerVolumeName+"-2", volume)
	})
}

func TestConfigureModelcarToContainerMultipleOCI(t *testing.T) {
	storageConfig := &types.StorageInitializerConfig{}

	t.Run("Single OCI URI produces original container names", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		}

		err := ConfigureModelcarToContainer(
			"oci://registry.example.com/model-a:v1",
			podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			storageConfig,
			0,
		)
		require.NoError(t, err)

		// Sidecar container should use the original constant name
		modelcar := GetContainerWithName(podSpec, constants.ModelcarContainerName)
		assert.NotNil(t, modelcar, "modelcar sidecar should be present")
		assert.Equal(t, "registry.example.com/model-a:v1", modelcar.Image)

		// Init container should use the original constant name
		assert.Len(t, podSpec.InitContainers, 1)
		assert.Equal(t, constants.ModelcarInitContainerName, podSpec.InitContainers[0].Name)

		// Volume should use the original constant name
		assert.Len(t, podSpec.Volumes, 1)
		assert.Equal(t, constants.StorageInitializerVolumeName, podSpec.Volumes[0].Name)

		// ShareProcessNamespace should be enabled
		require.NotNil(t, podSpec.ShareProcessNamespace)
		assert.True(t, *podSpec.ShareProcessNamespace)
	})

	t.Run("Two OCI URIs produce uniquely named sidecars", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		}

		// First OCI URI (index 0) — uses original names
		err := ConfigureModelcarToContainer(
			"oci://registry.example.com/model-a:v1",
			podSpec,
			constants.InferenceServiceContainerName,
			"/mnt/models/model-a",
			storageConfig,
			0,
		)
		require.NoError(t, err)

		// Second OCI URI (index 1) — uses suffixed names
		err = ConfigureModelcarToContainer(
			"oci://registry.example.com/model-b:v2",
			podSpec,
			constants.InferenceServiceContainerName,
			"/mnt/models/model-b",
			storageConfig,
			1,
		)
		require.NoError(t, err)

		// Should have 3 containers: kserve-container + modelcar + modelcar-1
		assert.Len(t, podSpec.Containers, 3, "should have kserve-container + 2 modelcar sidecars")

		modelcarA := GetContainerWithName(podSpec, constants.ModelcarContainerName)
		assert.NotNil(t, modelcarA, "first modelcar sidecar should exist")
		assert.Equal(t, "registry.example.com/model-a:v1", modelcarA.Image)

		modelcarB := GetContainerWithName(podSpec, constants.ModelcarContainerName+"-1")
		assert.NotNil(t, modelcarB, "second modelcar sidecar should exist")
		assert.Equal(t, "registry.example.com/model-b:v2", modelcarB.Image)

		// Should have 2 init containers: modelcar-init + modelcar-init-1
		assert.Len(t, podSpec.InitContainers, 2, "should have 2 modelcar init containers")
		assert.Equal(t, constants.ModelcarInitContainerName, podSpec.InitContainers[0].Name)
		assert.Equal(t, constants.ModelcarInitContainerName+"-1", podSpec.InitContainers[1].Name)

		// Should have 2 volumes with distinct names
		assert.Len(t, podSpec.Volumes, 2, "should have 2 emptyDir volumes")
		assert.Equal(t, constants.StorageInitializerVolumeName, podSpec.Volumes[0].Name)
		assert.Equal(t, constants.StorageInitializerVolumeName+"-1", podSpec.Volumes[1].Name)

		// Verify the kserve-container has volume mounts for BOTH volumes
		kserveContainer := GetContainerWithName(podSpec, constants.InferenceServiceContainerName)
		require.NotNil(t, kserveContainer)
		assert.Len(t, kserveContainer.VolumeMounts, 2, "kserve-container should have mounts for both volumes")

		mountNames := make([]string, 0, 2)
		for _, m := range kserveContainer.VolumeMounts {
			mountNames = append(mountNames, m.Name)
		}
		assert.Contains(t, mountNames, constants.StorageInitializerVolumeName)
		assert.Contains(t, mountNames, constants.StorageInitializerVolumeName+"-1")

		// Each modelcar sidecar should reference its own volume
		assert.Equal(t, constants.StorageInitializerVolumeName, modelcarA.VolumeMounts[0].Name)
		assert.Equal(t, constants.StorageInitializerVolumeName+"-1", modelcarB.VolumeMounts[0].Name)
	})

	t.Run("Three OCI URIs produce three distinct sidecars", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		}

		uris := []string{
			"oci://registry.example.com/base-model:v1",
			"oci://registry.example.com/adapter-a:v1",
			"oci://registry.example.com/adapter-b:v1",
		}
		paths := []string{
			"/mnt/models/base",
			"/mnt/models/adapter-a",
			"/mnt/models/adapter-b",
		}

		for i, uri := range uris {
			err := ConfigureModelcarToContainer(uri, podSpec, constants.InferenceServiceContainerName, paths[i], storageConfig, i)
			require.NoError(t, err)
		}

		// 1 user container + 3 modelcar sidecars
		assert.Len(t, podSpec.Containers, 4)
		assert.Len(t, podSpec.InitContainers, 3)
		assert.Len(t, podSpec.Volumes, 3)

		// Verify naming: modelcar, modelcar-1, modelcar-2
		assert.NotNil(t, GetContainerWithName(podSpec, "modelcar"))
		assert.NotNil(t, GetContainerWithName(podSpec, "modelcar-1"))
		assert.NotNil(t, GetContainerWithName(podSpec, "modelcar-2"))
	})

	t.Run("Error when target container not found", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "some-other-container"},
			},
		}

		err := ConfigureModelcarToContainer(
			"oci://registry.example.com/model:v1",
			podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			storageConfig,
			0,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no container found with name")
	})

	t.Run("Idempotent - calling twice with same index does not duplicate", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		}

		for i := 0; i < 2; i++ {
			err := ConfigureModelcarToContainer(
				"oci://registry.example.com/model:v1",
				podSpec,
				constants.InferenceServiceContainerName,
				constants.DefaultModelLocalMountPath,
				storageConfig,
				0,
			)
			require.NoError(t, err)
		}

		// Should still only have 1 modelcar sidecar and 1 init container
		assert.Len(t, podSpec.Containers, 2) // kserve-container + 1 modelcar
		assert.Len(t, podSpec.InitContainers, 1)
		assert.Len(t, podSpec.Volumes, 1)
	})
}

func TestValidateOCIMountPaths(t *testing.T) {
	t.Run("Single path is always valid", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{"/mnt/models"})
		assert.NoError(t, err)
	})

	t.Run("Empty paths are valid", func(t *testing.T) {
		err := ValidateOCIMountPaths(nil)
		assert.NoError(t, err)
	})

	t.Run("Distinct parent directories are valid", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/model-a/data",
			"/mnt/model-b/data",
		})
		assert.NoError(t, err)
	})

	t.Run("Shared parent directory is rejected", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/models/model-a",
			"/mnt/models/model-b",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "volume mount shadowing")
		assert.Contains(t, err.Error(), "/mnt/models")
	})

	t.Run("Three paths with collision on second pair", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/alpha/data",
			"/mnt/beta/model-x",
			"/mnt/beta/model-y", // collides with previous
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/mnt/beta")
	})

	t.Run("Default mount path used twice is rejected", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			constants.DefaultModelLocalMountPath,
			constants.DefaultModelLocalMountPath,
		})
		require.Error(t, err)
	})
}

func TestMaxOCISourcesPerPod(t *testing.T) {
	// Ensure the constant is reasonable and matches documented limit
	assert.Equal(t, 10, MaxOCISourcesPerPod,
		"MaxOCISourcesPerPod should be 10 — each URI adds 2 containers + 1 volume")
}
