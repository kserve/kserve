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
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	// ociFetchDockerConfigVolumeName is the projected-secret volume that carries the
	// registry credentials (docker config.json) into the fetch init container.
	ociFetchDockerConfigVolumeName = "kserve-oci-fetch-docker-config"
	// ociFetchDockerConfigDir is the directory where the docker config.json is mounted.
	// It is NOT under /root: the storage-initializer image runs as a non-root user (UID
	// 1000), which cannot traverse /root (mode 0700). /mnt is chowned to that user in the
	// image, so the mounted credentials are readable. The Python handler reads the file
	// path from ociFetchDockerConfigPathEnvVar and passes it to oras-py as an explicit
	// config_path (oras-py ignores DOCKER_CONFIG and otherwise reads ~/.docker/config.json).
	ociFetchDockerConfigDir = "/mnt/oci-fetch-auth"
	// ociFetchDockerConfigPathEnvVar signals to the Python handler where the docker
	// config.json is mounted, keeping the cross-language path in one place (the Go side).
	ociFetchDockerConfigPathEnvVar = "KSERVE_OCI_DOCKER_CONFIG"
	// ociFetchDefaultVolumeName is the fallback model volume name when modelPath does not
	// yield a usable name (e.g. the root path).
	ociFetchDefaultVolumeName = "oci-fetch-model"
	// ociFetchInsecureRegistryEnvVar signals to the Python handler that the target
	// registry should be treated as plain-HTTP/insecure (no TLS verification). Only
	// set when storageConfig.OciInsecureRegistry is explicitly true; absent otherwise,
	// so the Python side's default (secure/verified HTTPS) applies.
	ociFetchInsecureRegistryEnvVar = "KSERVE_OCI_INSECURE_REGISTRY"
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

		if err := mountImagePullSecretsAsDockerConfig(podSpec.ImagePullSecrets, initContainer, &podSpec.Volumes); err != nil {
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

// mountImagePullSecretsAsDockerConfig projects the pod's first imagePullSecret into the
// init container as a docker config.json so the Python storage initializer (oras-py) can
// authenticate to private OCI registries. The Python handler passes this file to oras-py via
// an explicit config_path argument (oras-py ignores DOCKER_CONFIG), so we mount it at the
// fixed path signaled via the KSERVE_OCI_DOCKER_CONFIG env var. That path is under /mnt
// (a UID-agnostic location), not /root, because the init container runs as UID 1000 and
// cannot traverse /root (mode 0700).
//
//   - 0 secrets: no-op. Anonymous pulls succeed for public registries; private registries
//     fail with a clear authorization error at pull time.
//   - 1 secret: the secret's ".dockerconfigjson" key is projected to <dir>/config.json.
//   - >1 secrets: the first secret is used and a warning is logged; multi-secret merging is
//     not yet supported (users can combine credentials into a single dockerconfigjson secret).
//
// The secret is referenced by name only; kubelet projects its contents at pod startup. A
// kubernetes.io/dockerconfigjson secret is assumed; a legacy kubernetes.io/dockercfg secret
// lacks the ".dockerconfigjson" key, so the projected file would be absent and the pull would
// fail with a clear error.
func mountImagePullSecretsAsDockerConfig(
	imagePullSecrets []corev1.LocalObjectReference,
	container *corev1.Container,
	volumes *[]corev1.Volume,
) error {
	if len(imagePullSecrets) == 0 {
		return nil
	}
	if len(imagePullSecrets) > 1 {
		log.Info("Multiple imagePullSecrets found for oci+fetch://; using the first only "+
			"(multi-secret merging is not yet supported, combine credentials into one dockerconfigjson secret)",
			"secretCount", len(imagePullSecrets), "selectedSecret", imagePullSecrets[0].Name)
	}
	secretName := imagePullSecrets[0].Name

	if volumeExists(*volumes, ociFetchDockerConfigVolumeName) {
		log.Info("docker config volume already present; skipping duplicate mount",
			"volume", ociFetchDockerConfigVolumeName)
		return nil
	}

	// DefaultMode 0400: readable only by the container UID; the docker config contains
	// registry credentials, so restrict it to the owner (defense-in-depth on top of the
	// read-only mount below).
	*volumes = append(*volumes, corev1.Volume{
		Name: ociFetchDockerConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To[int32](0o400),
				Items: []corev1.KeyToPath{
					{Key: corev1.DockerConfigJsonKey, Path: "config.json"},
				},
			},
		},
	})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      ociFetchDockerConfigVolumeName,
		MountPath: ociFetchDockerConfigDir,
		ReadOnly:  true,
	})
	// Tell the Python handler where to find the projected config.json.
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  ociFetchDockerConfigPathEnvVar,
		Value: ociFetchDockerConfigDir + "/config.json",
	})
	return nil
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
