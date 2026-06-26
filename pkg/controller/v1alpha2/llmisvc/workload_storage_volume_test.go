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
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

// baseStorageConfig returns a minimal StorageInitializerConfig suitable for unit tests.
func baseStorageConfig() *kserveTypes.StorageInitializerConfig {
	return &kserveTypes.StorageInitializerConfig{
		Image:         "kserve/storage-initializer:latest",
		CpuRequest:    "100m",
		CpuLimit:      "1",
		MemoryRequest: "256Mi",
		MemoryLimit:   "1Gi",
	}
}

func findVolume(podSpec *corev1.PodSpec, name string) *corev1.Volume {
	for i := range podSpec.Volumes {
		if podSpec.Volumes[i].Name == name {
			return &podSpec.Volumes[i]
		}
	}
	return nil
}

// TestAttachStorageInitializer_ModelVolumeSource verifies that
// attachStorageInitializer passes StorageConfig.ModelVolumeSource through to
// the pod volume, covering all three storage scenarios.
func TestAttachStorageInitializer_ModelVolumeSource(t *testing.T) {
	storageClassName := "fast-nvme"

	tests := []struct {
		name              string
		modelVolumeSource *corev1.VolumeSource
		checkVolume       func(t *testing.T, vol *corev1.Volume)
	}{
		{
			name:              "nil ModelVolumeSource uses emptyDir",
			modelVolumeSource: nil,
			checkVolume: func(t *testing.T, vol *corev1.Volume) {
				require.NotNil(t, vol.EmptyDir, "expected emptyDir")
				assert.Nil(t, vol.Ephemeral)
				assert.Nil(t, vol.PersistentVolumeClaim)
			},
		},
		{
			name: "ephemeral VolumeClaimTemplate",
			modelVolumeSource: &corev1.VolumeSource{
				Ephemeral: &corev1.EphemeralVolumeSource{
					VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							StorageClassName: &storageClassName,
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("50Gi"),
								},
							},
						},
					},
				},
			},
			checkVolume: func(t *testing.T, vol *corev1.Volume) {
				require.NotNil(t, vol.Ephemeral, "expected ephemeral volume")
				assert.Nil(t, vol.EmptyDir)
				assert.Equal(t, "fast-nvme", *vol.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName)
				assert.Equal(t, "50Gi", vol.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests.Storage().String())
			},
		},
		{
			name: "dedicated PVC",
			modelVolumeSource: &corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "model-cache-pvc",
				},
			},
			checkVolume: func(t *testing.T, vol *corev1.Volume) {
				require.NotNil(t, vol.PersistentVolumeClaim, "expected PVC volume")
				assert.Equal(t, "model-cache-pvc", vol.PersistentVolumeClaim.ClaimName)
				assert.Nil(t, vol.EmptyDir)
			},
		},
		{
			name: "per-service override: pre-existing volume is not replaced",
			modelVolumeSource: &corev1.VolumeSource{
				Ephemeral: &corev1.EphemeralVolumeSource{
					VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
					},
				},
			},
			checkVolume: func(t *testing.T, vol *corev1.Volume) {
				// The pod already had a user-defined volume; it must survive unchanged.
				require.NotNil(t, vol.PersistentVolumeClaim, "user-supplied PVC must be preserved")
				assert.Equal(t, "user-pvc", vol.PersistentVolumeClaim.ClaimName)
				assert.Nil(t, vol.Ephemeral, "ModelVolumeSource must not overwrite existing volume")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseStorageConfig()
			cfg.ModelVolumeSource = tc.modelVolumeSource

			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: constants.InferenceServiceContainerName},
				},
			}

			// For the per-service override test, pre-populate the volume.
			if tc.name == "per-service override: pre-existing volume is not replaced" {
				podSpec.Volumes = []corev1.Volume{
					{
						Name: constants.StorageInitializerVolumeName,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "user-pvc",
							},
						},
					},
				}
			}

			r := &LLMISVCReconciler{}
			err := r.attachStorageInitializer(
				"s3://bucket/model",
				corev1.PodSpec{},
				podSpec,
				cfg,
				nil,
				constants.InferenceServiceContainerName,
				constants.DefaultModelLocalMountPath,
			)
			require.NoError(t, err)

			vol := findVolume(podSpec, constants.StorageInitializerVolumeName)
			require.NotNil(t, vol, "kserve-provision-location volume must be present")
			tc.checkVolume(t, vol)
		})
	}
}
