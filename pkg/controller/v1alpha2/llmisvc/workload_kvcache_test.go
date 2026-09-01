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
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

func TestApplyKVCacheCarriers(t *testing.T) {
	kvSpec := &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")}

	podWithSlot := func() *corev1.PodSpec {
		return &corev1.PodSpec{Containers: []corev1.Container{{
			Name: mainContainerName,
			Env:  []corev1.EnvVar{{Name: kvTransferArgsEnvVar}, {Name: "HOME", Value: "/home"}},
		}}}
	}

	t.Run("fills the slot from the merged spec", func(t *testing.T) {
		podSpec := podWithSlot()
		cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{KVCacheOffloading: kvSpec, Template: podSpec},
		}}
		if err := applyKVCacheCarriers(cfg); err != nil {
			t.Fatalf("applyKVCacheCarriers() error = %v", err)
		}
		got, filled := utils.GetEnvVarValue(podSpec.Containers[0].Env, kvTransferArgsEnvVar)
		if !filled {
			t.Fatal("slot not filled")
		}
		want := `--kv-transfer-config '{"kv_connector":"OffloadingConnector",` +
			`"kv_connector_extra_config":{"cpu_bytes_to_use":10737418240,"spec_name":"CPUOffloadingSpec"},` +
			`"kv_role":"kv_both"}'`
		if got != want {
			t.Errorf("slot value:\n got  %s\n want %s", got, want)
		}
	})

	t.Run("strips the slot when offloading is not configured", func(t *testing.T) {
		podSpec := podWithSlot()
		cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{Template: podSpec},
		}}
		if err := applyKVCacheCarriers(cfg); err != nil {
			t.Fatalf("applyKVCacheCarriers() error = %v", err)
		}
		if got, present := utils.GetEnvVarValue(podSpec.Containers[0].Env, kvTransferArgsEnvVar); present {
			t.Errorf("slot left behind: %q", got)
		}
		if len(podSpec.Containers[0].Env) != 1 {
			t.Errorf("unrelated env vars disturbed: %v", podSpec.Containers[0].Env)
		}
	})

	t.Run("fills worker and prefill slots from their own specs", func(t *testing.T) {
		template, worker, prefill := podWithSlot(), podWithSlot(), podWithSlot()
		cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{KVCacheOffloading: kvSpec, Template: template, Worker: worker},
			Prefill: &v1alpha2.WorkloadSpec{
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("5Gi")},
				Template:          prefill,
			},
		}}
		if err := applyKVCacheCarriers(cfg); err != nil {
			t.Fatalf("applyKVCacheCarriers() error = %v", err)
		}
		for name, want := range map[string]string{
			"template": "10737418240", "worker": "10737418240", "prefill": "5368709120",
		} {
			podSpec := map[string]*corev1.PodSpec{"template": template, "worker": worker, "prefill": prefill}[name]
			got, _ := utils.GetEnvVarValue(podSpec.Containers[0].Env, kvTransferArgsEnvVar)
			if !strings.Contains(got, `"cpu_bytes_to_use":`+want) {
				t.Errorf("%s: want cpu_bytes_to_use %s, got %q", name, want, got)
			}
		}
	})

	// The upgrade contract: a preset that renders the argument itself must come
	// out untouched, so pinning it pins the pod spec it produces.
	t.Run("preset without the slot is bit-for-bit unchanged", func(t *testing.T) {
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:         mainContainerName,
				Env:          []corev1.EnvVar{{Name: "HOME", Value: "/home"}},
				VolumeMounts: []corev1.VolumeMount{{Name: "dshm", MountPath: "/dev/shm"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("32Gi")},
					Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Gi")},
				},
			}},
			Volumes: []corev1.Volume{{
				Name: "dshm",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory, SizeLimit: ptr.To(resource.MustParse("1Gi")),
				}},
			}},
		}
		want := podSpec.DeepCopy()
		for _, kv := range []*v1alpha2.KVCacheOffloadingSpec{
			nil,
			kvSpec,
			{CPU: resource.MustParse("10Gi"), EvictionPolicy: "lru"},
			{CPU: resource.MustParse("10Gi"), Secondary: []v1alpha2.SecondaryTierSpec{{
				FileSystem: &v1alpha2.FileSystemTierSpec{EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("100Gi")}},
			}}},
		} {
			cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{KVCacheOffloading: kv, Template: podSpec},
			}}
			if err := applyKVCacheCarriers(cfg); err != nil {
				t.Fatalf("applyKVCacheCarriers() error = %v", err)
			}
			if diff := cmp.Diff(want, podSpec); diff != "" {
				t.Fatalf("preset without the slot mutated (-want +got):\n%s", diff)
			}
		}
	})

	// The entrypoint expands the slot inside eval, so the payload must survive as a
	// single shell word whatever it contains. evictionPolicy is the only
	// user-controlled string in it today and a CRD enum keeps it to lru/arc; this
	// asserts the carrier does not quietly depend on that enum staying put.
	t.Run("a quote in the payload cannot break out of the eval", func(t *testing.T) {
		const nasty = `x'$(touch /tmp/pwned)'y`
		podSpec := podWithSlot()
		cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					CPU:            resource.MustParse("10Gi"),
					EvictionPolicy: nasty,
				},
				Template: podSpec,
			},
		}}
		if err := applyKVCacheCarriers(cfg); err != nil {
			t.Fatalf("applyKVCacheCarriers() error = %v", err)
		}
		got, filled := utils.GetEnvVarValue(podSpec.Containers[0].Env, kvTransferArgsEnvVar)
		if !filled {
			t.Fatal("slot not filled")
		}
		quoted, ok := strings.CutPrefix(got, "--kv-transfer-config ")
		if !ok {
			t.Fatalf("slot does not carry the flag: %s", got)
		}
		body, openOK := strings.CutPrefix(quoted, "'")
		interior, closeOK := strings.CutSuffix(body, "'")
		if !openOK || !closeOK {
			t.Fatalf("payload is not a single quoted word: %s", quoted)
		}
		// Every quote inside must belong to a close-escape-reopen sequence. A bare one
		// left over would end the word and hand the rest of the payload to the shell.
		if bare := strings.ReplaceAll(interior, `'\''`, ""); strings.Contains(bare, "'") {
			t.Errorf("bare quote survives, giving command execution: %s", got)
		}
		// Lossless: undo the shell quoting and the JSON must come back intact.
		unquoted := strings.ReplaceAll(interior, `'\''`, "'")
		var payload struct {
			Extra struct {
				EvictionPolicy string `json:"eviction_policy"`
			} `json:"kv_connector_extra_config"`
		}
		if err := json.Unmarshal([]byte(unquoted), &payload); err != nil {
			t.Fatalf("quoting is lossy, JSON no longer parses: %v\n%s", err, unquoted)
		}
		if payload.Extra.EvictionPolicy != nasty {
			t.Errorf("eviction_policy round-trip:\n got  %q\n want %q", payload.Extra.EvictionPolicy, nasty)
		}
	})

	// Runtimes let the last duplicate env entry win, so a second empty slot would
	// silently discard the value written to the first.
	t.Run("a duplicate slot is collapsed rather than left to shadow the fill", func(t *testing.T) {
		podSpec := podWithSlot()
		c := &podSpec.Containers[0]
		c.Env = append(c.Env, corev1.EnvVar{Name: kvTransferArgsEnvVar})
		cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{KVCacheOffloading: kvSpec, Template: podSpec},
		}}
		if err := applyKVCacheCarriers(cfg); err != nil {
			t.Fatalf("applyKVCacheCarriers() error = %v", err)
		}
		var slots []corev1.EnvVar
		for _, env := range podSpec.Containers[0].Env {
			if env.Name == kvTransferArgsEnvVar {
				slots = append(slots, env)
			}
		}
		if len(slots) != 1 {
			t.Fatalf("want the slot collapsed to one entry, got %d: %v", len(slots), slots)
		}
		if !strings.Contains(slots[0].Value, "cpu_bytes_to_use") {
			t.Errorf("surviving slot is not the filled one: %#v", slots[0])
		}
		if podSpec.Containers[0].Env[0].Name != kvTransferArgsEnvVar {
			t.Errorf("env order disturbed, which is a rollout: %v", podSpec.Containers[0].Env)
		}
	})

	// env merges by name, so a user override lands on the preset's slot entry.
	// Writing Value alongside their ValueFrom builds an EnvVar the apiserver rejects
	// only when the workload is written, with nothing on the service to explain it.
	t.Run("a slot carrying valueFrom is reported, not half-written", func(t *testing.T) {
		podSpec := podWithSlot()
		podSpec.Containers[0].Env[0].ValueFrom = &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "args"},
		}
		cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{KVCacheOffloading: kvSpec, Template: podSpec},
		}}
		err := applyKVCacheCarriers(cfg)
		if err == nil {
			t.Fatal("want an error naming the conflict, got nil")
		}
		for _, want := range []string{kvTransferArgsEnvVar, "valueFrom"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should name %q, got: %v", want, err)
			}
		}
		if v := podSpec.Containers[0].Env[0].Value; v != "" {
			t.Errorf("conflicting entry was half-written: %q", v)
		}
	})

	t.Run("nil pod specs do not panic", func(t *testing.T) {
		if err := applyKVCacheCarriers(&v1alpha2.LLMInferenceServiceConfig{}); err != nil {
			t.Fatalf("applyKVCacheCarriers() error = %v", err)
		}
	})

	// A degenerate cpu is admission's problem, not the carrier's. Failing here
	// would turn a spec the API already accepted into a terminal reconcile error,
	// stopping a workload that ran fine before the slot existed.
	t.Run("degenerate cpu renders rather than failing the reconcile", func(t *testing.T) {
		for _, cpu := range []string{"0", "-1Gi", "18446744073709551616"} {
			t.Run(cpu, func(t *testing.T) {
				podSpec := podWithSlot()
				cfg := &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse(cpu)},
						Template:          podSpec,
					},
				}}
				if err := applyKVCacheCarriers(cfg); err != nil {
					t.Fatalf("applyKVCacheCarriers() error = %v", err)
				}
				got, _ := utils.GetEnvVarValue(podSpec.Containers[0].Env, kvTransferArgsEnvVar)
				if !strings.Contains(got, "cpu_bytes_to_use") {
					t.Fatalf("slot should carry a rendered argument, got %q", got)
				}
			})
		}
	})

	t.Run("spec name follows the tiers, not the caller", func(t *testing.T) {
		cpuOnly, err := kvTransferJSON(&v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")})
		if err != nil {
			t.Fatalf("kvTransferJSON() error = %v", err)
		}
		if !strings.Contains(cpuOnly, `"spec_name":"CPUOffloadingSpec"`) {
			t.Errorf("a CPU-only tier should render CPUOffloadingSpec, got %s", cpuOnly)
		}

		tiered, err := kvTransferJSON(&v1alpha2.KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []v1alpha2.SecondaryTierSpec{{
				FileSystem: &v1alpha2.FileSystemTierSpec{EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("100Gi")}},
			}},
		})
		if err != nil {
			t.Fatalf("kvTransferJSON() error = %v", err)
		}
		if !strings.Contains(tiered, `"spec_name":"TieringOffloadingSpec"`) {
			t.Errorf("secondary tiers should render TieringOffloadingSpec, got %s", tiered)
		}
	})
}

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
			name:          "pvc.spec tier creates ephemeral volume using full PVC spec",
			containerName: "main",
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
			name:          "pvc.ref tier creates PVC volume with subPath",
			containerName: "main",
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
			name:          "mixed backends: pvc.ref at index 0 and pvc.spec at index 1",
			containerName: "main",
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

