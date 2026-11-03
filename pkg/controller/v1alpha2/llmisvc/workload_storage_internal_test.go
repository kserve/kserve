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

package llmisvc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// mergeStorageInitForTest wraps utils.MergeContainerWithPatch with the same
// Name/Args/Command restoration that the production code applies.
func mergeStorageInitForTest(ctrl *corev1.Container, user corev1.Container) (*corev1.Container, error) {
	merged, err := utils.MergeContainerWithPatch(*ctrl, user)
	if err != nil {
		return nil, err
	}
	merged.Name = ctrl.Name
	merged.Args = ctrl.Args
	merged.Command = ctrl.Command
	return &merged, nil
}

func TestExtractAndStripStorageInitializer(t *testing.T) {
	t.Parallel()

	t.Run("extracts and strips when present", func(t *testing.T) {
		t.Parallel()
		si := corev1.Container{
			Name:  constants.StorageInitializerContainerName,
			Image: "custom-storage:v1",
			Env:   []corev1.EnvVar{{Name: "CUSTOM_VAR", Value: "value"}},
		}
		other := corev1.Container{Name: "sidecar", Image: "sidecar:v1"}
		podSpec := &corev1.PodSpec{
			InitContainers: []corev1.Container{other, si},
		}

		extracted := extractAndStripStorageInitializer(podSpec)

		if extracted == nil {
			t.Fatal("expected extracted container, got nil")
		}
		if extracted.Name != constants.StorageInitializerContainerName {
			t.Fatalf("extracted name = %q, want %q", extracted.Name, constants.StorageInitializerContainerName)
		}
		if extracted.Image != "custom-storage:v1" {
			t.Fatalf("extracted image = %q, want %q", extracted.Image, "custom-storage:v1")
		}
		if len(podSpec.InitContainers) != 1 {
			t.Fatalf("remaining initContainers = %d, want 1", len(podSpec.InitContainers))
		}
		if podSpec.InitContainers[0].Name != "sidecar" {
			t.Fatalf("remaining container = %q, want %q", podSpec.InitContainers[0].Name, "sidecar")
		}
	})

	t.Run("returns nil when absent", func(t *testing.T) {
		t.Parallel()
		other := corev1.Container{Name: "sidecar", Image: "sidecar:v1"}
		podSpec := &corev1.PodSpec{
			InitContainers: []corev1.Container{other},
		}

		extracted := extractAndStripStorageInitializer(podSpec)

		if extracted != nil {
			t.Fatalf("expected nil, got %+v", extracted)
		}
		if len(podSpec.InitContainers) != 1 {
			t.Fatalf("initContainers = %d, want 1", len(podSpec.InitContainers))
		}
	})

	t.Run("returns nil for nil podSpec", func(t *testing.T) {
		t.Parallel()
		extracted := extractAndStripStorageInitializer(nil)
		if extracted != nil {
			t.Fatalf("expected nil, got %+v", extracted)
		}
	})
}

