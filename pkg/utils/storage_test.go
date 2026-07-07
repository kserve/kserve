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
	"k8s.io/apimachinery/pkg/api/resource"

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
		err := ValidateOCIMountPaths([]string{"/mnt/models"})
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Empty paths are valid", func(t *testing.T) {
		err := ValidateOCIMountPaths(nil)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Distinct parent directories are valid", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/model-a/data",
			"/mnt/model-b/data",
		})
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("Shared parent directory is rejected", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/models/model-a",
			"/mnt/models/model-b",
		})
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("volume mount shadowing"))
		g.Expect(err.Error()).To(gomega.ContainSubstring("/mnt/models"))
	})

	t.Run("Three paths with collision on second pair", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			"/mnt/alpha/data",
			"/mnt/beta/model-x",
			"/mnt/beta/model-y", // collides with previous
		})
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("/mnt/beta"))
	})

	t.Run("Default mount path used twice is rejected", func(t *testing.T) {
		err := ValidateOCIMountPaths([]string{
			constants.DefaultModelLocalMountPath,
			constants.DefaultModelLocalMountPath,
		})
		g.Expect(err).To(gomega.HaveOccurred())
	})
}

func TestMaxOCISourcesPerPod(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// Ensure the constant is reasonable and matches documented limit
	g.Expect(MaxOCISourcesPerPod).To(gomega.Equal(10),
		"MaxOCISourcesPerPod should be 10 — each URI adds 2 containers + 1 volume")
}

// TestCreateModelcarContainerFromCSC verifies that the CSC-derived modelcar
// helper copies operator-supplied container fields (env, envFrom, resources,
// securityContext, imagePullPolicy, lifecycle) verbatim while hard-setting
// the modelcar-owned fields (name, image, args) and prepending the mandatory
// sidecar volume mount.
func TestCreateModelcarContainerFromCSC(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	cscUID := int64(1234)
	csc := &corev1.Container{
		Name:            "cluster-storage-container-name", // must be overridden
		Image:           "cluster-storage-container-image", // must be overridden
		Command:         []string{"ignored"},
		Args:            []string{"csc-arg-must-be-overridden"},
		ImagePullPolicy: corev1.PullAlways,
		Env: []corev1.EnvVar{
			{Name: "FOO", Value: "bar"},
			{Name: "SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret"},
				Key:                  "key",
			}}},
		},
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm"}}},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
		},
		SecurityContext: &corev1.SecurityContext{RunAsUser: &cscUID},
		Lifecycle: &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"echo", "started"}}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "extra-mount", MountPath: "/extra"},
		},
	}

	sidecarName := "modelcar"
	image := "ghcr.io/example/llama:v1"
	modelPath := constants.DefaultModelLocalMountPath
	volumeName := constants.StorageInitializerVolumeName

	c := CreateModelcarContainerFromCSC(sidecarName, image, modelPath, volumeName, csc)

	// Modelcar-owned fields — always overridden by code.
	g.Expect(c.Name).To(gomega.Equal(sidecarName))
	g.Expect(c.Image).To(gomega.Equal(image))
	g.Expect(c.Args).To(gomega.Equal([]string{"sh", "-c", modelcarCommand(modelPath)}))

	// The sidecar's mandatory volume mount is appended alongside any CSC-supplied mounts.
	g.Expect(c.VolumeMounts).To(gomega.ContainElements(
		corev1.VolumeMount{Name: "extra-mount", MountPath: "/extra"},
		corev1.VolumeMount{Name: volumeName, MountPath: GetParentDirectory(modelPath), ReadOnly: false},
	))

	// Everything else must flow through from the CSC.
	g.Expect(c.ImagePullPolicy).To(gomega.Equal(corev1.PullAlways))
	g.Expect(c.Env).To(gomega.Equal(csc.Env))
	g.Expect(c.EnvFrom).To(gomega.Equal(csc.EnvFrom))
	g.Expect(c.Resources).To(gomega.Equal(csc.Resources))
	g.Expect(c.SecurityContext).To(gomega.Equal(csc.SecurityContext))
	g.Expect(c.Lifecycle).To(gomega.Equal(csc.Lifecycle))
	g.Expect(c.TerminationMessagePolicy).To(gomega.Equal(corev1.TerminationMessageFallbackToLogsOnError))
}

// TestCreateModelcarContainerFromCSC_NilCSC verifies that the helper is
// nil-safe: callers that reach the modelcar path without a matching CSC still
// get a functional sidecar with modelcar-owned fields set.
func TestCreateModelcarContainerFromCSC_NilCSC(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	c := CreateModelcarContainerFromCSC("modelcar", "img", constants.DefaultModelLocalMountPath, "vol", nil)

	g.Expect(c.Name).To(gomega.Equal("modelcar"))
	g.Expect(c.Image).To(gomega.Equal("img"))
	g.Expect(c.Args).To(gomega.Equal([]string{"sh", "-c", modelcarCommand(constants.DefaultModelLocalMountPath)}))
	g.Expect(c.VolumeMounts).To(gomega.HaveLen(1))
	g.Expect(c.VolumeMounts[0].Name).To(gomega.Equal("vol"))
	g.Expect(c.TerminationMessagePolicy).To(gomega.Equal(corev1.TerminationMessageFallbackToLogsOnError))
}

// TestCreateModelcarInitContainerFromCSC verifies the init-container variant
// applies the same CSC-verbatim + modelcar-owned override policy.
func TestCreateModelcarInitContainerFromCSC(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	csc := &corev1.Container{
		Name:  "cluster-storage-container-name",
		Image: "cluster-storage-container-image",
		Env:   []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m")},
		},
	}

	image := "ghcr.io/example/llama:v1"
	c := CreateModelcarInitContainerFromCSC("modelcar-init", image, csc)

	g.Expect(c.Name).To(gomega.Equal("modelcar-init"))
	g.Expect(c.Image).To(gomega.Equal(image))
	g.Expect(c.Args).To(gomega.Equal([]string{"sh", "-c", modelcarInitCommand(image)}))
	g.Expect(c.Env).To(gomega.Equal(csc.Env))
	g.Expect(c.Resources).To(gomega.Equal(csc.Resources))
	// No sidecar volume mount on the init container.
	g.Expect(c.VolumeMounts).To(gomega.BeEmpty())
}
