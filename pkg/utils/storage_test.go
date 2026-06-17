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

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/types"
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
	g := gomega.NewGomegaWithT(t)

	t.Run("Index 0 returns original constants", func(t *testing.T) {
		sidecar, init, volume := ModelcarNames(0)
		g.Expect(sidecar).To(gomega.Equal(constants.ModelcarContainerName))
		g.Expect(init).To(gomega.Equal(constants.ModelcarInitContainerName))
		g.Expect(volume).To(gomega.Equal(constants.StorageInitializerVolumeName))
	})

	t.Run("Index 1 returns suffixed names", func(t *testing.T) {
		sidecar, init, volume := ModelcarNames(1)
		g.Expect(sidecar).To(gomega.Equal(constants.ModelcarContainerName + "-1"))
		g.Expect(init).To(gomega.Equal(constants.ModelcarInitContainerName + "-1"))
		g.Expect(volume).To(gomega.Equal(constants.StorageInitializerVolumeName + "-1"))
	})

	t.Run("Index 2 returns suffixed names", func(t *testing.T) {
		sidecar, init, volume := ModelcarNames(2)
		g.Expect(sidecar).To(gomega.Equal(constants.ModelcarContainerName + "-2"))
		g.Expect(init).To(gomega.Equal(constants.ModelcarInitContainerName + "-2"))
		g.Expect(volume).To(gomega.Equal(constants.StorageInitializerVolumeName + "-2"))
	})
}

func TestConfigureModelcarToContainerMultipleOCI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
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
		g.Expect(err).ToNot(gomega.HaveOccurred())

		// Sidecar container should use the original constant name
		modelcar := GetContainerWithName(podSpec, constants.ModelcarContainerName)
		g.Expect(modelcar).ToNot(gomega.BeNil(), "modelcar sidecar should be present")
		g.Expect(modelcar.Image).To(gomega.Equal("registry.example.com/model-a:v1"))

		// Init container should use the original constant name
		g.Expect(podSpec.InitContainers).To(gomega.HaveLen(1))
		g.Expect(podSpec.InitContainers[0].Name).To(gomega.Equal(constants.ModelcarInitContainerName))

		// Volume should use the original constant name
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(1))
		g.Expect(podSpec.Volumes[0].Name).To(gomega.Equal(constants.StorageInitializerVolumeName))

		// ShareProcessNamespace should be enabled
		g.Expect(podSpec.ShareProcessNamespace).ToNot(gomega.BeNil())
		g.Expect(*podSpec.ShareProcessNamespace).To(gomega.BeTrue())
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
		g.Expect(err).ToNot(gomega.HaveOccurred())

		// Second OCI URI (index 1) — uses suffixed names
		err = ConfigureModelcarToContainer(
			"oci://registry.example.com/model-b:v2",
			podSpec,
			constants.InferenceServiceContainerName,
			"/mnt/models/model-b",
			storageConfig,
			1,
		)
		g.Expect(err).ToNot(gomega.HaveOccurred())

		// Should have 3 containers: kserve-container + modelcar + modelcar-1
		g.Expect(podSpec.Containers).To(gomega.HaveLen(3), "should have kserve-container + 2 modelcar sidecars")

		modelcarA := GetContainerWithName(podSpec, constants.ModelcarContainerName)
		g.Expect(modelcarA).ToNot(gomega.BeNil(), "first modelcar sidecar should exist")
		g.Expect(modelcarA.Image).To(gomega.Equal("registry.example.com/model-a:v1"))

		modelcarB := GetContainerWithName(podSpec, constants.ModelcarContainerName+"-1")
		g.Expect(modelcarB).ToNot(gomega.BeNil(), "second modelcar sidecar should exist")
		g.Expect(modelcarB.Image).To(gomega.Equal("registry.example.com/model-b:v2"))

		// Should have 2 init containers: modelcar-init + modelcar-init-1
		g.Expect(podSpec.InitContainers).To(gomega.HaveLen(2), "should have 2 modelcar init containers")
		g.Expect(podSpec.InitContainers[0].Name).To(gomega.Equal(constants.ModelcarInitContainerName))
		g.Expect(podSpec.InitContainers[1].Name).To(gomega.Equal(constants.ModelcarInitContainerName + "-1"))

		// Should have 2 volumes with distinct names
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(2), "should have 2 emptyDir volumes")
		g.Expect(podSpec.Volumes[0].Name).To(gomega.Equal(constants.StorageInitializerVolumeName))
		g.Expect(podSpec.Volumes[1].Name).To(gomega.Equal(constants.StorageInitializerVolumeName + "-1"))

		// Verify the kserve-container has volume mounts for BOTH volumes
		kserveContainer := GetContainerWithName(podSpec, constants.InferenceServiceContainerName)
		g.Expect(kserveContainer).ToNot(gomega.BeNil())
		g.Expect(kserveContainer.VolumeMounts).To(gomega.HaveLen(2), "kserve-container should have mounts for both volumes")

		mountNames := make([]string, 0, 2)
		for _, m := range kserveContainer.VolumeMounts {
			mountNames = append(mountNames, m.Name)
		}
		g.Expect(mountNames).To(gomega.ContainElement(constants.StorageInitializerVolumeName))
		g.Expect(mountNames).To(gomega.ContainElement(constants.StorageInitializerVolumeName + "-1"))

		// Each modelcar sidecar should reference its own volume
		g.Expect(modelcarA.VolumeMounts[0].Name).To(gomega.Equal(constants.StorageInitializerVolumeName))
		g.Expect(modelcarB.VolumeMounts[0].Name).To(gomega.Equal(constants.StorageInitializerVolumeName + "-1"))
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
			g.Expect(err).ToNot(gomega.HaveOccurred())
		}

		// 1 user container + 3 modelcar sidecars
		g.Expect(podSpec.Containers).To(gomega.HaveLen(4))
		g.Expect(podSpec.InitContainers).To(gomega.HaveLen(3))
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(3))

		// Verify naming: modelcar, modelcar-1, modelcar-2
		g.Expect(GetContainerWithName(podSpec, "modelcar")).ToNot(gomega.BeNil())
		g.Expect(GetContainerWithName(podSpec, "modelcar-1")).ToNot(gomega.BeNil())
		g.Expect(GetContainerWithName(podSpec, "modelcar-2")).ToNot(gomega.BeNil())
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
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("no container found with name"))
	})

	t.Run("Idempotent - calling twice with same index does not duplicate", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		}

		for range 2 {
			err := ConfigureModelcarToContainer(
				"oci://registry.example.com/model:v1",
				podSpec,
				constants.InferenceServiceContainerName,
				constants.DefaultModelLocalMountPath,
				storageConfig,
				0,
			)
			g.Expect(err).ToNot(gomega.HaveOccurred())
		}

		// Should still only have 1 modelcar sidecar and 1 init container
		g.Expect(podSpec.Containers).To(gomega.HaveLen(2)) // kserve-container + 1 modelcar
		g.Expect(podSpec.InitContainers).To(gomega.HaveLen(1))
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(1))
	})
}