func TestMergeStorageInitializerContainer(t *testing.T) {
	t.Parallel()

	defaultContainer := func() *corev1.Container {
		return &corev1.Container{
			Name:  constants.StorageInitializerContainerName,
			Image: "kserve/storage-initializer:latest",
			Args:  []string{"hf://model", "/mnt/models"},
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
		}
	}

	t.Run("user overrides resources", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		user := corev1.Container{
			Name: constants.StorageInitializerContainerName,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !merged.Resources.Limits.Cpu().Equal(resource.MustParse("4")) {
			t.Errorf("CPU limit = %s, want 4", merged.Resources.Limits.Cpu())
		}
		if !merged.Resources.Limits.Memory().Equal(resource.MustParse("8Gi")) {
			t.Errorf("memory limit = %s, want 8Gi", merged.Resources.Limits.Memory())
		}
		if !merged.Resources.Requests.Cpu().Equal(resource.MustParse("2")) {
			t.Errorf("CPU request = %s, want 2", merged.Resources.Requests.Cpu())
		}
		if merged.Name != constants.StorageInitializerContainerName {
			t.Errorf("name = %q, want %q", merged.Name, constants.StorageInitializerContainerName)
		}
		if diff := cmp.Diff(ctrl.Args, merged.Args); diff != "" {
			t.Errorf("args mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("user overrides image", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		user := corev1.Container{
			Name:  constants.StorageInitializerContainerName,
			Image: "my-registry/custom-storage:v2",
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if merged.Image != "my-registry/custom-storage:v2" {
			t.Errorf("image = %q, want %q", merged.Image, "my-registry/custom-storage:v2")
		}
	})

	t.Run("user adds env vars", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		ctrl.Env = []corev1.EnvVar{{Name: "DEFAULT_VAR", Value: "default"}}
		user := corev1.Container{
			Name: constants.StorageInitializerContainerName,
			Env:  []corev1.EnvVar{{Name: "CUSTOM_VAR", Value: "custom"}},
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		envMap := make(map[string]string)
		for _, e := range merged.Env {
			envMap[e.Name] = e.Value
		}
		if envMap["DEFAULT_VAR"] != "default" {
			t.Errorf("DEFAULT_VAR = %q, want %q", envMap["DEFAULT_VAR"], "default")
		}
		if envMap["CUSTOM_VAR"] != "custom" {
			t.Errorf("CUSTOM_VAR = %q, want %q", envMap["CUSTOM_VAR"], "custom")
		}
	})

	t.Run("user adds volume mounts", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		ctrl.VolumeMounts = []corev1.VolumeMount{
			{Name: "model-vol", MountPath: "/mnt/models"},
		}
		user := corev1.Container{
			Name: constants.StorageInitializerContainerName,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "custom-certs", MountPath: "/etc/certs"},
			},
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mountMap := make(map[string]string)
		for _, m := range merged.VolumeMounts {
			mountMap[m.Name] = m.MountPath
		}
		if mountMap["model-vol"] != "/mnt/models" {
			t.Errorf("model-vol mount missing or wrong path")
		}
		if mountMap["custom-certs"] != "/etc/certs" {
			t.Errorf("custom-certs mount missing or wrong path")
		}
	})

	t.Run("user overrides existing env var", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		ctrl.Env = []corev1.EnvVar{{Name: "SHARED_VAR", Value: "default"}}
		user := corev1.Container{
			Name: constants.StorageInitializerContainerName,
			Env:  []corev1.EnvVar{{Name: "SHARED_VAR", Value: "overridden"}},
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var found bool
		for _, e := range merged.Env {
			if e.Name == "SHARED_VAR" {
				found = true
				if e.Value != "overridden" {
					t.Errorf("SHARED_VAR = %q, want %q", e.Value, "overridden")
				}
			}
		}
		if !found {
			t.Error("SHARED_VAR not found in merged env")
		}
	})

	t.Run("controller args preserved even if user sets them", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		user := corev1.Container{
			Name: constants.StorageInitializerContainerName,
			Args: []string{"should-be-ignored", "/wrong/path"},
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if diff := cmp.Diff([]string{"hf://model", "/mnt/models"}, merged.Args); diff != "" {
			t.Errorf("args should be controller's, not user's (-want +got):\n%s", diff)
		}
	})

	t.Run("controller name preserved even if user sets different name", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		user := corev1.Container{
			Name:  "different-name",
			Image: "custom:v1",
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if merged.Name != constants.StorageInitializerContainerName {
			t.Errorf("name = %q, want %q", merged.Name, constants.StorageInitializerContainerName)
		}
	})

	t.Run("user passes HF_XET and TOKIO env vars to tune storage-initializer", func(t *testing.T) {
		t.Parallel()
		ctrl := defaultContainer()
		ctrl.Env = []corev1.EnvVar{{Name: "DEFAULT_VAR", Value: "default"}}
		user := corev1.Container{
			Name: constants.StorageInitializerContainerName,
			Env: []corev1.EnvVar{
				{Name: "HF_XET_HIGH_PERFORMANCE", Value: "0"},
				{Name: "TOKIO_WORKER_THREADS", Value: "1"},
			},
		}

		merged, err := mergeStorageInitForTest(ctrl, user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		envMap := make(map[string]string)
		for _, e := range merged.Env {
			envMap[e.Name] = e.Value
		}
		if envMap["DEFAULT_VAR"] != "default" {
			t.Errorf("DEFAULT_VAR = %q, want %q", envMap["DEFAULT_VAR"], "default")
		}
		if envMap["HF_XET_HIGH_PERFORMANCE"] != "0" {
			t.Errorf("HF_XET_HIGH_PERFORMANCE = %q, want %q", envMap["HF_XET_HIGH_PERFORMANCE"], "0")
		}
		if envMap["TOKIO_WORKER_THREADS"] != "1" {
			t.Errorf("TOKIO_WORKER_THREADS = %q, want %q", envMap["TOKIO_WORKER_THREADS"], "1")
		}
		if merged.Name != constants.StorageInitializerContainerName {
			t.Errorf("name = %q, want %q", merged.Name, constants.StorageInitializerContainerName)
		}
		if diff := cmp.Diff(ctrl.Args, merged.Args); diff != "" {
			t.Errorf("args mismatch (-want +got):\n%s", diff)
		}
	})
}
