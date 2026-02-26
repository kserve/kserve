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
	"fmt"
	"path"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

func testStorageConfig() *kserveTypes.StorageInitializerConfig {
	return &kserveTypes.StorageInitializerConfig{
		CpuRequest:     "100m",
		CpuLimit:       "1",
		MemoryRequest:  "100Mi",
		MemoryLimit:    "1Gi",
		CpuModelcar:    "10m",
		MemoryModelcar: "15Mi",
	}
}

func TestAttachStorageInitializer_TargetContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		modelPath     string
		wantDestArg   string              // expected second arg of storage-initializer
		wantMount     *corev1.VolumeMount // expected volume mount on target container
	}{
		{
			name:          "tokenizer with flat model name",
			containerName: tokenizerContainerName,
			modelPath:     path.Join(constants.DefaultModelLocalMountPath, "my-llama"),
			wantDestArg:   constants.DefaultModelLocalMountPath,
			wantMount: &corev1.VolumeMount{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: path.Join(constants.DefaultModelLocalMountPath, "my-llama"),
				ReadOnly:  true,
			},
		},
		{
			name:          "tokenizer with slash model name",
			containerName: tokenizerContainerName,
			modelPath:     path.Join(constants.DefaultModelLocalMountPath, "meta-llama/Llama-2-7b"),
			wantDestArg:   constants.DefaultModelLocalMountPath,
			wantMount: &corev1.VolumeMount{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: path.Join(constants.DefaultModelLocalMountPath, "meta-llama/Llama-2-7b"),
				ReadOnly:  true,
			},
		},
		{
			name:          "main container with default path",
			containerName: "main",
			modelPath:     constants.DefaultModelLocalMountPath,
			wantDestArg:   constants.DefaultModelLocalMountPath,
			wantMount: &corev1.VolumeMount{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: constants.DefaultModelLocalMountPath,
				ReadOnly:  true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: tc.containerName},
				},
			}

			err := r.attachStorageInitializer(
				"hf://meta-llama/Llama-2-7b",
				corev1.PodSpec{}, // empty curr
				podSpec,
				testStorageConfig(),
				tc.containerName,
				tc.modelPath,
			)
			if err != nil {
				t.Fatalf("attachStorageInitializer returned error: %v", err)
			}

			// Check init container dest arg
			initContainer := findContainer(podSpec.InitContainers, constants.StorageInitializerContainerName)
			if initContainer == nil {
				t.Fatal("storage-initializer init container not found")
			}
			if len(initContainer.Args) < 2 {
				t.Fatal("storage-initializer should have at least 2 args")
			}
			if initContainer.Args[1] != tc.wantDestArg {
				t.Errorf("storage-initializer dest arg = %q, want %q", initContainer.Args[1], tc.wantDestArg)
			}

			// Check target container volume mount
			container := findContainer(podSpec.Containers, tc.containerName)
			if container == nil {
				t.Fatalf("container %q not found", tc.containerName)
			}
			found := false
			for _, vm := range container.VolumeMounts {
				if vm.Name == tc.wantMount.Name &&
					vm.MountPath == tc.wantMount.MountPath &&
					vm.ReadOnly == tc.wantMount.ReadOnly {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected volume mount %+v not found on %s, got %+v",
					*tc.wantMount, tc.containerName, container.VolumeMounts)
			}
		})
	}
}

func TestAttachPVCModelArtifact_TargetContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		modelPath     string
		wantMount     *corev1.VolumeMount
	}{
		{
			name:          "tokenizer with flat model name",
			containerName: tokenizerContainerName,
			modelPath:     path.Join(constants.DefaultModelLocalMountPath, "my-llama"),
			wantMount: &corev1.VolumeMount{
				Name:      constants.PvcSourceMountName,
				MountPath: path.Join(constants.DefaultModelLocalMountPath, "my-llama"),
				SubPath:   "opt-125m",
				ReadOnly:  true,
			},
		},
		{
			name:          "tokenizer with slash model name",
			containerName: tokenizerContainerName,
			modelPath:     path.Join(constants.DefaultModelLocalMountPath, "facebook/opt-125m"),
			wantMount: &corev1.VolumeMount{
				Name:      constants.PvcSourceMountName,
				MountPath: path.Join(constants.DefaultModelLocalMountPath, "facebook/opt-125m"),
				SubPath:   "opt-125m",
				ReadOnly:  true,
			},
		},
		{
			name:          "main container with default path",
			containerName: "main",
			modelPath:     constants.DefaultModelLocalMountPath,
			wantMount: &corev1.VolumeMount{
				Name:      constants.PvcSourceMountName,
				MountPath: constants.DefaultModelLocalMountPath,
				SubPath:   "opt-125m",
				ReadOnly:  true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: tc.containerName},
				},
			}

			err := r.attachPVCModelArtifact("pvc://my-pvc/opt-125m", podSpec, tc.containerName, tc.modelPath)
			if err != nil {
				t.Fatalf("attachPVCModelArtifact returned error: %v", err)
			}

			container := findContainer(podSpec.Containers, tc.containerName)
			if container == nil {
				t.Fatalf("container %q not found", tc.containerName)
			}
			found := false
			for _, vm := range container.VolumeMounts {
				if vm.Name == tc.wantMount.Name &&
					vm.MountPath == tc.wantMount.MountPath &&
					vm.SubPath == tc.wantMount.SubPath &&
					vm.ReadOnly == tc.wantMount.ReadOnly {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected volume mount %+v not found on %s, got %+v",
					*tc.wantMount, tc.containerName, container.VolumeMounts)
			}
		})
	}
}