func TestValidateOCIMountPaths(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("Single path is always valid", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{"/mnt/models"}, types.OciModelModeModelcar)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Empty paths are valid", func(t *testing.T) {
		err := ValidateOCIMountPaths(nil, types.OciModelModeModelcar)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Distinct parent directories are valid", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/model-a/data",
			"/mnt/model-b/data",
		}, types.OciModelModeModelcar)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Shared parent directory is rejected", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/models/model-a",
			"/mnt/models/model-b",
		}, types.OciModelModeModelcar)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("volume mount shadowing"))
		g.Expect(err.Error()).To(gomega.ContainSubstring("/mnt/models"))
	})

	t.Run("Three paths with collision on second pair", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/alpha/data",
			"/mnt/beta/model-x",
			"/mnt/beta/model-y", // collides with previous
		}, types.OciModelModeModelcar)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("/mnt/beta"))
	})

	t.Run("Default mount path used twice is rejected", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			constants.DefaultModelLocalMountPath,
			constants.DefaultModelLocalMountPath,
		}, types.OciModelModeModelcar)
		g.Expect(err).To(gomega.HaveOccurred())
	})

	// Native mode: siblings under the same parent are fine; only exact duplicates fail.
	t.Run("Native mode allows shared parent directory", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/models/adapter-a",
			"/mnt/models/adapter-b",
		}, types.OciModelModeNative)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Native mode rejects exact path duplicate", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/models/adapter-a",
			"/mnt/models/adapter-a",
		}, types.OciModelModeNative)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("unique mount path"))
	})

	t.Run("Native mode allows DefaultModelLocalMountPath used once with sibling", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			constants.DefaultModelLocalMountPath,
			constants.DefaultModelLocalMountPath + "/adapter",
		}, types.OciModelModeNative)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})
}

func TestMaxOCISourcesPerPod(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// Ensure the constant is reasonable and matches documented limit
	g.Expect(MaxOCISourcesPerPod).To(gomega.Equal(10),
		"MaxOCISourcesPerPod should be 10 — each URI adds 2 containers + 1 volume")
}

