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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// LLMInferenceServiceSample defines a full sample of LLMInferenceService that can be used
// as a basis to apply LLMInferenceServiceConfigs. It is used for validating templated values
// in LLMInferenceServiceConfig CR.
func LLMInferenceServiceSample() *v1alpha1.LLMInferenceService {
	svcName := "test-llm-preset"
	nsName := "test-llm-preset-test"
	modelURL, _ := apis.ParseURL("llama")

	return &v1alpha1.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: nsName,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "llminferenceservice",
				"app.kubernetes.io/instance":  svcName,
				"app.kubernetes.io/component": "inference",
			},
			Annotations: map[string]string{
				"serving.kserve.io/model-uri": modelURL.String(),
			},
		},
		Spec: v1alpha1.LLMInferenceServiceSpec{
			Model: v1alpha1.LLMModelSpec{
				Name: ptr.To("llama"),
				URI:  *modelURL,
				Storage: &v1alpha1.LLMStorageSpec{
					Path: ptr.To("/models"),
					Parameters: &map[string]string{
						"storageUri": modelURL.String(),
					},
				},
			},
			WorkloadSpec: v1alpha1.WorkloadSpec{
				Replicas: ptr.To[int32](2),
				Parallelism: &v1alpha1.ParallelismSpec{
					Data:      ptr.To[int32](4),
					DataLocal: ptr.To[int32](2),
					Tensor:    ptr.To[int32](1),
					Expert:    true,
				},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kserve-container",
							Image: "ghcr.io/llm-d/llm-d:v0.2.0",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "MODEL_NAME",
									Value: "facebook/opt-125m",
								},
								{
									Name:  "VLLM_LOGGING_LEVEL",
									Value: "INFO",
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "nvidia.com/gpu",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					NodeSelector: map[string]string{
						"node.kubernetes.io/instance-type": "gpu-node",
					},
				},
				Worker: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kserve-container",
							Image: "ghcr.io/llm-d/llm-d:0.2.0",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									"nvidia.com/gpu":      resource.MustParse("1"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
									"nvidia.com/gpu":      resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			Prefill: &v1alpha1.WorkloadSpec{
				Replicas: ptr.To[int32](1),
				Parallelism: &v1alpha1.ParallelismSpec{
					Tensor:   ptr.To[int32](1),
					Pipeline: ptr.To[int32](1),
				},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kserve-container",
							Image: "ghcr.io/llm-d/llm-d:v0.2.0",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									"nvidia.com/gpu":      resource.MustParse("2"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("8"),
									corev1.ResourceMemory: resource.MustParse("16Gi"),
									"nvidia.com/gpu":      resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			Router: &v1alpha1.RouterSpec{
				Route: &v1alpha1.GatewayRoutesSpec{
					HTTP: &v1alpha1.HTTPRouteSpec{
						Refs: []corev1.LocalObjectReference{
							{Name: "custom-http-route"},
						},
					},
				},
				Gateway: &v1alpha1.GatewaySpec{
					Refs: []v1alpha1.UntypedObjectReference{
						{
							Name:      "kserve-ingress-gateway",
							Namespace: "kserve",
						},
					},
				},
				Scheduler: &v1alpha1.SchedulerSpec{
					Pool: &v1alpha1.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{
							Name: "custom-inference-pool",
						},
					},
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "scheduler",
								Image: "ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 9002,
										Name:          "grpc",
										Protocol:      corev1.ProtocolTCP,
									},
									{
										ContainerPort: 9003,
										Name:          "grpc-health",
										Protocol:      corev1.ProtocolTCP,
									},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("256m"),
										corev1.ResourceMemory: resource.MustParse("500Mi"),
									},
								},
								Env: []corev1.EnvVar{
									{Name: "ENABLE_LOAD_AWARE_SCORER", Value: "true"},
									{Name: "POOL_NAME", Value: svcName + "-inference-pool"},
									{Name: "POOL_NAMESPACE", Value: nsName},
								},
							},
						},
					},
				},
			},
		},
	}
}
