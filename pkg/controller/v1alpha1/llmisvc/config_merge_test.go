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

package llmisvc_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	ktesting "github.com/kserve/kserve/pkg/testing"

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

func TestMergeSpecs(t *testing.T) {
	tests := []struct {
		name    string
		cfgs    []v1alpha1.LLMInferenceServiceSpec
		want    v1alpha1.LLMInferenceServiceSpec
		wantErr bool
	}{
		{
			name:    "no configs",
			cfgs:    []v1alpha1.LLMInferenceServiceSpec{},
			want:    v1alpha1.LLMInferenceServiceSpec{},
			wantErr: false,
		},
		{
			name: "single config",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}}},
			},
			want:    v1alpha1.LLMInferenceServiceSpec{Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}}},
			wantErr: false,
		},
		{
			name: "two configs simple merge",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}}},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}},
			},
			wantErr: false,
		},
		{
			name: "two configs simple merge with sub-field override",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}},
					Router: &v1alpha1.RouterSpec{
						Route:     &v1alpha1.GatewayRoutesSpec{},
						Gateway:   &v1alpha1.GatewaySpec{},
						Scheduler: &v1alpha1.SchedulerSpec{},
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}},
					Router: &v1alpha1.RouterSpec{
						Scheduler: &v1alpha1.SchedulerSpec{
							Pool: &v1alpha1.InferencePoolSpec{
								Spec: &igwapi.InferencePoolSpec{
									TargetPortNumber: 9999,
								},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}},
				Router: &v1alpha1.RouterSpec{
					Route:   &v1alpha1.GatewayRoutesSpec{},
					Gateway: &v1alpha1.GatewaySpec{},
					Scheduler: &v1alpha1.SchedulerSpec{
						Pool: &v1alpha1.InferencePoolSpec{
							Spec: &igwapi.InferencePoolSpec{
								TargetPortNumber: 9999,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two configs with override",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-b"}},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](2),
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-b"}},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			wantErr: false,
		},
		{
			name: "three configs chained merge",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-a"}}},
				{
					Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-b"}},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: apis.URL{Path: "model-b"}},
			},
			wantErr: false,
		},
		{
			name: "deep merge with podspec template",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				// Base configuration
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
						Template: &corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "storage-initializer",
									Image: "kserve/storage-initializer:latest",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1Mi"),
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  "kserve-container",
									Image: "base:0.1",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("1"),
										},
									},
								},
							},
							Tolerations: []corev1.Toleration{
								{Key: "team", Operator: corev1.TolerationOpEqual, Value: "a"},
							},
						},
					},
				},
				// Override configuration
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](2),
						Template: &corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "storage-initializer",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1Gi"),
										},
									},
								},
							},
							Containers: []corev1.Container{
								// This container should replace the base one due to the same name
								{
									Name:  "kserve-container",
									Image: "override:1.0",
									Env: []corev1.EnvVar{
										{Name: "FOO", Value: "bar"},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"), // Override CPU
										},
									},
								},
								// This is a new container that should be added
								{
									Name:  "transformer",
									Image: "transformer:latest",
								},
							},
							// Tolerations should be REPLACED, not merged, as there is no patchMergeKey
							Tolerations: []corev1.Toleration{
								{Key: "gpu", Operator: corev1.TolerationOpExists},
							},
							PriorityClassName: "high-priority", // Add a new field
						},
					},
				},
			},
			// Expected result of the merge
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](2),
					Template: &corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								Name:  "storage-initializer",
								Image: "kserve/storage-initializer:latest", // Image is preserved from base
								Resources: corev1.ResourceRequirements{ // Resources are updated from override
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("1Gi"),
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:  "kserve-container",
								Image: "override:1.0",
								Env: []corev1.EnvVar{
									{Name: "FOO", Value: "bar"},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("2"),
									},
								},
							},
							{
								Name:  "transformer",
								Image: "transformer:latest",
							},
						},
						// Tolerations slice is replaced by the override
						Tolerations: []corev1.Toleration{
							{Key: "gpu", Operator: corev1.TolerationOpExists},
						},
						PriorityClassName: "high-priority",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "merge with prefill spec",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				// Base has only a decode workload
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
						Template: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "decode:0.1"}}},
					},
				},
				// Override adds a prefill workload
				{
					Prefill: &v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](4),
						Template: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "prefill:0.1"}}},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				// Base workload spec is preserved
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](1),
					Template: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "decode:0.1"}}},
				},
				// Prefill spec is added
				Prefill: &v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](4),
					Template: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "prefill:0.1"}}},
				},
			},
		},
		{
			name: "merge with worker spec",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				// Base has the main head/decode template
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "head:0.1"}}},
					},
				},
				// Override adds a worker spec
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Worker: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "worker:0.1"}}},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					// Head template is preserved
					Template: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "head:0.1"}}},
					// Worker spec is added
					Worker: &corev1.PodSpec{Containers: []corev1.Container{{Name: "kserve-container", Image: "worker:0.1"}}},
				},
			},
		},
		{
			name: "merge with parallelism spec",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				// Base defines tensor parallelism
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor: ptr.To[int32](2),
						},
					},
				},
				// Override defines pipeline parallelism
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Pipeline: ptr.To[int32](4),
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					// Both parallelism values should be present
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int32](2),
						Pipeline: ptr.To[int32](4),
					},
				},
			},
		},
		{
			name: "deep merge of prefill spec",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				// Base defines a prefill workload with replicas and a container with a resource request
				{
					Prefill: &v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](2),
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "prefill-container",
									Image: "prefill:0.1",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
									},
								},
							},
						},
					},
				},
				// Override changes replica count and adds an environment variable to the container
				{
					Prefill: &v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](4),
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "prefill-container",
									Env: []corev1.EnvVar{
										{Name: "PREFILL_MODE", Value: "FAST"},
									},
								},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Prefill: &v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](4), // Replicas are overridden
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "prefill-container",
								Image: "prefill:0.1", // Image is preserved from base
								Env: []corev1.EnvVar{ // Env var is added from override
									{Name: "PREFILL_MODE", Value: "FAST"},
								},
								Resources: corev1.ResourceRequirements{ // Resources are preserved from base
									Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "4 chained merge router, epp, multi node",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Router: &v1alpha1.RouterSpec{
						Route:   &v1alpha1.GatewayRoutesSpec{},
						Gateway: &v1alpha1.GatewaySpec{},
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor:   ptr.To[int32](1),
							Pipeline: ptr.To[int32](1),
						},
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("1"),
											"nvidia.com/gpu":   resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
				{
					Router: &v1alpha1.RouterSpec{
						Scheduler: &v1alpha1.SchedulerSpec{
							Pool: &v1alpha1.InferencePoolSpec{
								Spec: &igwapi.InferencePoolSpec{
									TargetPortNumber: 0,
									EndpointPickerConfig: igwapi.EndpointPickerConfig{
										ExtensionRef: &igwapi.Extension{
											ExtensionConnection: igwapi.ExtensionConnection{
												FailureMode: ptr.To(igwapi.FailClose),
											},
										},
									},
								},
							},
							Template: &corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "main",
									},
								},
							},
						},
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor:   ptr.To[int32](4),
							Pipeline: ptr.To[int32](2),
						},
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("1"),
											"nvidia.com/gpu":   resource.MustParse("4"),
										},
									},
								},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Router: &v1alpha1.RouterSpec{
					Route:   &v1alpha1.GatewayRoutesSpec{},
					Gateway: &v1alpha1.GatewaySpec{},
					Scheduler: &v1alpha1.SchedulerSpec{
						Pool: &v1alpha1.InferencePoolSpec{
							Spec: &igwapi.InferencePoolSpec{
								TargetPortNumber: 0,
								EndpointPickerConfig: igwapi.EndpointPickerConfig{
									ExtensionRef: &igwapi.Extension{
										ExtensionConnection: igwapi.ExtensionConnection{
											FailureMode: ptr.To(igwapi.FailClose),
										},
									},
								},
							},
						},
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
								},
							},
						},
					},
				},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int32](4),
						Pipeline: ptr.To[int32](2),
					},
					Worker: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
										"nvidia.com/gpu":   resource.MustParse("4"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "4 chained merge router with scheduler, http route and gateway ref, multi node",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Router: &v1alpha1.RouterSpec{
						Route: &v1alpha1.GatewayRoutesSpec{
							HTTP: &v1alpha1.HTTPRouteSpec{
								Spec: &gwapiv1.HTTPRouteSpec{
									CommonRouteSpec: gwapiv1.CommonRouteSpec{
										ParentRefs: []gwapiv1.ParentReference{
											{
												Name: "my-parent",
											},
										},
									},
									Hostnames: nil,
									Rules:     nil,
								},
								Refs: []corev1.LocalObjectReference{{Name: "my-route"}},
							},
						},
						Gateway: &v1alpha1.GatewaySpec{
							Refs: []v1alpha1.UntypedObjectReference{{Name: "my-gateway"}},
						},
					},
				},
				{
					Router: &v1alpha1.RouterSpec{
						Route: &v1alpha1.GatewayRoutesSpec{
							HTTP: &v1alpha1.HTTPRouteSpec{
								Refs: []corev1.LocalObjectReference{{Name: "my-second-route"}},
							},
						},
						Gateway: &v1alpha1.GatewaySpec{
							Refs: []v1alpha1.UntypedObjectReference{{Name: "my-second-gateway"}},
						},
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor:   ptr.To[int32](1),
							Pipeline: ptr.To[int32](1),
						},
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("1"),
											"nvidia.com/gpu":   resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
				{
					Router: &v1alpha1.RouterSpec{
						Scheduler: &v1alpha1.SchedulerSpec{
							Pool: &v1alpha1.InferencePoolSpec{
								Ref: &corev1.LocalObjectReference{
									Name: "my-pool",
								},
							},
						},
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor:   ptr.To[int32](4),
							Pipeline: ptr.To[int32](2),
						},
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("1"),
											"nvidia.com/gpu":   resource.MustParse("4"),
										},
									},
								},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Router: &v1alpha1.RouterSpec{
					Route: &v1alpha1.GatewayRoutesSpec{
						HTTP: &v1alpha1.HTTPRouteSpec{
							Spec: &gwapiv1.HTTPRouteSpec{
								CommonRouteSpec: gwapiv1.CommonRouteSpec{
									ParentRefs: []gwapiv1.ParentReference{
										{
											Name: "my-parent",
										},
									},
								},
								Hostnames: nil,
								Rules:     nil,
							},
							Refs: []corev1.LocalObjectReference{{Name: "my-second-route"}},
						},
					},
					Gateway: &v1alpha1.GatewaySpec{
						Refs: []v1alpha1.UntypedObjectReference{{Name: "my-second-gateway"}},
					},
					Scheduler: &v1alpha1.SchedulerSpec{
						Pool: &v1alpha1.InferencePoolSpec{
							Ref: &corev1.LocalObjectReference{
								Name: "my-pool",
							},
						},
					},
				},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int32](4),
						Pipeline: ptr.To[int32](2),
					},
					Worker: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
										"nvidia.com/gpu":   resource.MustParse("4"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "merge requests and limits",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Router: &v1alpha1.RouterSpec{
						Route:   &v1alpha1.GatewayRoutesSpec{},
						Gateway: &v1alpha1.GatewaySpec{},
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor:   ptr.To[int32](1),
							Pipeline: ptr.To[int32](1),
						},
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("1"),
											"nvidia.com/gpu":   resource.MustParse("1"),
										},
										Limits: corev1.ResourceList{
											"nvidia.com/gpu": resource.MustParse("1"),
										},
									},
									Env: []corev1.EnvVar{
										{Name: "a", Value: "1"},
										{Name: "z", Value: "42"},
									},
									Args: []string{
										"a", "b",
									},
								},
							},
						},
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("1Gi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"),
										},
									},
									Env: []corev1.EnvVar{
										{Name: "b", Value: "2"},
										{Name: "z", Value: ""},
									},
									Args: []string{
										"x", "y",
									},
								},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Router: &v1alpha1.RouterSpec{
					Route:   &v1alpha1.GatewayRoutesSpec{},
					Gateway: &v1alpha1.GatewaySpec{},
				},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int32](1),
						Pipeline: ptr.To[int32](1),
					},
					Worker: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
										"nvidia.com/gpu":      resource.MustParse("1"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("2"),
										"nvidia.com/gpu":   resource.MustParse("1"),
									},
								},
								Env: []corev1.EnvVar{
									{Name: "b", Value: "2"},
									{Name: "a", Value: "1"},
									{Name: "z", Value: "42"},
								},
								Args: []string{
									"x", "y",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "merge LoRA adapters",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{
						URI: apis.URL{Path: "base-model"},
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.LLMModelSpec{
								{URI: apis.URL{Path: "lora-model"}},
							},
						},
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.LLMModelSpec{
								{URI: apis.URL{Path: "lora-model2"}},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI: apis.URL{Path: "base-model"},
					LoRA: &v1alpha1.LoRASpec{
						Adapters: []v1alpha1.LLMModelSpec{
							{URI: apis.URL{Path: "lora-model2"}},
						},
					},
				},
			},
		},
		{
			name: "merge model criticality",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{
						URI:         apis.URL{Path: "model-uri"},
						Criticality: ptr.To(igwapi.Sheddable),
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						Criticality: ptr.To(igwapi.Critical),
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI:         apis.URL{Path: "model-uri"},
					Criticality: ptr.To(igwapi.Critical),
				},
			},
		},
		{
			name: "merge model URI",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{
						URI: apis.URL{Scheme: "hf", Host: "hub.com", Path: "/model-a"},
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						URI: apis.URL{Scheme: "s3", Host: "bucket.com", Path: "/model-b"},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI: apis.URL{Scheme: "s3", Host: "bucket.com", Path: "/model-b"},
				},
			},
		},
		{
			name: "merge baseRefs",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "base-config-1"},
						{Name: "base-config-2"},
					},
				},
				{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "override-config-1"},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				BaseRefs: []corev1.LocalObjectReference{
					{Name: "override-config-1"},
				},
			},
		},
		{
			name: "merge ingress spec",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Router: &v1alpha1.RouterSpec{
						Ingress: &v1alpha1.IngressSpec{
							Refs: []v1alpha1.UntypedObjectReference{
								{Name: "base-ingress", Namespace: "base-ns"},
							},
						},
					},
				},
				{
					Router: &v1alpha1.RouterSpec{
						Ingress: &v1alpha1.IngressSpec{
							Refs: []v1alpha1.UntypedObjectReference{
								{Name: "override-ingress", Namespace: "override-ns"},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Router: &v1alpha1.RouterSpec{
					Ingress: &v1alpha1.IngressSpec{
						Refs: []v1alpha1.UntypedObjectReference{
							{Name: "override-ingress", Namespace: "override-ns"},
						},
					},
				},
			},
		},
		{
			name: "merge with nil pointer handling",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{
						URI:  apis.URL{Path: "model-uri"},
						Name: ptr.To("base-name"),
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						Name: nil, // nil pointer should not override non-nil base
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](3),
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI:  apis.URL{Path: "model-uri"},
					Name: ptr.To("base-name"), // Base value should be preserved
				},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](3),
				},
			},
		},
		{
			name: "merge complex nested structures",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{
						URI:         apis.URL{Path: "base-model"},
						Name:        ptr.To("base-name"),
						Criticality: ptr.To(igwapi.Sheddable),
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.LLMModelSpec{
								{URI: apis.URL{Path: "lora-model"}},
							},
						},
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor: ptr.To[int32](2),
						},
					},
					Router: &v1alpha1.RouterSpec{
						Gateway: &v1alpha1.GatewaySpec{
							Refs: []v1alpha1.UntypedObjectReference{{Name: "base-gw"}},
						},
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						Name:        ptr.To("override-name"),
						Criticality: ptr.To(igwapi.Critical),
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.LLMModelSpec{
								{URI: apis.URL{Path: "lora-model2"}},
							},
						},
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](5),
						Parallelism: &v1alpha1.ParallelismSpec{
							Pipeline: ptr.To[int32](4),
						},
					},
					Router: &v1alpha1.RouterSpec{
						Route: &v1alpha1.GatewayRoutesSpec{
							HTTP: &v1alpha1.HTTPRouteSpec{
								Refs: []corev1.LocalObjectReference{{Name: "override-route"}},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI:         apis.URL{Path: "base-model"}, // Base URI preserved
					Name:        ptr.To("override-name"),      // Override name
					Criticality: ptr.To(igwapi.Critical),
					LoRA: &v1alpha1.LoRASpec{
						Adapters: []v1alpha1.LLMModelSpec{
							{URI: apis.URL{Path: "lora-model2"}},
						},
					},
				},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](5),
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int32](2), // Base tensor preserved
						Pipeline: ptr.To[int32](4), // Override pipeline
					},
				},
				Router: &v1alpha1.RouterSpec{
					Gateway: &v1alpha1.GatewaySpec{
						Refs: []v1alpha1.UntypedObjectReference{{Name: "base-gw"}},
					},
					Route: &v1alpha1.GatewayRoutesSpec{
						HTTP: &v1alpha1.HTTPRouteSpec{
							Refs: []corev1.LocalObjectReference{{Name: "override-route"}},
						},
					},
				},
			},
		},
		{
			name: "merge empty structures",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						URI: apis.URL{Path: "populated-model"},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI: apis.URL{Path: "populated-model"},
				},
			},
		},
		{
			name: "merge with zero values vs nil pointers",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](0), // Zero value, but non-nil pointer
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: nil, // Nil pointer should not override zero value
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](0), // Zero value should be preserved
				},
			},
		},
		{
			name: "merge pod spec with nil containers",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "busybox",
									Name:  "busybox",
								},
							},
							InitContainers: []corev1.Container{
								{
									Image: "busybox-init",
									Name:  "busybox-init",
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "vol1",
								},
							},
						},
						Replicas: ptr.To[int32](1),
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{},
						Replicas: nil,
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Image: "busybox",
								Name:  "busybox",
							},
						},
						InitContainers: []corev1.Container{
							{
								Image: "busybox-init",
								Name:  "busybox-init",
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "vol1",
							},
						},
					},
					Replicas: ptr.To[int32](1),
				},
			},
		},
		{
			name: "merge pod spec with empty containers",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "busybox",
									Name:  "busybox",
								},
							},
							InitContainers: []corev1.Container{
								{
									Image: "busybox-init",
									Name:  "busybox-init",
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "vol1",
								},
							},
						},
						Replicas: ptr.To[int32](1),
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{},
						},
						Replicas: nil,
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Image: "busybox",
								Name:  "busybox",
							},
						},
						InitContainers: []corev1.Container{
							{
								Image: "busybox-init",
								Name:  "busybox-init",
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "vol1",
							},
						},
					},
					Replicas: ptr.To[int32](1),
				},
			},
		},
		{
			name: "merge pod spec, add container",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "busybox",
									Name:  "busybox",
								},
							},
							InitContainers: []corev1.Container{
								{
									Image: "busybox-sidecar",
									Name:  "busybox-sidecar",
								},
							},
						},
						Replicas: ptr.To[int32](2),
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "busybox-2",
									Name:  "busybox-2",
								},
							},
							InitContainers: []corev1.Container{},
						},
						Replicas: nil,
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Image: "busybox-2",
								Name:  "busybox-2",
							},
							{
								Image: "busybox",
								Name:  "busybox",
							},
						},
						InitContainers: []corev1.Container{
							{
								Image: "busybox-sidecar",
								Name:  "busybox-sidecar",
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
		},
		{
			name: "merge pod spec, add container",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "busybox",
									Name:  "busybox",
								},
							},
							InitContainers: []corev1.Container{
								{
									Image: "busybox-sidecar",
									Name:  "busybox-sidecar",
								},
							},
						},
						Replicas: ptr.To[int32](2),
					},
				},
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "busybox-2",
									Name:  "busybox-2",
								},
							},
							InitContainers: []corev1.Container{},
						},
						Replicas: nil,
					},
				},
				{},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Image: "busybox-2",
								Name:  "busybox-2",
							},
							{
								Image: "busybox",
								Name:  "busybox",
							},
						},
						InitContainers: []corev1.Container{
							{
								Image: "busybox-sidecar",
								Name:  "busybox-sidecar",
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			ctx = log.IntoContext(ctx, pkgtest.NewTestLogger(t))

			got, err := llmisvc.MergeSpecs(ctx, tt.cfgs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("MergeSpecs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeSpecs() got = \n%#v\n, want \n%#v\nDiff (-want, +got):\n%s", got, tt.want, diff)
			}
		})
	}
}

