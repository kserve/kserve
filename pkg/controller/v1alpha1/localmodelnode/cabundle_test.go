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

package localmodelnode

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/s3"
)

func TestMountCaBundleVolume_WithEnvVar(t *testing.T) {
	reconciler := &LocalModelNodeReconciler{Log: ctrl.Log.WithName("test")}
	container := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: s3.AWSCABundleConfigMap, Value: "my-ca-bundle"},
		},
	}
	volumes := []corev1.Volume{}

	reconciler.mountCaBundleVolume(container, &volumes)

	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	if volumes[0].Name != CaBundleVolumeName {
		t.Errorf("expected volume name %q, got %q", CaBundleVolumeName, volumes[0].Name)
	}
	if volumes[0].ConfigMap.Name != "my-ca-bundle" {
		t.Errorf("expected configmap name %q, got %q", "my-ca-bundle", volumes[0].ConfigMap.Name)
	}

	foundMount := false
	for _, vm := range container.VolumeMounts {
		if vm.Name == CaBundleVolumeName {
			foundMount = true
			if vm.MountPath != constants.DefaultCaBundleVolumeMountPath {
				t.Errorf("expected mount path %q, got %q", constants.DefaultCaBundleVolumeMountPath, vm.MountPath)
			}
			if !vm.ReadOnly {
				t.Error("expected volume mount to be read-only")
			}
		}
	}
	if !foundMount {
		t.Error("CA bundle volume mount not found on container")
	}

	envMap := map[string]string{}
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	if envMap[constants.CaBundleConfigMapNameEnvVarKey] != "my-ca-bundle" {
		t.Errorf("expected %s=%q, got %q", constants.CaBundleConfigMapNameEnvVarKey, "my-ca-bundle", envMap[constants.CaBundleConfigMapNameEnvVarKey])
	}
	if envMap[constants.CaBundleVolumeMountPathEnvVarKey] != constants.DefaultCaBundleVolumeMountPath {
		t.Errorf("expected %s=%q, got %q", constants.CaBundleVolumeMountPathEnvVarKey, constants.DefaultCaBundleVolumeMountPath, envMap[constants.CaBundleVolumeMountPathEnvVarKey])
	}
}

func TestMountCaBundleVolume_WithoutEnvVar(t *testing.T) {
	reconciler := &LocalModelNodeReconciler{Log: ctrl.Log.WithName("test")}
	container := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "UNRELATED", Value: "value"},
		},
	}
	volumes := []corev1.Volume{}
	envBefore := append([]corev1.EnvVar{}, container.Env...)

	reconciler.mountCaBundleVolume(container, &volumes)

	if len(volumes) != 0 {
		t.Fatalf("expected 0 volumes when no CA bundle env, got %d", len(volumes))
	}
	if len(container.VolumeMounts) != 0 {
		t.Fatalf("expected 0 volume mounts when no CA bundle env, got %d", len(container.VolumeMounts))
	}
	if !reflect.DeepEqual(container.Env, envBefore) {
		t.Errorf("expected container env unchanged, got %+v", container.Env)
	}
}

func TestMountCaBundleVolume_EmptyValue(t *testing.T) {
	reconciler := &LocalModelNodeReconciler{Log: ctrl.Log.WithName("test")}
	container := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: s3.AWSCABundleConfigMap, Value: ""},
		},
	}
	volumes := []corev1.Volume{}
	envBefore := append([]corev1.EnvVar{}, container.Env...)

	reconciler.mountCaBundleVolume(container, &volumes)

	if len(volumes) != 0 {
		t.Fatalf("expected 0 volumes for empty configmap name, got %d", len(volumes))
	}
	if !reflect.DeepEqual(container.Env, envBefore) {
		t.Errorf("expected container env unchanged, got %+v", container.Env)
	}
}

func TestErrStorageKeyNotFound_IsTyped(t *testing.T) {
	err := fmt.Errorf("specified storage key foo not found: %w", credentials.ErrStorageKeyNotFound)
	if !errors.Is(err, credentials.ErrStorageKeyNotFound) {
		t.Error("expected errors.Is to match ErrStorageKeyNotFound")
	}
}

func TestErrStorageConfigSecretNotFound_IsTyped(t *testing.T) {
	err := fmt.Errorf("can't read storage secret bar: %w", credentials.ErrStorageConfigSecretNotFound)
	if !errors.Is(err, credentials.ErrStorageConfigSecretNotFound) {
		t.Error("expected errors.Is to match ErrStorageConfigSecretNotFound")
	}
}
