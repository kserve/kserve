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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/types"
)

type StorageMountParams struct {
	MountPath  string
	SubPath    string
	VolumeName string
	PVCName    string
	ReadOnly   bool
}

// ParsePvcURI parses a PVC URI of the form "pvc://<name>[/path]" into its components.
//
// Parameters:
//   - srcURI: The source URI string, which must begin with the "pvc://" prefix.
//
// Returns:
//   - pvcName: The name of the PVC (<name> part).
//   - pvcPath: The optional <path> component. If not provided, this will be an empty string.
//   - err: An error if strings.Split would return zero.
//
// The function expects the input to follow the "pvc://<name>[/path]" format, however the pvc:// prefix is not validated.
//
// Examples:
//
//	"pvc://myclaim"           => pvcName: "myclaim", pvcPath: "", err: nil
//	"pvc://myclaim/models"    => pvcName: "myclaim", pvcPath: "models", err: nil
//	"pvc://myclaim/models/v1" => pvcName: "myclaim", pvcPath: "models/v1", err: nil
//	"s3://bucket/path"        => pvcName: "s3:", pvcPath: "/bucket/path", err: nil
//	"" (empty string)         => pvcName: "", pvcPath: "", err: nil
func ParsePvcURI(srcURI string) (pvcName string, pvcPath string, err error) {
	parts := strings.Split(strings.TrimPrefix(srcURI, constants.PvcURIPrefix), "/")
	switch len(parts) {
	case 0:
		return "", "", fmt.Errorf("invalid URI must be pvc://<pvcname>/[path]: %s", srcURI)
	case 1:
		pvcName = parts[0]
		pvcPath = ""
	default:
		pvcName = parts[0]
		pvcPath = strings.Join(parts[1:], "/")
	}

	return pvcName, pvcPath, nil
}

// addVolumeMountToContainer adds a volume mount to a specific container
func addVolumeMountToContainer(container *corev1.Container, storageMountParams StorageMountParams) bool {
	// Check if mount already exists
	for _, mount := range container.VolumeMounts {
		if mount.Name == storageMountParams.VolumeName {
			return false // Mount already exists
		}
	}

	// Add the volume mount
	sourceVolumeMount := corev1.VolumeMount{
		Name:      storageMountParams.VolumeName,
		MountPath: storageMountParams.MountPath,
		SubPath:   storageMountParams.SubPath,
		ReadOnly:  storageMountParams.ReadOnly,
	}

	container.VolumeMounts = append(container.VolumeMounts, sourceVolumeMount)
	return true
}

// AddModelMount adds a mount to the specified container in the given PodSpec based on the provided modelUri.
// If the mount or volume already exists, it will not be duplicated.
//
// Parameters:
//   - modelUri: The URI specifying the PVC and optional sub-path to mount.
//   - containerName: The name of the container within the PodSpec to which the model should be mounted.
//   - readOnly: Whether the mount should be read-only.
//   - podSpec: PodSpec to modify.
//
// Returns:
//   - error: An error if the modelUri is invalid or if any other issue occurs; otherwise, nil.
func AddModelMount(storageMountParams StorageMountParams, containerName string, podSpec *corev1.PodSpec) error {
	var volumeSource corev1.VolumeSource

	if storageMountParams.PVCName != "" {
		volumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: storageMountParams.PVCName,
			},
		}
	} else {
		volumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}

	mountAdded := false

	// Check regular containers first
	for idx := range podSpec.Containers {
		if podSpec.Containers[idx].Name == containerName {
			mountAdded = addVolumeMountToContainer(&podSpec.Containers[idx], storageMountParams)
			break
		}
	}

	// If not found in regular containers, check init containers
	if !mountAdded {
		for idx := range podSpec.InitContainers {
			if podSpec.InitContainers[idx].Name == containerName {
				storageMountParams.ReadOnly = false // init containers need to write to the mount
				mountAdded = addVolumeMountToContainer(&podSpec.InitContainers[idx], storageMountParams)
				break
			}
		}
	}

	if mountAdded {
		// add the volume on the pod
		volumeExists := false
		for _, volume := range podSpec.Volumes {
			if volume.Name == storageMountParams.VolumeName {
				volumeExists = true
				break
			}
		}

		if !volumeExists {
			modelVolume := corev1.Volume{
				Name:         storageMountParams.VolumeName,
				VolumeSource: volumeSource,
			}
			podSpec.Volumes = append(podSpec.Volumes, modelVolume)
		}
	}

	return nil
}