func TestReplaceVariables(t *testing.T) {
	tests := []struct {
		name    string
		llmSvc  *v1alpha1.LLMInferenceService
		cfg     *v1alpha1.LLMInferenceServiceConfig
		extra   *llmisvc.Config
		want    *v1alpha1.LLMInferenceServiceConfig
		wantErr bool
	}{
		{
			name: "Replace model name",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("{{ .Spec.Model.Name }}"),
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Args: []string{
									"--served-model-name",
									"{{ .Spec.Model.Name }}",
								}},
							},
						},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("meta-llama/Llama-3.2-3B-Instruct"),
					},
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("meta-llama/Llama-3.2-3B-Instruct"),
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Args: []string{
									"--served-model-name",
									"meta-llama/Llama-3.2-3B-Instruct",
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "template with ChildName function",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: "{{ ChildName .Name `-sa` }}",
							Containers: []corev1.Container{
								{
									Name: "main",
									Env: []corev1.EnvVar{
										{Name: "DEPLOYMENT_NAME", Value: "{{ ChildName .Name `-deployment` }}"},
									},
								},
							},
						},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm",
					Namespace: "test-ns",
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: "test-llm-sa",
							Containers: []corev1.Container{
								{
									Name: "main",
									Env: []corev1.EnvVar{
										{Name: "DEPLOYMENT_NAME", Value: "test-llm-deployment"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "template in arrays",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{
										"--model-name={{ .Name }}",
										"--namespace={{ .Namespace }}",
										"--config-path=/config/{{ .Name }}.yaml",
									},
									Env: []corev1.EnvVar{
										{Name: "MODEL_NAME", Value: "{{ .Name }}"},
										{Name: "NAMESPACE", Value: "{{ .Namespace }}"},
									},
								},
							},
						},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gpt-model",
					Namespace: "ml-team",
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{
										"--model-name=gpt-model",
										"--namespace=ml-team",
										"--config-path=/config/gpt-model.yaml",
									},
									Env: []corev1.EnvVar{
										{Name: "MODEL_NAME", Value: "gpt-model"},
										{Name: "NAMESPACE", Value: "ml-team"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "template with complex nested model spec",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("{{ .Spec.Model.Name }}"),
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{
										"--served-model-name={{ .Spec.Model.Name }}",
										"--model-path={{ .Spec.Model.URI.Host }}{{ .Spec.Model.URI.Path }}",
									},
								},
							},
						},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("meta-llama/Llama-3.2-3B-Instruct"),
						URI:  mustParseURL("hf://meta-llama/Llama-3.2-3B-Instruct"),
					},
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("meta-llama/Llama-3.2-3B-Instruct"),
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{
										"--served-model-name=meta-llama/Llama-3.2-3B-Instruct",
										"--model-path=meta-llama/Llama-3.2-3B-Instruct",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "template with nil pointer access should not error if default value is provided",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To(`{{ if .Spec.Model.Name }}{{ .Spec.Model.Name }}{{ else }}default-model{{ end }}`),
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: nil, // Nil pointer
					},
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("default-model"),
					},
				},
			},
		},
		{
			name: "template with router configurations",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Router: &v1alpha1.RouterSpec{
						Route: &v1alpha1.GatewayRoutesSpec{
							HTTP: &v1alpha1.HTTPRouteSpec{
								Refs: []corev1.LocalObjectReference{
									{Name: "{{ .Name }}-route"},
								},
							},
						},
						Gateway: &v1alpha1.GatewaySpec{
							Refs: []v1alpha1.UntypedObjectReference{
								{Name: "{{ .Name }}-gateway", Namespace: "{{ .Namespace }}"},
							},
						},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "router-test",
					Namespace: "routing-ns",
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Router: &v1alpha1.RouterSpec{
						Route: &v1alpha1.GatewayRoutesSpec{
							HTTP: &v1alpha1.HTTPRouteSpec{
								Refs: []corev1.LocalObjectReference{
									{Name: "router-test-route"},
								},
							},
						},
						Gateway: &v1alpha1.GatewaySpec{
							Refs: []v1alpha1.UntypedObjectReference{
								{Name: "router-test-gateway", Namespace: "routing-ns"},
							},
						},
					},
				},
			},
		},
		{
			name: "template with multiple variables in single string",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Env: []corev1.EnvVar{
										{Name: "FULL_NAME", Value: "{{ .Namespace }}/{{ .Name }}"},
										{Name: "CONFIG_PATH", Value: "/config/{{ .Namespace }}-{{ .Name }}.yaml"},
									},
								},
							},
						},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-var",
					Namespace: "test-ns",
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Env: []corev1.EnvVar{
										{Name: "FULL_NAME", Value: "test-ns/multi-var"},
										{Name: "CONFIG_PATH", Value: "/config/test-ns-multi-var.yaml"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "template with invalid syntax should error",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("{{ .Name"), // Invalid template syntax - missing closing brace
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "template with non-existent field should error",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("{{ .NonExistentField }}"),
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "template in baseRefs",
			cfg: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "{{ .Name }}-base-config"},
						{Name: "{{ .Namespace }}-shared-config"},
					},
				},
			},
			llmSvc: &v1alpha1.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "base-ref-test",
					Namespace: "template-ns",
				},
			},
			want: &v1alpha1.LLMInferenceServiceConfig{
				Spec: v1alpha1.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "base-ref-test-base-config"},
						{Name: "template-ns-shared-config"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := llmisvc.ReplaceVariables(tt.llmSvc, tt.cfg, tt.extra)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplaceVariables() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("ReplaceVariables() got = %#v, want %#v\nDiff:\n%s", got, tt.want, diff)
				}
			}
		})
	}
}

