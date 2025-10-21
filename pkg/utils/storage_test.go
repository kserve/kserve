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
	"k8s.io/utils/ptr"

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

func TestParsePvcURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		srcURI         string
		expectedPvcName string
		expectedPvcPath string
		expectError    bool
	}{
		"PvcWithoutPath": {
			srcURI:          "pvc://myclaim",
			expectedPvcName: "myclaim",
			expectedPvcPath: "",
			expectError:     false,
		},
		"PvcWithPath": {
			srcURI:          "pvc://myclaim/models",
			expectedPvcName: "myclaim",
			expectedPvcPath: "models",
			expectError:     false,
		},
		"PvcWithNestedPath": {
			srcURI:          "pvc://myclaim/models/v1",
			expectedPvcName: "myclaim",
			expectedPvcPath: "models/v1",
			expectError:     false,
		},
		"NonPvcURI": {
			srcURI:          "s3://bucket/path",
			expectedPvcName: "s3:",
			expectedPvcPath: "/bucket/path",
			expectError:     false,
		},
		"EmptyString": {
			srcURI:          "",
			expectedPvcName: "",
			expectedPvcPath: "",
			expectError:     false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			pvcName, pvcPath, err := ParsePvcURI(scenario.srcURI)
			g.Expect(pvcName).To(gomega.Equal(scenario.expectedPvcName))
			g.Expect(pvcPath).To(gomega.Equal(scenario.expectedPvcPath))
			if scenario.expectError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestAddEmptyDirVolumeIfNotPresent(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		initialVolumes   []corev1.Volume
		volumeName       string
		expectedVolumes  []corev1.Volume
	}{
		"AddVolumeToEmptyList": {
			initialVolumes: []corev1.Volume{},
			volumeName:     "test-volume",
			expectedVolumes: []corev1.Volume{
				{
					Name: "test-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
		"AddVolumeToExistingList": {
			initialVolumes: []corev1.Volume{
				{
					Name: "existing-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			volumeName: "test-volume",
			expectedVolumes: []corev1.Volume{
				{
					Name: "existing-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "test-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
		"VolumeAlreadyExists": {
			initialVolumes: []corev1.Volume{
				{
					Name: "test-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			volumeName: "test-volume",
			expectedVolumes: []corev1.Volume{
				{
					Name: "test-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			podSpec := &corev1.PodSpec{
				Volumes: scenario.initialVolumes,
			}
			AddEmptyDirVolumeIfNotPresent(podSpec, scenario.volumeName)
			g.Expect(podSpec.Volumes).To(gomega.Equal(scenario.expectedVolumes))
		})
	}
}

func TestGetStorageResources(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		storageURIs           []string
		storagePaths          []string
		expectedVolumeMounts  []corev1.VolumeMount
		expectedVolumes       []corev1.Volume
		expectedInitArgs      []string
	}{
		"SingleStorage": {
			storageURIs:  []string{"s3://bucket/model"},
			storagePaths: []string{"/mnt/models"},
			expectedVolumeMounts: []corev1.VolumeMount{
				{
					Name:      "mnt-models",
					MountPath: "/mnt/models",
					ReadOnly:  false,
				},
			},
			expectedVolumes: []corev1.Volume{
				{
					Name: "mnt-models",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			expectedInitArgs: []string{"s3://bucket/model", "/mnt/models"},
		},
		"MultipleStorageCommonPath": {
			storageURIs:  []string{"s3://bucket/model1", "s3://bucket/model2"},
			storagePaths: []string{"/mnt/models/model1", "/mnt/models/model2"},
			expectedVolumeMounts: []corev1.VolumeMount{
				{
					Name:      "mnt-models",
					MountPath: "/mnt/models",
					ReadOnly:  false,
				},
			},
			expectedVolumes: []corev1.Volume{
				{
					Name: "mnt-models",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			expectedInitArgs: []string{"s3://bucket/model1", "/mnt/models/model1", "s3://bucket/model2", "/mnt/models/model2"},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			volumeMounts, volumes, initArgs, err := GetStorageResources(scenario.storageURIs, scenario.storagePaths)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(volumeMounts).To(gomega.Equal(scenario.expectedVolumeMounts))
			g.Expect(volumes).To(gomega.Equal(scenario.expectedVolumes))
			g.Expect(initArgs).To(gomega.Equal(scenario.expectedInitArgs))
		})
	}
}

func TestAddModelMount(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		initialPodSpec     *corev1.PodSpec
		mountParams        StorageMountParams
		containerName      string
		expectedVolumes    []corev1.Volume
		expectedMountAdded bool
	}{
		"AddPvcMountToRegularContainer": {
			initialPodSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "test-container"},
				},
				Volumes: []corev1.Volume{},
			},
			mountParams: StorageMountParams{
				MountPath:  "/mnt/models",
				SubPath:    "model1",
				VolumeName: "model-volume",
				PVCName:    "test-pvc",
				ReadOnly:   true,
			},
			containerName: "test-container",
			expectedVolumes: []corev1.Volume{
				{
					Name: "model-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
			expectedMountAdded: true,
		},
		"AddEmptyDirMountToInitContainer": {
			initialPodSpec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init-container"},
				},
				Volumes: []corev1.Volume{},
			},
			mountParams: StorageMountParams{
				MountPath:  "/mnt/models",
				VolumeName: "model-volume",
				PVCName:    "", // Empty means EmptyDir
				ReadOnly:   true,
			},
			containerName: "init-container",
			expectedVolumes: []corev1.Volume{
				{
					Name: "model-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			expectedMountAdded: true,
		},
		"ContainerNotFound": {
			initialPodSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "other-container"},
				},
				Volumes: []corev1.Volume{},
			},
			mountParams: StorageMountParams{
				MountPath:  "/mnt/models",
				VolumeName: "model-volume",
				PVCName:    "test-pvc",
				ReadOnly:   true,
			},
			containerName:      "nonexistent-container",
			expectedVolumes:    []corev1.Volume{},
			expectedMountAdded: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := AddModelMount(scenario.mountParams, scenario.containerName, scenario.initialPodSpec)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(scenario.initialPodSpec.Volumes).To(gomega.Equal(scenario.expectedVolumes))

			// Check if the mount was added to the correct container
			if scenario.expectedMountAdded {
				var found bool
				// Check regular containers
				for _, container := range scenario.initialPodSpec.Containers {
					if container.Name == scenario.containerName {
						for _, mount := range container.VolumeMounts {
							if mount.Name == scenario.mountParams.VolumeName {
								found = true
								g.Expect(mount.MountPath).To(gomega.Equal(scenario.mountParams.MountPath))
								g.Expect(mount.SubPath).To(gomega.Equal(scenario.mountParams.SubPath))
								break
							}
						}
					}
				}
				// Check init containers
				for _, container := range scenario.initialPodSpec.InitContainers {
					if container.Name == scenario.containerName {
						for _, mount := range container.VolumeMounts {
							if mount.Name == scenario.mountParams.VolumeName {
								found = true
								g.Expect(mount.MountPath).To(gomega.Equal(scenario.mountParams.MountPath))
								g.Expect(mount.ReadOnly).To(gomega.BeFalse()) // Init containers should have ReadOnly = false
								break
							}
						}
					}
				}
				g.Expect(found).To(gomega.BeTrue())
			}
		})
	}
}

func TestCreateInitContainerWithConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		storageConfig       *types.StorageInitializerConfig
		containerArgs       []string
		expectedImage       string
		validateHuggingFace bool
		description         string
	}{
		"WithCustomImage": {
			storageConfig: &types.StorageInitializerConfig{
				Image:         "custom-image:latest",
				CpuRequest:    "100m",
				CpuLimit:      "500m",
				MemoryRequest: "128Mi",
				MemoryLimit:   "512Mi",
			},
			containerArgs:       []string{"s3://bucket/model", "/mnt/models"},
			expectedImage:       "custom-image:latest",
			validateHuggingFace: true,
			description:         "Custom image with HuggingFace env vars",
		},
		"WithDefaultImage": {
			storageConfig: &types.StorageInitializerConfig{
				Image:         "", // Empty means use default
				CpuRequest:    "200m",
				CpuLimit:      "1000m",
				MemoryRequest: "256Mi",
				MemoryLimit:   "1Gi",
			},
			containerArgs:       []string{"gs://bucket/model", "/opt/models"},
			expectedImage:       constants.StorageInitializerContainerImage + ":" + constants.StorageInitializerContainerImageVersion,
			validateHuggingFace: true,
			description:         "Default image with HuggingFace env vars",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			container := CreateInitContainerWithConfig(scenario.storageConfig, scenario.containerArgs)

			// Basic container properties
			g.Expect(container.Name).To(gomega.Equal(constants.StorageInitializerContainerName))
			g.Expect(container.Image).To(gomega.Equal(scenario.expectedImage))
			g.Expect(container.Args).To(gomega.Equal(scenario.containerArgs))
			g.Expect(container.TerminationMessagePolicy).To(gomega.Equal(corev1.TerminationMessageFallbackToLogsOnError))

			// Check resources
			g.Expect(container.Resources.Requests[corev1.ResourceCPU]).To(gomega.Equal(resource.MustParse(scenario.storageConfig.CpuRequest)))
			g.Expect(container.Resources.Requests[corev1.ResourceMemory]).To(gomega.Equal(resource.MustParse(scenario.storageConfig.MemoryRequest)))
			g.Expect(container.Resources.Limits[corev1.ResourceCPU]).To(gomega.Equal(resource.MustParse(scenario.storageConfig.CpuLimit)))
			g.Expect(container.Resources.Limits[corev1.ResourceMemory]).To(gomega.Equal(resource.MustParse(scenario.storageConfig.MemoryLimit)))

			// Check volume mounts
			g.Expect(container.VolumeMounts).To(gomega.HaveLen(1))
			g.Expect(container.VolumeMounts[0].Name).To(gomega.Equal(constants.StorageInitializerVolumeName))
			g.Expect(container.VolumeMounts[0].MountPath).To(gomega.Equal(constants.DefaultModelLocalMountPath))
			g.Expect(container.VolumeMounts[0].ReadOnly).To(gomega.BeFalse())

			if scenario.validateHuggingFace {
				// Check HuggingFace environment variables - these are critical for the fix
				expectedEnvVars := map[string]string{
					"HF_HUB_ENABLE_HF_TRANSFER":        "1",
					"HF_XET_HIGH_PERFORMANCE":          "1",
					"HF_XET_NUM_CONCURRENT_RANGE_GETS": "8",
				}

				for expectedName, expectedValue := range expectedEnvVars {
					found := false
					for _, env := range container.Env {
						if env.Name == expectedName {
							found = true
							// Critical: Ensure only `value` is set, not `valueFrom`
							g.Expect(env.Value).To(gomega.Equal(expectedValue), "Expected env var %s to have value %s", expectedName, expectedValue)
							g.Expect(env.ValueFrom).To(gomega.BeNil(), "Expected env var %s to not have valueFrom set (would cause K8s validation error)", expectedName)
							break
						}
					}
					g.Expect(found).To(gomega.BeTrue(), "Expected HuggingFace environment variable %s not found", expectedName)
				}
			}
		})
	}
}

func TestCreateInitContainerWithConflictingEnvVars(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// This test specifically addresses the issue mentioned: env variables with valueFrom
	// that would conflict with the HuggingFace env vars and cause validation errors

	containerArgs := []string{"s3://bucket/model", "/mnt/models"}

	// Create a minimal container WITHOUT calling CreateInitContainerWithConfig first
	// This simulates the state before HuggingFace env vars are added
	container := &corev1.Container{
		Name:                     constants.StorageInitializerContainerName,
		Image:                    "test-image:v1",
		Args:                     containerArgs,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		VolumeMounts: []corev1.VolumeMount{{
			Name:      constants.StorageInitializerVolumeName,
			MountPath: constants.DefaultModelLocalMountPath,
			ReadOnly:  false,
		}},
	}

	// Simulate the scenario where credential injection adds env vars with valueFrom FIRST
	// This would happen if credential injection or other systems add env vars before our code runs
	preExistingEnvVars := []corev1.EnvVar{
		{
			Name: "HF_HUB_ENABLE_HF_TRANSFER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "hf-secret",
					},
					Key: "transfer",
				},
			},
		},
		{
			Name: "OTHER_ENV",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "config-map",
					},
					Key: "other",
				},
			},
		},
	}

	// Set the pre-existing env vars (this happens BEFORE our HuggingFace logic)
	container.Env = preExistingEnvVars

	// Now apply the HuggingFace env vars using the same logic as CreateInitContainerWithConfig
	huggingFaceEnvVars := []corev1.EnvVar{
		{
			Name:  "HF_HUB_ENABLE_HF_TRANSFER",
			Value: "1",
		},
		{
			Name:  "HF_XET_HIGH_PERFORMANCE",
			Value: "1",
		},
		{
			Name:  "HF_XET_NUM_CONCURRENT_RANGE_GETS",
			Value: "8",
		},
	}

	// This is the critical line that mirrors the logic in CreateInitContainerWithConfig
	container.Env = AppendEnvVarIfNotExists(container.Env, huggingFaceEnvVars...)

	// Verify that no duplicate environment variables exist
	envCount := make(map[string]int)
	for _, env := range container.Env {
		envCount[env.Name]++
	}

	for envName, count := range envCount {
		g.Expect(count).To(gomega.Equal(1), "Environment variable %s appears %d times, should appear exactly once", envName, count)
	}

	// Verify that the pre-existing HF_HUB_ENABLE_HF_TRANSFER with valueFrom is preserved
	// and the new one with value is NOT added (preventing the K8s validation error)
	hfTransferFound := false
	for _, env := range container.Env {
		if env.Name == "HF_HUB_ENABLE_HF_TRANSFER" {
			hfTransferFound = true
			// Should preserve the original valueFrom, not add the value version
			g.Expect(env.ValueFrom).ToNot(gomega.BeNil(), "Pre-existing HF_HUB_ENABLE_HF_TRANSFER with valueFrom should be preserved")
			g.Expect(env.Value).To(gomega.BeEmpty(), "Pre-existing HF_HUB_ENABLE_HF_TRANSFER should not have value set when valueFrom exists")
			break
		}
	}
	g.Expect(hfTransferFound).To(gomega.BeTrue(), "HF_HUB_ENABLE_HF_TRANSFER env var should exist")

	// Verify other HuggingFace env vars were added correctly (since they didn't conflict)
	hfPerformanceFound := false
	hfConcurrentFound := false
	for _, env := range container.Env {
		if env.Name == "HF_XET_HIGH_PERFORMANCE" {
			hfPerformanceFound = true
			g.Expect(env.Value).To(gomega.Equal("1"))
			g.Expect(env.ValueFrom).To(gomega.BeNil())
		}
		if env.Name == "HF_XET_NUM_CONCURRENT_RANGE_GETS" {
			hfConcurrentFound = true
			g.Expect(env.Value).To(gomega.Equal("8"))
			g.Expect(env.ValueFrom).To(gomega.BeNil())
		}
	}
	g.Expect(hfPerformanceFound).To(gomega.BeTrue(), "HF_XET_HIGH_PERFORMANCE should be added")
	g.Expect(hfConcurrentFound).To(gomega.BeTrue(), "HF_XET_NUM_CONCURRENT_RANGE_GETS should be added")

	// Verify OTHER_ENV is still present and unchanged
	otherEnvFound := false
	for _, env := range container.Env {
		if env.Name == "OTHER_ENV" {
			otherEnvFound = true
			g.Expect(env.ValueFrom).ToNot(gomega.BeNil(), "OTHER_ENV should preserve its valueFrom")
			break
		}
	}
	g.Expect(otherEnvFound).To(gomega.BeTrue(), "OTHER_ENV should be preserved")
}

func TestCreateModelcarContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	storageConfig := &types.StorageInitializerConfig{
		CpuModelcar:    "200m",
		MemoryModelcar: "256Mi",
		UidModelcar:    ptr.To(int64(1000)),
	}

	image := "test-modelcar:latest"
	modelPath := "/mnt/models/test"
	container := CreateModelcarContainer(image, modelPath, storageConfig)

	g.Expect(container.Name).To(gomega.Equal(constants.ModelcarContainerName))
	g.Expect(container.Image).To(gomega.Equal(image))

	// Check resources
	g.Expect(container.Resources.Requests[corev1.ResourceCPU]).To(gomega.Equal(resource.MustParse("200m")))
	g.Expect(container.Resources.Requests[corev1.ResourceMemory]).To(gomega.Equal(resource.MustParse("256Mi")))
	g.Expect(container.Resources.Limits[corev1.ResourceCPU]).To(gomega.Equal(resource.MustParse("200m")))
	g.Expect(container.Resources.Limits[corev1.ResourceMemory]).To(gomega.Equal(resource.MustParse("256Mi")))

	// Check security context
	g.Expect(container.SecurityContext).ToNot(gomega.BeNil())
	g.Expect(container.SecurityContext.RunAsUser).To(gomega.Equal(ptr.To(int64(1000))))

	// Check volume mounts
	g.Expect(container.VolumeMounts).To(gomega.HaveLen(1))
	g.Expect(container.VolumeMounts[0].Name).To(gomega.Equal(constants.StorageInitializerVolumeName))
	g.Expect(container.VolumeMounts[0].MountPath).To(gomega.Equal(GetParentDirectory(modelPath)))
	g.Expect(container.VolumeMounts[0].ReadOnly).To(gomega.BeFalse())
}

func TestCreateModelcarInitContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	storageConfig := &types.StorageInitializerConfig{
		CpuModelcar:    "100m",
		MemoryModelcar: "128Mi",
	}

	image := "test-modelcar:v1.0"
	container := CreateModelcarInitContainer(image, storageConfig)

	g.Expect(container.Name).To(gomega.Equal(constants.ModelcarInitContainerName))
	g.Expect(container.Image).To(gomega.Equal(image))

	// Check resources
	g.Expect(container.Resources.Requests[corev1.ResourceCPU]).To(gomega.Equal(resource.MustParse("100m")))
	g.Expect(container.Resources.Requests[corev1.ResourceMemory]).To(gomega.Equal(resource.MustParse("128Mi")))
	g.Expect(container.Resources.Limits[corev1.ResourceCPU]).To(gomega.Equal(resource.MustParse("100m")))
	g.Expect(container.Resources.Limits[corev1.ResourceMemory]).To(gomega.Equal(resource.MustParse("128Mi")))

	// Check args contain validation logic
	g.Expect(container.Args).To(gomega.HaveLen(3))
	g.Expect(container.Args[0]).To(gomega.Equal("sh"))
	g.Expect(container.Args[1]).To(gomega.Equal("-c"))
	g.Expect(container.Args[2]).To(gomega.ContainSubstring("Pre-fetching modelcar"))
	g.Expect(container.Args[2]).To(gomega.ContainSubstring(image))
}

func TestConfigureModelcarToContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		modelUri            string
		initialPodSpec      *corev1.PodSpec
		targetContainerName string
		storageConfig       *types.StorageInitializerConfig
		expectError         bool
		validateResult      func(*corev1.PodSpec)
	}{
		"ConfigureSuccessfully": {
			modelUri: "oci://registry.com/model:latest",
			initialPodSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "predictor"},
				},
			},
			targetContainerName: "predictor",
			storageConfig: &types.StorageInitializerConfig{
				UidModelcar: ptr.To(int64(1001)),
			},
			expectError: false,
			validateResult: func(podSpec *corev1.PodSpec) {
				// Check that process namespace sharing is enabled
				g.Expect(podSpec.ShareProcessNamespace).To(gomega.Equal(ptr.To(true)))

				// Check that modelcar container was added
				g.Expect(podSpec.Containers).To(gomega.HaveLen(2))
				var modelcarContainer *corev1.Container
				for _, container := range podSpec.Containers {
					if container.Name == constants.ModelcarContainerName {
						modelcarContainer = &container
						break
					}
				}
				g.Expect(modelcarContainer).ToNot(gomega.BeNil())
				g.Expect(modelcarContainer.Image).To(gomega.Equal("registry.com/model:latest"))

				// Check that init container was added
				g.Expect(podSpec.InitContainers).To(gomega.HaveLen(1))
				g.Expect(podSpec.InitContainers[0].Name).To(gomega.Equal(constants.ModelcarInitContainerName))

				// Check that target container has async environment variable
				var targetContainer *corev1.Container
				for _, container := range podSpec.Containers {
					if container.Name == "predictor" {
						targetContainer = &container
						break
					}
				}
				g.Expect(targetContainer).ToNot(gomega.BeNil())
				var asyncEnvFound bool
				for _, env := range targetContainer.Env {
					if env.Name == constants.ModelInitModeEnvVarKey && env.Value == "async" {
						asyncEnvFound = true
						break
					}
				}
				g.Expect(asyncEnvFound).To(gomega.BeTrue())

				// Check that security context is set
				g.Expect(targetContainer.SecurityContext).ToNot(gomega.BeNil())
				g.Expect(targetContainer.SecurityContext.RunAsUser).To(gomega.Equal(ptr.To(int64(1001))))
			},
		},
		"ContainerNotFound": {
			modelUri: "oci://registry.com/model:latest",
			initialPodSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "other-container"},
				},
			},
			targetContainerName: "nonexistent-container",
			storageConfig:       &types.StorageInitializerConfig{},
			expectError:         true,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := ConfigureModelcarToContainer(scenario.modelUri, scenario.initialPodSpec, scenario.targetContainerName, scenario.storageConfig)
			if scenario.expectError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				if scenario.validateResult != nil {
					scenario.validateResult(scenario.initialPodSpec)
				}
			}
		})
	}
}