// AddEmptyDirVolumeIfNotPresent adds an emptyDir volume only if not present in the
// list. pod and pod.Spec must not be nil
func AddEmptyDirVolumeIfNotPresent(podSpec *corev1.PodSpec, name string) {
	for _, v := range podSpec.Volumes {
		if v.Name == name {
			return
		}
	}
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

// findCommonParentPath finds the common parent directory of multiple paths
func FindCommonParentPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	if len(paths) == 1 {
		return paths[0]
	}

	// Split all paths into components
	pathComponents := make([][]string, len(paths))
	minLength := -1
	for i, path := range paths {
		// Clean the path and split by "/"
		cleanPath := strings.Trim(path, "/")
		if cleanPath == "" {
			pathComponents[i] = []string{}
		} else {
			pathComponents[i] = strings.Split(cleanPath, "/")

			if minLength == -1 {
				minLength = len(pathComponents[i])
			} else {
				minLength = min(minLength, len(pathComponents[i]))
			}
		}
	}

	// Find common prefix
	var commonComponents []string
	for i := range minLength {
		levelComponents := make(map[string]struct{})
		var pathComponent string

		for _, components := range pathComponents {
			pathComponent = components[i]
			levelComponents[pathComponent] = struct{}{}
		}

		if len(levelComponents) == 1 {
			commonComponents = append(commonComponents, pathComponent)
		} else {
			break
		}
	}

	if len(commonComponents) == 0 {
		return "/"
	}

	return "/" + strings.Join(commonComponents, "/")
}

// Helper function to generate volume name from path
func GetVolumeNameFromPath(path string) string {
	// Convert path to valid volume name (remove slashes, etc.)
	return strings.ReplaceAll(strings.Trim(path, "/"), "/", "-")
}

func GetStorageResources(storageURIs []string, storagePaths []string) ([]corev1.VolumeMount, []corev1.Volume, []string, error) {
	initContainerArgs := make([]string, 0, len(storageURIs)*2)
	var volumeMounts []corev1.VolumeMount
	var volumes []corev1.Volume
	mountPaths := make([]string, 0, len(storageURIs))

	for i := range storageURIs {
		initContainerArgs = append(initContainerArgs, storageURIs[i], storagePaths[i])
		mountPaths = append(mountPaths, storagePaths[i])
	}

	mountPath := FindCommonParentPath(mountPaths)

	volumeName := GetVolumeNameFromPath(mountPath)

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  false,
	})

	volumes = append(volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	return volumeMounts, volumes, initContainerArgs, nil
}

func CreateInitContainerWithConfig(storageConfig *types.StorageInitializerConfig, containerArgs []string) *corev1.Container {
	storageInitializerImage := constants.StorageInitializerContainerImage + ":" + constants.StorageInitializerContainerImageVersion

	if storageConfig.Image != "" {
		storageInitializerImage = storageConfig.Image
	}

	return &corev1.Container{
		Name:                     constants.StorageInitializerContainerName,
		Image:                    storageInitializerImage,
		Args:                     containerArgs,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		VolumeMounts: []corev1.VolumeMount{{
			Name:      constants.StorageInitializerVolumeName,
			MountPath: constants.DefaultModelLocalMountPath,
			ReadOnly:  false,
		}},
		Env: []corev1.EnvVar{
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
		},
		Resources: corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(storageConfig.CpuLimit),
				corev1.ResourceMemory: resource.MustParse(storageConfig.MemoryLimit),
			},
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(storageConfig.CpuRequest),
				corev1.ResourceMemory: resource.MustParse(storageConfig.MemoryRequest),
			},
		},
	}
}

// CreateModelcarContainer creates the definition of a container holding a model intended to be used as a sidecar (modelcar).
// The container is configured with CPU, memory, and UID settings from the storage initializer configuration.
//
// Parameters:
//   - image: The container image to use for the modelcar.
//   - modelPath: The path where the model should be mounted inside the container.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//   - *corev1.Container: The modelcar container definition.
func CreateModelcarContainer(image string, modelPath string, storageConfig *types.StorageInitializerConfig) *corev1.Container {
	cpu := storageConfig.CpuModelcar
	if cpu == "" {
		cpu = constants.CpuModelcarDefault
	}
	memory := storageConfig.MemoryModelcar
	if memory == "" {
		memory = constants.MemoryModelcarDefault
	}

	modelContainer := &corev1.Container{
		Name:  constants.ModelcarContainerName,
		Image: image,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: GetParentDirectory(modelPath),
				ReadOnly:  false,
			},
		},
		Args: []string{
			"sh",
			"-c",
			// $$$$ gets escaped by YAML to $$, which is the current PID
			fmt.Sprintf("ln -sf /proc/$$$$/root/models %s && sleep infinity", modelPath),
		},
		Resources: corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				// Could possibly be reduced to even less
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
		},
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}

	if storageConfig.UidModelcar != nil {
		modelContainer.SecurityContext = &corev1.SecurityContext{
			RunAsUser: storageConfig.UidModelcar,
		}
	}

	return modelContainer
}

