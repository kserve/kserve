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

package credentials

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// OciFetchDockerConfigVolumeName is the projected-secret volume that carries the
	// registry credentials (docker config.json) into the storage-initializer container.
	OciFetchDockerConfigVolumeName = "kserve-oci-fetch-docker-config"
	// OciFetchDockerConfigDir is the directory where the docker config.json is mounted.
	// It is NOT under /root: the storage-initializer image runs as a non-root user (UID
	// 1000), which cannot traverse /root (mode 0700). /mnt is chowned to that user in the
	// image, so the mounted credentials are readable. The Python handler reads the file
	// path from OciFetchDockerConfigPathEnvVar and passes it to oras-py as an explicit
	// config_path (oras-py ignores DOCKER_CONFIG and otherwise reads ~/.docker/config.json).
	OciFetchDockerConfigDir = "/mnt/oci-fetch-auth"
	// OciFetchDockerConfigPathEnvVar signals to the Python handler where the docker
	// config.json is mounted, keeping the cross-language path in one place (the Go side).
	OciFetchDockerConfigPathEnvVar = "KSERVE_OCI_DOCKER_CONFIG"
	// OciInsecureRegistryEnvVar signals to the Python handler that the target
	// registry should be treated as plain-HTTP/insecure (no TLS verification).
	OciInsecureRegistryEnvVar = "KSERVE_OCI_INSECURE_REGISTRY"
)

// MountImagePullSecretsAsDockerConfig projects the first imagePullSecret into the
// container as a docker config.json so the Python storage initializer (oras-py) can
// authenticate to private OCI registries. The Python handler reads this file via
// OciFetchDockerConfigPathEnvVar (oras-py ignores DOCKER_CONFIG), so we mount it at
// a UID-agnostic path under /mnt, not /root (UID 1000 cannot traverse /root).
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
func MountImagePullSecretsAsDockerConfig(
	imagePullSecrets []corev1.LocalObjectReference,
	container *corev1.Container,
	volumes *[]corev1.Volume,
) error {
	if len(imagePullSecrets) == 0 {
		return nil
	}
	if len(imagePullSecrets) > 1 {
		log.Info("Multiple imagePullSecrets found for OCI fetch; using the first only "+
			"(multi-secret merging is not yet supported, combine credentials into one dockerconfigjson secret)",
			"secretCount", len(imagePullSecrets), "selectedSecret", imagePullSecrets[0].Name)
	}
	secretName := imagePullSecrets[0].Name

	if ociDockerConfigVolumeExists(*volumes) {
		log.Info("docker config volume already present; skipping duplicate mount",
			"volume", OciFetchDockerConfigVolumeName)
		return nil
	}

	// Do not set DefaultMode 0400. Secret files are root-owned unless fsGroup is
	// applied; the storage-initializer runs as UID 1000 and cannot read owner-only
	// root-owned files. Kubelet's default 0644 is readable by that user. The mount
	// is still read-only.
	*volumes = append(*volumes, corev1.Volume{
		Name: OciFetchDockerConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
				Items: []corev1.KeyToPath{
					{Key: corev1.DockerConfigJsonKey, Path: "config.json"},
				},
			},
		},
	})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      OciFetchDockerConfigVolumeName,
		MountPath: OciFetchDockerConfigDir,
		ReadOnly:  true,
	})
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  OciFetchDockerConfigPathEnvVar,
		Value: OciFetchDockerConfigDir + "/config.json",
	})
	return nil
}

// SetOciInsecureRegistryEnv sets KSERVE_OCI_INSECURE_REGISTRY=true on the download
// container when storageInitializer.ociInsecureRegistry is enabled. Idempotent.
func SetOciInsecureRegistryEnv(container *corev1.Container) {
	if container == nil {
		return
	}
	for _, env := range container.Env {
		if env.Name == OciInsecureRegistryEnvVar {
			return
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  OciInsecureRegistryEnvVar,
		Value: "true",
	})
}

func ociDockerConfigVolumeExists(volumes []corev1.Volume) bool {
	for _, v := range volumes {
		if v.Name == OciFetchDockerConfigVolumeName {
			return true
		}
	}
	return false
}
