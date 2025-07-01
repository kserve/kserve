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

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
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
				{Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-a")}},
			},
			want:    v1alpha1.LLMInferenceServiceSpec{Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-a")}},
			wantErr: false,
		},
		{
			name: "two configs simple merge",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-a")}},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-a")},
			},
			wantErr: false,
		},
		{
			name: "two configs with override",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-a")},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-b")},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](2),
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-b")},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			wantErr: false,
		},
		{
			name: "three configs chained merge",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-a")}},
				{
					Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-b")},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{URI: mustParseURL("model-b")},
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
							Tensor: ptr.To[int64](2),
						},
					},
				},
				// Override defines pipeline parallelism
				{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Parallelism: &v1alpha1.ParallelismSpec{
							Pipeline: ptr.To[int64](4),
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha1.WorkloadSpec{
					// Both parallelism values should be present
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int64](2),
						Pipeline: ptr.To[int64](4),
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
							Tensor:   ptr.To[int64](1),
							Pipeline: ptr.To[int64](1),
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
							Tensor:   ptr.To[int64](4),
							Pipeline: ptr.To[int64](2),
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
						Tensor:   ptr.To[int64](4),
						Pipeline: ptr.To[int64](2),
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
							Tensor:   ptr.To[int64](1),
							Pipeline: ptr.To[int64](1),
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
							Tensor:   ptr.To[int64](4),
							Pipeline: ptr.To[int64](2),
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
						Tensor:   ptr.To[int64](4),
						Pipeline: ptr.To[int64](2),
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
							Tensor:   ptr.To[int64](1),
							Pipeline: ptr.To[int64](1),
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
						Tensor:   ptr.To[int64](1),
						Pipeline: ptr.To[int64](1),
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
						URI: mustParseURL("base-model"),
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.ModelSpec{
								{StorageURI: "s3://bucket/adapter1", Framework: "pytorch", Memory: resource.MustParse("1Gi")},
							},
						},
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.ModelSpec{
								{StorageURI: "s3://bucket/adapter2", Framework: "pytorch", Memory: resource.MustParse("512Mi")},
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI: mustParseURL("base-model"),
					LoRA: &v1alpha1.LoRASpec{
						Adapters: []v1alpha1.ModelSpec{
							{StorageURI: "s3://bucket/adapter2", Framework: "pytorch", Memory: resource.MustParse("512Mi")},
						},
					},
				},
			},
		},
		{
			name: "merge storage spec",
			cfgs: []v1alpha1.LLMInferenceServiceSpec{
				{
					Model: v1alpha1.LLMModelSpec{
						URI: mustParseURL("model-uri"),
						Storage: &v1alpha1.LLMStorageSpec{
							Path:       ptr.To("/models/base"),
							StorageKey: ptr.To("base-key"),
							Parameters: &map[string]string{
								"region": "us-east-1",
								"bucket": "my-bucket",
							},
						},
					},
				},
				{
					Model: v1alpha1.LLMModelSpec{
						Storage: &v1alpha1.LLMStorageSpec{
							Path:       ptr.To("/models/override"),
							StorageKey: ptr.To("override-key"),
							Parameters: &map[string]string{
								"region":     "us-west-2",
								"encryption": "aes256",
							},
						},
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI: mustParseURL("model-uri"),
					Storage: &v1alpha1.LLMStorageSpec{
						Path:       ptr.To("/models/override"),
						StorageKey: ptr.To("override-key"),
						Parameters: &map[string]string{
							"region":     "us-west-2",
							"bucket":     "my-bucket",
							"encryption": "aes256",
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
						URI:         mustParseURL("model-uri"),
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
					URI:         mustParseURL("model-uri"),
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
						URI:  mustParseURL("model-uri"),
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
					URI:  mustParseURL("model-uri"),
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
						URI:         mustParseURL("base-model"),
						Name:        ptr.To("base-name"),
						Criticality: ptr.To(igwapi.Sheddable),
						LoRA: &v1alpha1.LoRASpec{
							Adapters: []v1alpha1.ModelSpec{
								{StorageURI: "base-adapter", Framework: "pytorch", Memory: resource.MustParse("1Gi")},
							},
						},
						Storage: &v1alpha1.LLMStorageSpec{
							Path:       ptr.To("/base/path"),
							StorageKey: ptr.To("base-key"),
						},
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](1),
						Parallelism: &v1alpha1.ParallelismSpec{
							Tensor: ptr.To[int64](2),
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
							Adapters: []v1alpha1.ModelSpec{
								{StorageURI: "override-adapter", Framework: "tensorflow", Memory: resource.MustParse("2Gi")},
							},
						},
						Storage: &v1alpha1.LLMStorageSpec{
							Path: ptr.To("/override/path"),
							Parameters: &map[string]string{
								"new-param": "new-value",
							},
						},
					},
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Replicas: ptr.To[int32](5),
						Parallelism: &v1alpha1.ParallelismSpec{
							Pipeline: ptr.To[int64](4),
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
					URI:         mustParseURL("base-model"), // Base URI preserved
					Name:        ptr.To("override-name"),    // Override name
					Criticality: ptr.To(igwapi.Critical),
					LoRA: &v1alpha1.LoRASpec{
						Adapters: []v1alpha1.ModelSpec{
							{StorageURI: "override-adapter", Framework: "tensorflow", Memory: resource.MustParse("2Gi")},
						},
					},
					Storage: &v1alpha1.LLMStorageSpec{
						Path:       ptr.To("/override/path"),
						StorageKey: ptr.To("base-key"), // Base key preserved
						Parameters: &map[string]string{
							"new-param": "new-value",
						},
					},
				},
				WorkloadSpec: v1alpha1.WorkloadSpec{
					Replicas: ptr.To[int32](5),
					Parallelism: &v1alpha1.ParallelismSpec{
						Tensor:   ptr.To[int64](2), // Base tensor preserved
						Pipeline: ptr.To[int64](4), // Override pipeline
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
						URI: mustParseURL("populated-model"),
					},
				},
			},
			want: v1alpha1.LLMInferenceServiceSpec{
				Model: v1alpha1.LLMModelSpec{
					URI: mustParseURL("populated-model"),
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := llmisvc.MergeSpecs(tt.cfgs...)
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

func mustParseURL(s string) apis.URL {
	u, err := apis.ParseURL(s)
	if err != nil {
		panic(err)
	}
	return *u
}
