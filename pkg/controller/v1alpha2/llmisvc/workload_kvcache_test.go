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
		name        string
		secondary   []v1alpha2.SecondaryTierSpec
		wantVolumes []corev1.Volume
		wantMounts  []corev1.VolumeMount
	}{
		{
			name:        "empty secondary list produces no volumes",
			secondary:   nil,
			wantVolumes: nil,
			wantMounts:  nil,
		},
		{
			name:        "nil fileSystem entry is skipped",
			secondary:   []v1alpha2.SecondaryTierSpec{{FileSystem: nil}},
			wantVolumes: nil,
			wantMounts:  nil,
		},
		{
			name: "emptyDir tier creates emptyDir volume with sizeLimit",
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
			name: "pvc.spec tier creates ephemeral volume using full PVC spec",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Spec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: ptr.To("fast-nvme"),
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("100Gi"),
								},
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
									StorageClassName: ptr.To("fast-nvme"),
									AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
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
			name: "pvc.ref tier creates PVC volume with subPath",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "my-pvc", Path: "kv-cache/"},
					},
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
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/kv-cache-0", SubPath: "kv-cache/"},
			},
		},
		{
			name: "multiple tiers get indexed names and default mountPaths",
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
			name: "mixed backends: pvc.ref at index 0 and pvc.spec at index 1",
			secondary: []v1alpha2.SecondaryTierSpec{
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv/"},
					},
				}},
				{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Spec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: ptr.To("fast-nvme"),
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("50Gi"),
								},
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
									StorageClassName: ptr.To("fast-nvme"),
									AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
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
				{Name: "kv-cache-secondary-0", MountPath: "/mnt/kv-cache-0", SubPath: "kv/"},
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
			attachKVCacheSecondaryTiers(podSpec, tt.secondary, "main")

			if diff := cmp.Diff(tt.wantVolumes, podSpec.Volumes); diff != "" {
				t.Errorf("volumes mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantMounts, podSpec.Containers[0].VolumeMounts); diff != "" {
				t.Errorf("volumeMounts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// kvCachePodSpec builds a pod shaped like the base preset: a "main" container with a
// /dev/shm mount backed by a memory-medium emptyDir, plus optional memory req/limit.
// Any of shmSize/memReq/memLimit may be "" to omit that field.
func kvCachePodSpec(shmSize, memReq, memLimit string) *corev1.PodSpec {
	shm := &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}
	if shmSize != "" {
		shm.SizeLimit = ptr.To(resource.MustParse(shmSize))
	}
	res := corev1.ResourceRequirements{}
	if memReq != "" {
		res.Requests = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(memReq)}
	}
	if memLimit != "" {
		res.Limits = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(memLimit)}
	}
	return &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:         "main",
			Resources:    res,
			VolumeMounts: []corev1.VolumeMount{{Name: "dshm", MountPath: "/dev/shm"}},
		}},
		Volumes: []corev1.Volume{{
			Name:         "dshm",
			VolumeSource: corev1.VolumeSource{EmptyDir: shm},
		}},
	}
}

func TestAttachKVCachePrimaryTier(t *testing.T) {
	// wantQty compares a quantity field, treating "" as "expected absent/nil".
	wantQty := func(t *testing.T, label, want string, got *resource.Quantity) {
		t.Helper()
		if want == "" {
			if got != nil {
				t.Errorf("%s: expected absent, got %s", label, got.String())
			}
			return
		}
		if got == nil {
			t.Errorf("%s: expected %s, got absent", label, want)
			return
		}
		if w := resource.MustParse(want); got.Cmp(w) != 0 {
			t.Errorf("%s: expected %s, got %s", label, want, got.String())
		}
	}

	tests := []struct {
		name         string
		cpu          resource.Quantity
		podSpec      *corev1.PodSpec
		wantMount    bool
		wantShm      string
		wantMemReq   string
		wantMemLimit string
	}{
		{
			name:         "grows an explicitly-set /dev/shm by cpu*120% and bumps set memory req/limit",
			cpu:          resource.MustParse("10Gi"),
			podSpec:      kvCachePodSpec("1Gi", "32Gi", "64Gi"),
			wantMount:    true,
			wantShm:      "13Gi",
			wantMemReq:   "44Gi",
			wantMemLimit: "76Gi",
		},
		{
			name:         "no /dev/shm mount: does not create a mount or volume",
			cpu:          resource.MustParse("10Gi"),
			podSpec:      &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			wantMount:    false,
			wantShm:      "",
			wantMemReq:   "",
			wantMemLimit: "",
		},
		{
			name:         "resources memory limit set but request unset: bumps only the limit",
			cpu:          resource.MustParse("10Gi"),
			podSpec:      kvCachePodSpec("1Gi", "", "64Gi"),
			wantMount:    true,
			wantShm:      "13Gi",
			wantMemReq:   "",
			wantMemLimit: "76Gi",
		},
		{
			name:         "no memory set on resources, leaves both request and limit unset",
			cpu:          resource.MustParse("10Gi"),
			podSpec:      kvCachePodSpec("1Gi", "", ""),
			wantMount:    true,
			wantShm:      "13Gi",
			wantMemReq:   "",
			wantMemLimit: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachKVCachePrimaryTier(tt.podSpec, tt.cpu, "main")

			hasMount := false
			for _, m := range tt.podSpec.Containers[0].VolumeMounts {
				if m.MountPath == "/dev/shm" {
					hasMount = true
				}
			}
			if hasMount != tt.wantMount {
				t.Errorf("/dev/shm mount present = %v, want %v", hasMount, tt.wantMount)
			}

			var shm *resource.Quantity
			for i := range tt.podSpec.Volumes {
				if tt.podSpec.Volumes[i].Name == "dshm" && tt.podSpec.Volumes[i].EmptyDir != nil {
					shm = tt.podSpec.Volumes[i].EmptyDir.SizeLimit
				}
			}
			wantQty(t, "/dev/shm sizeLimit", tt.wantShm, shm)

			c := tt.podSpec.Containers[0]
			var memReq, memLimit *resource.Quantity
			if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memReq = &q
			}
			if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				memLimit = &q
			}
			wantQty(t, "memory request", tt.wantMemReq, memReq)
			wantQty(t, "memory limit", tt.wantMemLimit, memLimit)
		})
	}
}
