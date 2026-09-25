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

package podconfig

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/kernelcache/registryauth"
)

const (
	registryVolume = "mcv-registry-access"
	registryPath   = "/var/run/secrets/mcv-registry"
)

// ApplyCaptureRegistry mounts deferred registry access without changing the Pod identity.
func ApplyCaptureRegistry(pod *corev1.PodSpec, container *corev1.Container, cfg v1beta1.KernelCacheRegistryConfig, secretName string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	sources := []corev1.VolumeProjection{}
	env := []corev1.EnvVar{}
	switch cfg.Auth.Type {
	case "", v1beta1.KernelCacheRegistryAuthTypeNone:
	case v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken:
		if cfg.Endpoint == "" || strings.ContainsAny(cfg.Endpoint, "/ \t\n") {
			return errors.New("registry.endpoint must be a registry host with optional port")
		}
		if secretName == "" {
			return errors.New("capture registry access Secret name is required")
		}
		sources = append(sources, corev1.VolumeProjection{Secret: &corev1.SecretProjection{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Optional: ptr.To(true),
			Items: []corev1.KeyToPath{{Key: registryauth.AccessKey, Path: registryauth.AccessKey}},
		}})
		env = append(env, corev1.EnvVar{Name: "MCV_REGISTRY_ACCESS_FILE", Value: registryPath + "/" + registryauth.AccessKey})
		// The webhook passes the operator-configured endpoint from the kernelcache
		// section of the inferenceservice-config ConfigMap as the allowlist.
		env = append(env, corev1.EnvVar{Name: "MCV_REGISTRY_ALLOWED_ENDPOINT", Value: cfg.Endpoint})
	default:
		return fmt.Errorf("unsupported registry.auth.type %q", cfg.Auth.Type)
	}
	if ref := cfg.CAConfigMapRef; ref != nil {
		if ref.Name == "" || ref.Key == "" {
			return errors.New("registry.caConfigMapRef requires name and key")
		}
		sources = append(sources, corev1.VolumeProjection{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name}, Optional: ptr.To(true), Items: []corev1.KeyToPath{{Key: ref.Key, Path: "ca.crt"}}}})
		env = append(env, corev1.EnvVar{Name: "MCV_REGISTRY_CA_FILE", Value: registryPath + "/ca.crt"})
	}
	if len(sources) == 0 {
		container.Env = append(container.Env, env...)
		return nil
	}
	volume := corev1.Volume{Name: registryVolume, VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: sources}}}
	found := false
	for _, existing := range pod.Volumes {
		if existing.Name == registryVolume {
			if !reflect.DeepEqual(existing, volume) {
				return fmt.Errorf("conflicting volume %q", registryVolume)
			}
			found = true
		}
	}
	if !found {
		pod.Volumes = append(pod.Volumes, volume)
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: registryVolume, MountPath: registryPath, ReadOnly: true})
	container.Env = append(container.Env, env...)
	return nil
}