// CreateModelcarInitContainer is similar to CreateModelcarContainer but returns an init container definition.
// This init container is intended to run before the main containers to pre-fetch and validate the modelcar image.
//
// Parameters:
//   - image: The container image to use for the modelcar init container.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//   - *corev1.Container: The modelcar init container definition.
func CreateModelcarInitContainer(image string, storageConfig *types.StorageInitializerConfig) *corev1.Container {
	cpu := storageConfig.CpuModelcar
	if cpu == "" {
		cpu = constants.CpuModelcarDefault
	}
	memory := storageConfig.MemoryModelcar
	if memory == "" {
		memory = constants.MemoryModelcarDefault
	}

	modelContainer := &corev1.Container{
		Name:  constants.ModelcarInitContainerName,
		Image: image,
		Args: []string{
			"sh",
			"-c",
			// Check that the expected models directory exists
			"echo 'Pre-fetching modelcar " + image + ": ' && [ -d /models ] && [ \"$$(ls -A /models)\" ] && echo 'OK ... Prefetched and valid (/models exists)' || (echo 'NOK ... Prefetched but modelcar is invalid (/models does not exist or is empty)' && exit 1)",
		},
		Resources: corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				// Could possibly be reduced to even less
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
		},
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}

	return modelContainer
}

// ConfigureModelcarToContainer configures the OCI image specified in modelUri as a modelcar to the
// specified target container of a given PodSpec. The configuration includes:
//   - Adding an environment variable `async` to indicate to the runtime that the model directory may not be available immediately.
//   - Setting the user ID for the target container, if specified in storageConfig.
//   - Adding a modelcar and init containers (for pre-fetching the model) if not already present.
//   - Mounting a volume to the target container to access the model directory (via a shared volume).
//   - Enabling process namespace sharing (because of the shared volume).
//
// Parameters:
//   - modelUri: The URI specifying the model image location.
//   - podSpec: The PodSpec to modify.
//   - targetContainerName: The name of the container to configure the modelcar for.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//   - error: An error if the target container is not found or if configuration fails; otherwise, nil.
func ConfigureModelcarToContainer(modelUri string, podSpec *corev1.PodSpec, targetContainerName string, storageConfig *types.StorageInitializerConfig) error {
	targetContainer := GetContainerWithName(podSpec, targetContainerName)
	if targetContainer == nil {
		return fmt.Errorf("no container found with name %s", targetContainerName)
	}

	// Indicate to the runtime that it the model directory could be
	// available a bit later only so that it should wait and retry when
	// starting up
	AddOrReplaceEnv(targetContainer, constants.ModelInitModeEnvVarKey, "async")

	// Mount volume initialized by the modelcar container to the target container
	modelParentDir := GetParentDirectory(constants.DefaultModelLocalMountPath)
	AddEmptyDirVolumeIfNotPresent(podSpec, constants.StorageInitializerVolumeName)
	AddVolumeMountIfNotPresent(targetContainer, constants.StorageInitializerVolumeName, modelParentDir, false)

	// If configured, run as the given user. There might be certain installations
	// of Kubernetes where sharing the filesystem via the process namespace only works
	// when both containers are running as root
	if storageConfig.UidModelcar != nil {
		targetContainer.SecurityContext = &corev1.SecurityContext{
			RunAsUser: storageConfig.UidModelcar,
		}
	}

	// Create the modelcar that is used as a sidecar in Pod and add it to the end
	// of the containers (but only if not already have been added)
	if GetContainerWithName(podSpec, constants.ModelcarContainerName) == nil {
		// Extract image reference for modelcar from URI
		image := strings.TrimPrefix(modelUri, constants.OciURIPrefix)

		modelContainer := CreateModelcarContainer(image, constants.DefaultModelLocalMountPath, storageConfig)
		podSpec.Containers = append(podSpec.Containers, *modelContainer)

		// Add the model container as an init-container to pre-fetch the model before
		// the runtimes starts.
		modelInitContainer := CreateModelcarInitContainer(image, storageConfig)
		podSpec.InitContainers = append(podSpec.InitContainers, *modelInitContainer)
	}

	// Enable process namespace sharing so that the modelcar's root filesystem
	// can be reached by the user container
	podSpec.ShareProcessNamespace = ptr.To(true)

	return nil
}
