/*
Copyright 2026 The KServe Authors.

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
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
	pkgtypes "github.com/kserve/kserve/pkg/types"
)

const (
	// OciCaBundleVolumeName is the ConfigMap volume that carries a custom CA
	// bundle for private-registry TLS. Shared with the storage-initializer
	// webhook / LocalModel S3 CA mount path so the same volume is not mounted twice.
	OciCaBundleVolumeName = "cabundle-cert"
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

// ociFetchDockerConfigDefaultMode matches the pre-extract InferenceService oci+fetch
// volume (0400). Changing it here would roll every existing oci+fetch ISVC pod.
var ociFetchDockerConfigDefaultMode = int32(0o400)

// FirstNamedImagePullSecret returns the first imagePullSecret with a non-empty
// name. LocalModel cache CRs admit at most one named secret; InferenceService
// PodSpecs can still list several, and this helper keeps that path selecting
// a usable name.
func FirstNamedImagePullSecret(imagePullSecrets []corev1.LocalObjectReference) (string, bool) {
	for _, secret := range imagePullSecrets {
		if secret.Name != "" {
			return secret.Name, true
		}
	}
	return "", false
}

// FetchAndValidateDockerConfigJSONSecret loads a Secret and checks it is a
// kubernetes.io/dockerconfigjson with a ".dockerconfigjson" key.
// Use an uncached Reader (mgr.GetAPIReader() or equivalent) so validating a
// Secret does not start a cluster-wide Secret informer on the manager cache.
func FetchAndValidateDockerConfigJSONSecret(ctx context.Context, reader client.Reader, namespace, name string) error {
	if name == "" {
		return errors.New("imagePullSecret name must be non-empty")
	}
	secret := &corev1.Secret{}
	if err := reader.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		if apierr.IsNotFound(err) {
			return fmt.Errorf("imagePullSecret %q not found in namespace %q", name, namespace)
		}
		return err
	}
	if secret.Type != corev1.SecretTypeDockerConfigJson {
		return fmt.Errorf("imagePullSecret %q in namespace %q has type %q, want %s", name, namespace, secret.Type, corev1.SecretTypeDockerConfigJson)
	}
	if _, ok := secret.Data[corev1.DockerConfigJsonKey]; !ok {
		return fmt.Errorf("imagePullSecret %q in namespace %q is missing key %q", name, namespace, corev1.DockerConfigJsonKey)
	}
	return nil
}

// MountImagePullSecretsAsDockerConfig projects the first named imagePullSecret into the
// container as a docker config.json so the Python storage initializer (oras-py) can
// authenticate to private OCI registries. The Python handler reads this file via
// OciFetchDockerConfigPathEnvVar (oras-py ignores DOCKER_CONFIG), so we mount it at
// a UID-agnostic path under /mnt, not /root (UID 1000 cannot traverse /root).
//
// LocalModel cache CRs admit at most one named secret. InferenceService PodSpecs
// can still list several; extra names are ignored and a warning is logged. Combine
// credentials for multiple registries into one dockerconfigjson secret.
func MountImagePullSecretsAsDockerConfig(
	imagePullSecrets []corev1.LocalObjectReference,
	container *corev1.Container,
	volumes *[]corev1.Volume,
) {
	secretName, ok := FirstNamedImagePullSecret(imagePullSecrets)
	if !ok {
		return
	}
	if len(imagePullSecrets) > 1 {
		log.Info("Multiple imagePullSecrets found for OCI fetch; using the first named secret only "+
			"(multi-secret merging is not yet supported, combine credentials into one dockerconfigjson secret)",
			"secretCount", len(imagePullSecrets), "selectedSecret", secretName)
	}

	if ociDockerConfigVolumeExists(*volumes) {
		log.Info("docker config volume already present; skipping duplicate mount",
			"volume", OciFetchDockerConfigVolumeName)
		return
	}

	*volumes = append(*volumes, corev1.Volume{
		Name: OciFetchDockerConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &ociFetchDockerConfigDefaultMode,
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
}

// SetOciInsecureRegistryEnv sets KSERVE_OCI_INSECURE_REGISTRY=true on the download
// container when storageInitializer.ociInsecureRegistry is enabled. Overwrites an
// existing value of the same name.
func SetOciInsecureRegistryEnv(container *corev1.Container) {
	if container == nil {
		return
	}
	for i := range container.Env {
		if container.Env[i].Name == OciInsecureRegistryEnvVar {
			container.Env[i].Value = "true"
			return
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  OciInsecureRegistryEnvVar,
		Value: "true",
	})
}

// MountOciCaBundle mounts storageInitializer.caBundleConfigMapName for OCI
// private-registry TLS. Mirrors the InferenceService oci+fetch webhook path so
// the Python handler can read cabundle.crt via CA_BUNDLE_VOLUME_MOUNT_POINT.
// No-op when caBundleConfigMapName is unset or the cabundle volume is already present.
//
// Outside the KServe system namespace the mirrored per-namespace ConfigMap
// (global-ca-bundle) is mounted, matching CommonStorageInitialization.
func MountOciCaBundle(
	storageConfig *pkgtypes.StorageInitializerConfig,
	namespace string,
	container *corev1.Container,
	volumes *[]corev1.Volume,
) {
	if storageConfig == nil || storageConfig.CaBundleConfigMapName == "" || container == nil || volumes == nil {
		return
	}
	if ociCaBundleVolumeExists(*volumes) {
		return
	}

	caBundleConfigMapName := storageConfig.CaBundleConfigMapName
	if namespace != constants.KServeNamespace {
		caBundleConfigMapName = constants.DefaultGlobalCaBundleConfigMapName
	}
	caBundleVolumeMountPath := storageConfig.CaBundleVolumeMountPath
	if caBundleVolumeMountPath == "" {
		caBundleVolumeMountPath = constants.DefaultCaBundleVolumeMountPath
	}

	*volumes = append(*volumes, corev1.Volume{
		Name: OciCaBundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: caBundleConfigMapName},
			},
		},
	})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      OciCaBundleVolumeName,
		MountPath: caBundleVolumeMountPath,
		ReadOnly:  true,
	})
	if !ociEnvExists(container.Env, constants.CaBundleConfigMapNameEnvVarKey) {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  constants.CaBundleConfigMapNameEnvVarKey,
			Value: caBundleConfigMapName,
		})
	}
	if !ociEnvExists(container.Env, constants.CaBundleVolumeMountPathEnvVarKey) {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  constants.CaBundleVolumeMountPathEnvVarKey,
			Value: caBundleVolumeMountPath,
		})
	}
}

func ociDockerConfigVolumeExists(volumes []corev1.Volume) bool {
	for _, v := range volumes {
		if v.Name == OciFetchDockerConfigVolumeName {
			return true
		}
	}
	return false
}

func ociCaBundleVolumeExists(volumes []corev1.Volume) bool {
	for _, v := range volumes {
		if v.Name == OciCaBundleVolumeName {
			return true
		}
	}
	return false
}

func ociEnvExists(envs []corev1.EnvVar, name string) bool {
	for _, e := range envs {
		if e.Name == name {
			return true
		}
	}
	return false
}
