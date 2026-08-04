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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

// TestAttachModelArtifacts_PVCWithLoRAAndSpeculator_MountOrder reproduces the scenario from
// PR #5687 review feedback: a pvc:// base model combined with an hf:// LoRA adapter and an
// hf:// speculator model. Once both are present, the shared storage-initializer emptyDir's
// common parent path collapses from a per-adapter/speculator path to "/mnt", which is a
// strict ancestor of the base model's own PVC mount at "/mnt/models". If the shallower "/mnt"
// mount were to appear after "/mnt/models" in the container's VolumeMounts list, the base
// model would be shadowed and invisible to the container at runtime. This test pins the fix:
// the shared parent mount must always be ordered before the deeper base-model mount.
func TestAttachModelArtifacts_PVCWithLoRAAndSpeculator_MountOrder(t *testing.T) {
	t.Parallel()

	modelURI, _ := apis.ParseURL("pvc://my-pvc/model-path")
	speculatorURI, _ := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")

	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *speculatorURI},
				Config: map[string]string{"method": "eagle3", "num_speculative_tokens": "3"},
			},
		},
	}

	config := &Config{
		StorageConfig: &kserveTypes.StorageInitializerConfig{
			CpuLimit:      "1",
			CpuRequest:    "100m",
			MemoryLimit:   "1Gi",
			MemoryRequest: "100Mi",
		},
		CredentialConfig: &credentials.CredentialConfig{},
		ResolvedLoRAAdapters: []resolvedLoRAAdapter{
			{
				name:      "adapter1",
				mountPath: "/mnt/lora/adapter1",
				uri:       "hf://org/adapter1",
				scheme:    constants.HfURIPrefix,
			},
		},
	}

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachModelArtifacts(t.Context(), serviceAccount, llmSvc, corev1.PodSpec{}, podSpec, config, "main", constants.DefaultModelLocalMountPath, true, true)
	require.NoError(t, err)

	mainContainer := getContainerByName(podSpec, "main")
	require.NotNil(t, mainContainer)

	pvcMountIdx, sharedMountIdx := -1, -1
	for i, vm := range mainContainer.VolumeMounts {
		switch vm.Name {
		case constants.PvcSourceMountName:
			pvcMountIdx = i
			assert.Equal(t, constants.DefaultModelLocalMountPath, vm.MountPath)
		case constants.StorageInitializerVolumeName:
			sharedMountIdx = i
			assert.Equal(t, "/mnt", vm.MountPath, "common parent of /mnt/lora/adapter1 and /mnt/speculator/model is /mnt")
		}
	}

	require.NotEqual(t, -1, pvcMountIdx, "expected a mount for the base PVC model")
	require.NotEqual(t, -1, sharedMountIdx, "expected a shared mount for LoRA/speculator downloads")
	assert.Less(t, sharedMountIdx, pvcMountIdx,
		"the shallower shared mount (/mnt) must be ordered before the deeper base model mount (/mnt/models), "+
			"otherwise mounting /mnt afterward would shadow /mnt/models")
}

func TestResolveVolumeMountOverlap(t *testing.T) {
	t.Parallel()

	t.Run("reorders shallower mount before deeper mount", func(t *testing.T) {
		t.Parallel()
		container := &corev1.Container{
			Name: "main",
			VolumeMounts: []corev1.VolumeMount{
				{Name: "kserve-pvc-source", MountPath: "/mnt/models"},
				{Name: "kserve-provision-location", MountPath: "/mnt"},
			},
		}

		err := resolveVolumeMountOverlap(container)
		require.NoError(t, err)

		require.Len(t, container.VolumeMounts, 2)
		assert.Equal(t, "/mnt", container.VolumeMounts[0].MountPath)
		assert.Equal(t, "/mnt/models", container.VolumeMounts[1].MountPath)
	})

	t.Run("rejects two different volumes at the exact same path", func(t *testing.T) {
		t.Parallel()
		container := &corev1.Container{
			Name: "main",
			VolumeMounts: []corev1.VolumeMount{
				{Name: "modelcar-volume", MountPath: "/mnt"},
				{Name: "kserve-provision-location", MountPath: "/mnt"},
			},
		}

		err := resolveVolumeMountOverlap(container)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/mnt")
		assert.Contains(t, err.Error(), "modelcar-volume")
		assert.Contains(t, err.Error(), "kserve-provision-location")
	})

	t.Run("leaves non-overlapping mounts unchanged", func(t *testing.T) {
		t.Parallel()
		container := &corev1.Container{
			Name: "main",
			VolumeMounts: []corev1.VolumeMount{
				{Name: "a", MountPath: "/mnt/lora/adapter1"},
				{Name: "b", MountPath: "/mnt/speculator/model"},
			},
		}

		err := resolveVolumeMountOverlap(container)
		require.NoError(t, err)

		require.Len(t, container.VolumeMounts, 2)
		assert.Equal(t, "/mnt/lora/adapter1", container.VolumeMounts[0].MountPath)
		assert.Equal(t, "/mnt/speculator/model", container.VolumeMounts[1].MountPath)
	})

	t.Run("nil container is a no-op", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, resolveVolumeMountOverlap(nil))
	})
}
