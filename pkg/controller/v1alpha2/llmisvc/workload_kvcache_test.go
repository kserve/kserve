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
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestAttachKVCacheSecondaryTiers(t *testing.T) {
	tests := []struct {
		name          string
		secondary     []v1alpha2.SecondaryTierSpec
		containerName string
		wantVolumes   []corev1.Volume
		wantMounts    []corev1.VolumeMount
	}{
		{
			name:          "empty secondary list produces no volumes",
			secondary:     nil,
			containerName: "main",
			wantVolumes:   nil,
			wantMounts:    nil,
		},
		{
			name:          "nil fileSystem entry is skipped",
			secondary:     []v1alpha2.SecondaryTierSpec{{FileSystem: nil}},
			containerName: "main",
			wantVolumes:   nil,
			wantMounts:    nil,
		},
		{
			name:          "emptyDir tier creates emptyDir volume with sizeLimit",
			containerName: "main",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("100Gi")},
				}},
			},
			wantVolumes: []corev1.Volume{
				{
					Name: "kv-cache-secondary-0",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: ptr.To(resource.MustParse("100Gi")),
						},
					},
				},
			},
			wantMounts: []corev1.VolumeMount{
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/kv-cache-0"},
			},
		},
		{
			name:          "emptyDir tier respects custom mountPath",
			containerName: "main",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					MountPath: "/mnt/nvme",
					EmptyDir:  &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("50Gi")},
				}},
			},
			wantVolumes: []corev1.Volume{
				{
					Name: "kv-cache-secondary-0",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: ptr.To(resource.MustParse("50Gi")),
						},
					},
				},
			},
			wantMounts: []corev1.VolumeMount{
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/nvme"},
			},
		},
		{
			name:          "pvc tier creates ephemeral volume with hardcoded RWO",
			containerName: "main",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						StorageClassName: ptr.To("fast-nvme"),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("100Gi"),
							},
						},
					},
				}},
			},
			wantVolumes: []corev1.Volume{
				{
					Name: "kv-cache-secondary-0",
					VolumeSource: corev1.VolumeSource{
						Ephemeral: &corev1.EphemeralVolumeSource{
							VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
								Spec: corev1.PersistentVolumeClaimSpec{
									AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
									StorageClassName: ptr.To("fast-nvme"),
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("100Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			wantMounts: []corev1.VolumeMount{
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/kv-cache-0"},
			},
		},
		{
			name:          "ref tier creates PVC volume with subPath",
			containerName: "main",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					MountPath: "/mnt/shared",
					Ref:       &v1alpha2.PVCRefTierSpec{Name: "my-pvc", Path: "kv-cache/"},
				}},
			},
			wantVolumes: []corev1.Volume{
				{
					Name: "kv-cache-secondary-0",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-pvc",
						},
					},
				},
			},
			wantMounts: []corev1.VolumeMount{
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/shared", SubPath: "kv-cache/"},
			},
		},
		{
			name:          "multiple tiers get indexed names and default mountPaths",
			containerName: "main",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("100Gi")},
				}},
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("200Gi")},
				}},
			},
			wantVolumes: []corev1.Volume{
				{
					Name: "kv-cache-secondary-0",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: ptr.To(resource.MustParse("100Gi")),
						},
					},
				},
				{
					Name: "kv-cache-secondary-1",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: ptr.To(resource.MustParse("200Gi")),
						},
					},
				},
			},
			wantMounts: []corev1.VolumeMount{
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/kv-cache-0"},
				{Name: "kv-cache-secondary-1", MountPath: "/mnt/kv-cache-1"},
			},
		},
		{
			name:          "mixed backends: ref at index 0 and pvc at index 1",
			containerName: "main",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					MountPath: "/mnt/shared",
					Ref:       &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv/"},
				}},
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						StorageClassName: ptr.To("fast-nvme"),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("50Gi"),
							},
						},
					},
				}},
			},
			wantVolumes: []corev1.Volume{
				{
					Name: "kv-cache-secondary-0",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "shared-pvc",
						},
					},
				},
				{
					Name: "kv-cache-secondary-1",
					VolumeSource: corev1.VolumeSource{
						Ephemeral: &corev1.EphemeralVolumeSource{
							VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
								Spec: corev1.PersistentVolumeClaimSpec{
									AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
									StorageClassName: ptr.To("fast-nvme"),
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("50Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			wantMounts: []corev1.VolumeMount{
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/shared", SubPath: "kv/"},
				{Name: "kv-cache-secondary-1", MountPath: "/mnt/kv-cache-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main"},
				},
			}
			attachKVCacheSecondaryTiers(podSpec, tt.secondary, tt.containerName)

			if diff := cmp.Diff(tt.wantVolumes, podSpec.Volumes); diff != "" {
				t.Errorf("volumes mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantMounts, podSpec.Containers[0].VolumeMounts); diff != "" {
				t.Errorf("volumeMounts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