func TestParseShmHeadroomPercent(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  int64
		wantK bool
	}{
		{"120", 120, true},
		{"100", 100, true}, // a preset asking for no headroom
		// A preset is not worth failing a reconcile over, so anything unusable
		// leaves the declared size alone rather than erroring.
		{"1.2x", 0, false},
		{"", 0, false},
		{"0", 0, false},
		{"-10", 0, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, ok := parseShmHeadroomPercent(tc.raw)
			if got != tc.want || ok != tc.wantK {
				t.Errorf("= (%d, %v), want (%d, %v)", got, ok, tc.want, tc.wantK)
			}
		})
	}
}

func TestApplyKVCacheShmSizing(t *testing.T) {
	shmPod := func(baseline *resource.Quantity, medium corev1.StorageMedium) *corev1.PodSpec {
		return &corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:         mainContainerName,
				VolumeMounts: []corev1.VolumeMount{{Name: "dshm", MountPath: sharedMemoryMountPath}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("32Gi")},
					Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Gi")},
				},
			}},
			Volumes: []corev1.Volume{{
				Name:         "dshm",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: medium, SizeLimit: baseline}},
			}},
		}
	}
	cfgWith := func(podSpec *corev1.PodSpec, cpu string) *v1alpha2.LLMInferenceServiceConfig {
		return &v1alpha2.LLMInferenceServiceConfig{Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template:          podSpec,
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse(cpu)},
			},
		}}
	}
	sizeLimitOf := func(podSpec *corev1.PodSpec) *resource.Quantity { return podSpec.Volumes[0].EmptyDir.SizeLimit }

	t.Run("adds the scaled tier to the preset baseline", func(t *testing.T) {
		for _, tc := range []struct {
			baseline, cpu string
			percent       int64
			want          string
		}{
			{"1Gi", "10Gi", 120, "13Gi"},   // single-node / prefill / decode
			{"8Gi", "10Gi", 120, "20Gi"},   // data-parallel leader and workers
			{"1Gi", "10Gi", 100, "11Gi"},   // a preset asking for no headroom
			{"1Gi", "200Gi", 120, "241Gi"}, // large tiers scale the same way
		} {
			t.Run(tc.baseline+"+"+tc.cpu, func(t *testing.T) {
				podSpec := shmPod(ptr.To(resource.MustParse(tc.baseline)), corev1.StorageMediumMemory)
				applyKVCacheShmSizing(cfgWith(podSpec, tc.cpu), tc.percent)
				if got := sizeLimitOf(podSpec).String(); got != tc.want {
					t.Errorf("sizeLimit = %s, want %s", got, tc.want)
				}
			})
		}
	})

	// The tier is charged to container memory, but the right limit also depends on
	// the model's working set, which the spec does not carry.
	t.Run("never touches container resources", func(t *testing.T) {
		podSpec := shmPod(ptr.To(resource.MustParse("1Gi")), corev1.StorageMediumMemory)
		want := podSpec.Containers[0].Resources.DeepCopy()
		applyKVCacheShmSizing(cfgWith(podSpec, "10Gi"), 120)
		if diff := cmp.Diff(want, &podSpec.Containers[0].Resources); diff != "" {
			t.Errorf("resources changed (-want +got):\n%s", diff)
		}
	})

	t.Run("leaves the volume alone when it is not ours to size", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			baseline *resource.Quantity
			medium   corev1.StorageMedium
			mutate   func(*corev1.PodSpec)
			want     string
		}{
			{name: "no size limit means no ceiling to raise", medium: corev1.StorageMediumMemory},
			{
				name:     "a disk-backed volume is not what the connector maps into",
				baseline: ptr.To(resource.MustParse("1Gi")), medium: corev1.StorageMediumDefault, want: "1Gi",
			},
			{
				name: "no shared-memory mount", baseline: ptr.To(resource.MustParse("1Gi")),
				medium: corev1.StorageMediumMemory, want: "1Gi",
				mutate: func(p *corev1.PodSpec) { p.Containers[0].VolumeMounts = nil },
			},
			{
				name: "no model server container", baseline: ptr.To(resource.MustParse("1Gi")),
				medium: corev1.StorageMediumMemory, want: "1Gi",
				mutate: func(p *corev1.PodSpec) { p.Containers[0].Name = "sidecar" },
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				podSpec := shmPod(tc.baseline, tc.medium)
				if tc.mutate != nil {
					tc.mutate(podSpec)
				}
				applyKVCacheShmSizing(cfgWith(podSpec, "10Gi"), 120)
				got := sizeLimitOf(podSpec)
				if tc.want == "" {
					if got != nil {
						t.Errorf("sizeLimit = %v, want nil", got)
					}
					return
				}
				if got.String() != tc.want {
					t.Errorf("sizeLimit = %s, want %s", got.String(), tc.want)
				}
			})
		}
	})

	t.Run("sizes worker and prefill pods from their own tiers", func(t *testing.T) {
		main := shmPod(ptr.To(resource.MustParse("1Gi")), corev1.StorageMediumMemory)
		worker := shmPod(ptr.To(resource.MustParse("8Gi")), corev1.StorageMediumMemory)
		prefill := shmPod(ptr.To(resource.MustParse("1Gi")), corev1.StorageMediumMemory)
		cfg := cfgWith(main, "10Gi")
		cfg.Spec.Worker = worker
		cfg.Spec.Prefill = &v1alpha2.WorkloadSpec{
			Template:          prefill,
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("5Gi")},
		}

		applyKVCacheShmSizing(cfg, 120)

		for _, tc := range []struct {
			name    string
			podSpec *corev1.PodSpec
			want    string
		}{
			{"template", main, "13Gi"},
			{"worker", worker, "20Gi"},
			{"prefill", prefill, "7Gi"},
		} {
			if got := sizeLimitOf(tc.podSpec).String(); got != tc.want {
				t.Errorf("%s sizeLimit = %s, want %s", tc.name, got, tc.want)
			}
		}
	})

	t.Run("does nothing without a tier", func(t *testing.T) {
		podSpec := shmPod(ptr.To(resource.MustParse("1Gi")), corev1.StorageMediumMemory)
		cfg := cfgWith(podSpec, "10Gi")
		cfg.Spec.KVCacheOffloading = nil
		applyKVCacheShmSizing(cfg, 120)
		if got := sizeLimitOf(podSpec).String(); got != "1Gi" {
			t.Errorf("sizeLimit = %s, want 1Gi", got)
		}
	})
}