func mustParseURL(s string) apis.URL {
	u, err := apis.ParseURL(s)
	if err != nil {
		panic(err)
	}
	return *u
}

func TestAdditionalData(t *testing.T) {
	tests := []struct {
		name             string
		config           string
		reconcilerConfig llmisvc.Config
		wantErr          bool
		want             func(llmSvc *v1alpha1.LLMInferenceServiceConfig, g *GomegaWithT)
	}{
		{
			name: "additional structs replacements",
			config: `apiVersion: serving.kserve.io/v1alpha1
kind: LLMInferenceServiceConfig
metadata:
  name: test-config
  namespace: "{{ .GlobalConfig.SystemNamespace }}"
spec:
  router:
    route:
      http:
        spec:
          parentRefs:
            - group: gateway.networking.k8s.io
              kind: Gateway
              name: "{{ .GlobalConfig.IngressGatewayName }}"
              namespace: "{{ .GlobalConfig.IngressGatewayNamespace }}"`,
			reconcilerConfig: llmisvc.Config{
				SystemNamespace:         "my-kserve",
				IngressGatewayName:      "my-gateway",
				IngressGatewayNamespace: "my-ns",
			},
			want: func(llmSvc *v1alpha1.LLMInferenceServiceConfig, g *GomegaWithT) {
				httpRouteSpec := llmSvc.Spec.Router.Route.HTTP.Spec
				expectedGatewayRef := gwapiv1.ParentReference{
					Name:      "my-gateway",
					Namespace: ptr.To(gwapiv1.Namespace("my-ns")),
				}
				g.Expect(httpRouteSpec).To(ktesting.HaveGatewayRefs(expectedGatewayRef))
				g.Expect(llmSvc.Namespace).To(Equal("my-kserve"))
			},
		},
		{
			name: "template with non-existing key should error",
			config: `apiVersion: serving.kserve.io/v1alpha1
kind: LLMInferenceServiceConfig
metadata:
  name: "{{ .GlobalConfig.NonExistentConfig.SomeField }}"
spec:
  model:
    name: "static-model"`,
			wantErr: true,
			want:    func(llmSvc *v1alpha1.LLMInferenceServiceConfig, g *GomegaWithT) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset := &v1alpha1.LLMInferenceServiceConfig{}
			if err := yaml.Unmarshal([]byte(tt.config), preset); err != nil {
				t.Errorf("Failed to unmarshal YAML: %v", err)
				return
			}

			llmSvc := &v1alpha1.LLMInferenceService{}
			got, err := llmisvc.ReplaceVariables(llmSvc, preset, &tt.reconcilerConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplaceVariables() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tt.want(got, NewGomegaWithT(t))
		})
	}
}
