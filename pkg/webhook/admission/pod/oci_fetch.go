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

package pod

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	// Aliases for credentials package constants so existing webhook tests keep compiling.
	ociFetchDockerConfigVolumeName = credentials.OciFetchDockerConfigVolumeName
	ociFetchDockerConfigDir        = credentials.OciFetchDockerConfigDir
	ociFetchDockerConfigPathEnvVar = credentials.OciFetchDockerConfigPathEnvVar
	ociFetchInsecureRegistryEnvVar = credentials.OciInsecureRegistryEnvVar
	// ociFetchDefaultVolumeName is the fallback model volume name when modelPath does not
	// yield a usable name (e.g. the root path).
	ociFetchDefaultVolumeName = "oci-fetch-model"
)

// ConfigureOciFetchToContainer wires an oci+fetch:// model into targetContainerName by
// injecting the standard kserve storage-initializer init container. Unlike native
// (Kubernetes ImageVolume) and modelcar (sidecar) modes, fetch reuses the regular
// init-container download mechanism: the init container receives the normalized oci://
// URI as a storage arg and the Python storage initializer's oci:// handler pulls the
// image's model layers into a shared emptyDir volume at modelPath.
//
// Registry authentication is supplied by projecting the pod's first imagePullSecret as a
// docker config.json into the init container (see mountImagePullSecretsAsDockerConfig); a
// custom CA bundle for private-registry TLS is mounted when configured, mirroring the CA
// bundle handling in CommonStorageInitialization.
//
// modelUri must be the normalized oci:// URI (ParseOciScheme strips the +fetch suffix).
// The function is idempotent and safe to call once per (URI, target container) pair: the
// init container is created at most once per pod, each URI's (uri, path) arg pair is added
// at most once, and per-path model mounts are de-duplicated by AddModelMount.
//
// Limitation: mixing oci+fetch:// with non-OCI storage URIs (S3/GCS/…) in the same pod is
// unsupported — the shared storage-initializer init-container name causes the non-OCI
// download path to be skipped. Single or multiple oci+fetch:// sources are supported.
func ConfigureOciFetchToContainer(
	modelUri string,
	podSpec *corev1.PodSpec,
	targetContainerName string,
	modelPath string,
	storageConfig *types.StorageInitializerConfig,
	namespace string,
) error {
	if utils.GetContainerWithName(podSpec, targetContainerName) == nil {
		return fmt.Errorf("no container found with name %s", targetContainerName)
	}

	initContainer := getStorageInitializerInitContainer(podSpec)
	if initContainer == nil {
		// Build the init container with the (uri, path) arg pair the Python initializer
		// expects, then attach registry credentials + CA bundle to it.
		built := utils.CreateInitContainerWithConfig(storageConfig, []string{modelUri, modelPath})
		podSpec.InitContainers = append(podSpec.InitContainers, *built)
		initContainer = &podSpec.InitContainers[len(podSpec.InitContainers)-1]

		if err := credentials.MountImagePullSecretsAsDockerConfig(podSpec.ImagePullSecrets, initContainer, &podSpec.Volumes); err != nil {
			return err
		}
		mountCaBundleForFetch(storageConfig, namespace, initContainer, podSpec)
		if storageConfig.OciInsecureRegistry {
			initContainer.Env = append(initContainer.Env, corev1.EnvVar{
				Name:  ociFetchInsecureRegistryEnvVar,
				Value: "true",
			})
		}
	} else if !initContainerArgsContainPair(initContainer.Args, modelUri, modelPath) {
		// Additional fetch source: append its (uri, path) pair to the shared init container.
		initContainer.Args = append(initContainer.Args, modelUri, modelPath)
	}

	// Share the downloaded model between the init container (writer) and the target
	// container (reader) via a single emptyDir volume mounted at modelPath on both.
	volumeName := utils.GetVolumeNameFromPath(modelPath)
	if volumeName == "" {
		volumeName = ociFetchDefaultVolumeName
	}
	mountParams := utils.StorageMountParams{
		MountPath:  modelPath,
		VolumeName: volumeName,
		ReadOnly:   false,
	}
	if err := utils.AddModelMount(mountParams, constants.StorageInitializerContainerName, podSpec); err != nil {
		return err
	}
	return utils.AddModelMount(mountParams, targetContainerName, podSpec)
}

// mountImagePullSecretsAsDockerConfig is a thin wrapper around the shared helper so
// existing webhook unit tests can keep calling the unexported name.
func mountImagePullSecretsAsDockerConfig(
	imagePullSecrets []corev1.LocalObjectReference,
	container *corev1.Container,
	volumes *[]corev1.Volume,
) error {
	return credentials.MountImagePullSecretsAsDockerConfig(imagePullSecrets, container, volumes)
}

// mountCaBundleForFetch mounts a custom CA bundle configmap into the fetch init container
// for private-registry TLS, mirroring the CA bundle handling in CommonStorageInitialization.
// It is a no-op when no CA bundle is configured.
func mountCaBundleForFetch(
	storageConfig *types.StorageInitializerConfig,
	namespace string,
	initContainer *corev1.Container,
	podSpec *corev1.PodSpec,
) {
	if storageConfig.CaBundleConfigMapName == "" {
		return
	}
	if volumeExists(podSpec.Volumes, CaBundleVolumeName) {
		// Already mounted (idempotent call); nothing to do.
		return
	}
	caBundleConfigMapName := storageConfig.CaBundleConfigMapName
	// Outside the KServe namespace the bundle is mirrored to a per-namespace configmap.
	if namespace != constants.KServeNamespace {
		caBundleConfigMapName = constants.DefaultGlobalCaBundleConfigMapName
	}
	caBundleVolumeMountPath := storageConfig.CaBundleVolumeMountPath
	if caBundleVolumeMountPath == "" {
		caBundleVolumeMountPath = constants.DefaultCaBundleVolumeMountPath
	}

	initContainer.Env = append(initContainer.Env,
		corev1.EnvVar{Name: constants.CaBundleConfigMapNameEnvVarKey, Value: caBundleConfigMapName},
		corev1.EnvVar{Name: constants.CaBundleVolumeMountPathEnvVarKey, Value: caBundleVolumeMountPath},
	)
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: CaBundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: caBundleConfigMapName},
			},
		},
	})
	initContainer.VolumeMounts = append(initContainer.VolumeMounts, corev1.VolumeMount{
		Name:      CaBundleVolumeName,
		MountPath: caBundleVolumeMountPath,
		ReadOnly:  true,
	})
}

// getStorageInitializerInitContainer returns a pointer to the storage-initializer init
// container, or nil if absent. utils.GetContainerWithName only searches regular
// containers, not init containers.
func getStorageInitializerInitContainer(podSpec *corev1.PodSpec) *corev1.Container {
	for idx := range podSpec.InitContainers {
		if podSpec.InitContainers[idx].Name == constants.StorageInitializerContainerName {
			return &podSpec.InitContainers[idx]
		}
	}
	return nil
}

// initContainerArgsContainPair reports whether args already contains the consecutive
// (uri, path) pair, so repeated calls for the same source (e.g. for the main and
// transformer containers) don't duplicate init-container args.
func initContainerArgsContainPair(args []string, uri, path string) bool {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] == uri && args[i+1] == path {
			return true
		}
	}
	return false
}

// volumeExists reports whether podSpec.Volumes already has a volume with the given name.
// Callers should skip adding a duplicate to avoid Kubernetes rejecting the pod on admission.
func volumeExists(volumes []corev1.Volume, name string) bool {
	for _, v := range volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}