func TestAttachOciModelArtifact_TargetContainer(t *testing.T) {
	tests := []struct {
		name             string
		containerName    string
		modelPath        string
		wantMount        *corev1.VolumeMount // expected volume mount on target container
		wantModelcarArgs []string            // expected modelcar container args
	}{
		{
			name:          "tokenizer with flat model name",
			containerName: tokenizerContainerName,
			modelPath:     path.Join(constants.DefaultModelLocalMountPath, "my-llama"),
			wantMount: &corev1.VolumeMount{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: utils.GetParentDirectory(path.Join(constants.DefaultModelLocalMountPath, "my-llama")),
				ReadOnly:  false,
			},
			wantModelcarArgs: []string{
				"sh", "-c",
				fmt.Sprintf("mkdir -p %s && ln -sf /proc/$$$$/root/models %s && sleep infinity",
					constants.DefaultModelLocalMountPath,
					path.Join(constants.DefaultModelLocalMountPath, "my-llama")),
			},
		},
		{
			name:          "tokenizer with slash model name",
			containerName: tokenizerContainerName,
			modelPath:     path.Join(constants.DefaultModelLocalMountPath, "meta-llama/Llama-2-7b"),
			wantMount: &corev1.VolumeMount{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: utils.GetParentDirectory(path.Join(constants.DefaultModelLocalMountPath, "meta-llama/Llama-2-7b")),
				ReadOnly:  false,
			},
			wantModelcarArgs: []string{
				"sh", "-c",
				fmt.Sprintf("mkdir -p %s && ln -sf /proc/$$$$/root/models %s && sleep infinity",
					path.Join(constants.DefaultModelLocalMountPath, "meta-llama"),
					path.Join(constants.DefaultModelLocalMountPath, "meta-llama/Llama-2-7b")),
			},
		},
		{
			name:          "main container with default path",
			containerName: "main",
			modelPath:     constants.DefaultModelLocalMountPath,
			wantMount: &corev1.VolumeMount{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: utils.GetParentDirectory(constants.DefaultModelLocalMountPath),
				ReadOnly:  false,
			},
			wantModelcarArgs: []string{
				"sh", "-c",
				fmt.Sprintf("ln -sf /proc/$$$$/root/models %s && sleep infinity",
					constants.DefaultModelLocalMountPath),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: tc.containerName},
				},
			}

			err := r.attachOciModelArtifact("oci://ghcr.io/test/model:v1", podSpec, testStorageConfig(), tc.containerName, tc.modelPath)
			if err != nil {
				t.Fatalf("attachOciModelArtifact returned error: %v", err)
			}

			// Check target container volume mount
			container := findContainer(podSpec.Containers, tc.containerName)
			if container == nil {
				t.Fatalf("container %q not found", tc.containerName)
			}
			found := false
			for _, vm := range container.VolumeMounts {
				if vm.Name == tc.wantMount.Name &&
					vm.MountPath == tc.wantMount.MountPath &&
					vm.ReadOnly == tc.wantMount.ReadOnly {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected volume mount %+v not found on %s, got %+v",
					*tc.wantMount, tc.containerName, container.VolumeMounts)
			}

			// Check modelcar container args
			modelcarContainer := findContainer(podSpec.Containers, constants.ModelcarContainerName)
			if modelcarContainer == nil {
				t.Fatal("modelcar container not found")
			}
			if len(modelcarContainer.Args) != len(tc.wantModelcarArgs) {
				t.Fatalf("modelcar args length = %d, want %d", len(modelcarContainer.Args), len(tc.wantModelcarArgs))
			}
			for i, arg := range modelcarContainer.Args {
				if arg != tc.wantModelcarArgs[i] {
					t.Errorf("modelcar args[%d] = %q, want %q", i, arg, tc.wantModelcarArgs[i])
				}
			}
		})
	}
}

func TestIsUsingTokenizerSidecar(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha2.LLMInferenceServiceSpec
		want bool
	}{
		{
			name: "tokenizer container present",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: tokenizerContainerName},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "tokenizer container among others",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "scheduler"},
								{Name: tokenizerContainerName},
								{Name: "other"},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "no tokenizer container",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "scheduler"},
								{Name: "other"},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "empty containers list",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "nil template",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{},
				},
			},
			want: false,
		},
		{
			name: "nil scheduler",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{},
			},
			want: false,
		},
		{
			name: "nil router",
			spec: v1alpha2.LLMInferenceServiceSpec{},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isUsingTokenizerSidecar(tc.spec)
			if got != tc.want {
				t.Errorf("isUsingTokenizerSidecar() = %v, want %v", got, tc.want)
			}
		})
	}
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}