func TestParseOciScheme(t *testing.T) {
	cases := []struct {
		uri       string
		wantMode  string
		wantNorm  string
		wantIsOci bool
	}{
		{
			uri:       "oci+native://registry.io/image:tag",
			wantMode:  "native",
			wantNorm:  "oci://registry.io/image:tag",
			wantIsOci: true,
		},
		{
			uri:       "oci+modelcar://registry.io/image:tag",
			wantMode:  "modelcar",
			wantNorm:  "oci://registry.io/image:tag",
			wantIsOci: true,
		},
		{
			uri:       "oci+fetch://registry.io/image:tag",
			wantMode:  "fetch",
			wantNorm:  "oci://registry.io/image:tag",
			wantIsOci: true,
		},
		{
			uri:       "oci://registry.io/image:tag",
			wantMode:  "",
			wantNorm:  "oci://registry.io/image:tag",
			wantIsOci: true,
		},
		{
			uri:       "s3://bucket/path/to/model",
			wantMode:  "",
			wantNorm:  "s3://bucket/path/to/model",
			wantIsOci: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			gotMode, gotNorm, gotIsOci := ParseOciScheme(tc.uri)
			g.Expect(gotMode).To(gomega.Equal(tc.wantMode), "mode")
			g.Expect(gotNorm).To(gomega.Equal(tc.wantNorm), "normalizedURI")
			g.Expect(gotIsOci).To(gomega.Equal(tc.wantIsOci), "isOci")
		})
	}
}

func TestConfigureOciNativeToContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cfg := &types.StorageInitializerConfig{}

	t.Run("happy path: ImageVolume and VolumeMount added", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "kserve-container"},
			},
		}
		err := ConfigureOciNativeToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			podSpec, "kserve-container",
			constants.DefaultModelLocalMountPath, cfg,
		)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(1))
		g.Expect(podSpec.Volumes[0].VolumeSource.Image).ToNot(gomega.BeNil())
		g.Expect(podSpec.Volumes[0].VolumeSource.Image.Reference).To(gomega.Equal("registry.io/mymodel:v1"))
		g.Expect(podSpec.Containers[0].VolumeMounts).To(gomega.HaveLen(1))
		g.Expect(podSpec.Containers[0].VolumeMounts[0].ReadOnly).To(gomega.BeTrue())
		g.Expect(podSpec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal(constants.DefaultModelLocalMountPath))
	})

	t.Run("idempotent: second call with same path is a no-op", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "kserve-container"},
			},
		}
		uri := constants.OciURIPrefix + "registry.io/mymodel:v1"
		_ = ConfigureOciNativeToContainer(uri, podSpec, "kserve-container", constants.DefaultModelLocalMountPath, cfg)
		err := ConfigureOciNativeToContainer(uri, podSpec, "kserve-container", constants.DefaultModelLocalMountPath, cfg)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(1), "volume should not be duplicated")
	})

	t.Run("multi-adapter: two adapters at different paths coexist", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "kserve-container"},
			},
		}
		err1 := ConfigureOciNativeToContainer(
			constants.OciURIPrefix+"registry.io/adapter-a:v1",
			podSpec, "kserve-container", "/mnt/models/adapter-a", cfg,
		)
		err2 := ConfigureOciNativeToContainer(
			constants.OciURIPrefix+"registry.io/adapter-b:v1",
			podSpec, "kserve-container", "/mnt/models/adapter-b", cfg,
		)
		g.Expect(err1).ToNot(gomega.HaveOccurred())
		g.Expect(err2).ToNot(gomega.HaveOccurred())
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(2))
		g.Expect(podSpec.Containers[0].VolumeMounts).To(gomega.HaveLen(2))
	})

	t.Run("mountPath collision: different volume already claims modelPath", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "kserve-container",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "other-volume", MountPath: constants.DefaultModelLocalMountPath},
					},
				},
			},
		}
		err := ConfigureOciNativeToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			podSpec, "kserve-container", constants.DefaultModelLocalMountPath, cfg,
		)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("already used by volume"))
	})

	t.Run("missing container: returns error", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "some-other-container"},
			},
		}
		err := ConfigureOciNativeToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			podSpec, "kserve-container", constants.DefaultModelLocalMountPath, cfg,
		)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("no container found"))
	})

	t.Run("multi-container: same image volume mounted on two target containers", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
				{Name: constants.TransformerContainerName},
			},
		}
		uri := constants.OciURIPrefix + "registry.io/mymodel:v1"
		err1 := ConfigureOciNativeToContainer(uri, podSpec, constants.InferenceServiceContainerName, constants.DefaultModelLocalMountPath, cfg)
		err2 := ConfigureOciNativeToContainer(uri, podSpec, constants.TransformerContainerName, constants.DefaultModelLocalMountPath, cfg)
		g.Expect(err1).ToNot(gomega.HaveOccurred())
		g.Expect(err2).ToNot(gomega.HaveOccurred())
		g.Expect(podSpec.Volumes).To(gomega.HaveLen(1), "shared volume should appear exactly once")
		g.Expect(podSpec.Containers[0].VolumeMounts).To(gomega.HaveLen(1), "kserve-container should have the mount")
		g.Expect(podSpec.Containers[1].VolumeMounts).To(gomega.HaveLen(1), "transformer should have the mount")
		g.Expect(podSpec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal(constants.DefaultModelLocalMountPath))
		g.Expect(podSpec.Containers[1].VolumeMounts[0].MountPath).To(gomega.Equal(constants.DefaultModelLocalMountPath))
	})

	_ = g // suppress unused warning from outer scope
}
