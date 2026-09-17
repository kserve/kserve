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

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func testModelURI(org, model string) apis.URL {
	return apis.URL{Scheme: "hf", Host: org, Path: "/" + model}
}

func cpuLLMInferenceService(name, namespace, org, model string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: testModelURI(org, model),
			},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "main",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("4"),
								corev1.ResourceMemory: resource.MustParse("16Gi"),
							},
						},
					}},
				},
			},
		},
	}
}

func gpuLLMInferenceService(name, namespace, org, model string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: testModelURI(org, model),
			},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "main",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:                    resource.MustParse("2"),
								corev1.ResourceMemory:                 resource.MustParse("8Gi"),
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
							},
						},
					}},
				},
			},
		},
	}
}

func TestResolveAccelerator(t *testing.T) {
	tests := []struct {
		name     string
		podSpec  *corev1.PodSpec
		expected string
	}{
		{
			name:     "nil pod spec",
			podSpec:  nil,
			expected: acceleratorCPU,
		},
		{
			name: "cpu only",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("16Gi"),
						},
					},
				}},
			},
			expected: acceleratorCPU,
		},
		{
			name: "nvidia gpu in requests",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
						},
					},
				}},
			},
			expected: acceleratorGPU,
		},
		{
			name: "nvidia gpu in limits only",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
						},
					},
				}},
			},
			expected: acceleratorGPU,
		},
		{
			name: "amd gpu",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("amd.com/gpu"): resource.MustParse("1"),
						},
					},
				}},
			},
			expected: acceleratorGPU,
		},
		{
			name: "intel gpu.intel.com/i915",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("gpu.intel.com/i915"): resource.MustParse("1"),
						},
					},
				}},
			},
			expected: acceleratorGPU,
		},
		{
			name: "intel gpu.intel.com/xe",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("gpu.intel.com/xe"): resource.MustParse("1"),
						},
					},
				}},
			},
			expected: acceleratorGPU,
		},
		{
			name: "habana gaudi",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("habana.ai/gaudi"): resource.MustParse("1"),
						},
					},
				}},
			},
			expected: acceleratorGPU,
		},
		{
			name: "gpu in second container",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
							},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "empty containers",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{},
			},
			expected: acceleratorCPU,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAccelerator(tt.podSpec)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRecordAndDeleteLLMInferenceServiceInfo(t *testing.T) {
	llmInferenceServiceInfo.Reset()

	cpuSvc := cpuLLMInferenceService("opt-125m-cpu", "test-ns", "facebook", "opt-125m")
	gpuSvc := gpuLLMInferenceService("opt-125m-gpu", "test-ns", "facebook", "opt-125m")

	recordLLMInferenceServiceInfo(cpuSvc)
	recordLLMInferenceServiceInfo(gpuSvc)

	assert.Equal(t, 2, testutil.CollectAndCount(llmInferenceServiceInfo))

	deleteLLMInferenceServiceInfo(cpuSvc)
	assert.Equal(t, 1, testutil.CollectAndCount(llmInferenceServiceInfo))

	deleteLLMInferenceServiceInfo(gpuSvc)
	assert.Equal(t, 0, testutil.CollectAndCount(llmInferenceServiceInfo))
}

func TestRecordLLMInferenceServiceInfoLabels(t *testing.T) {
	llmInferenceServiceInfo.Reset()

	cpuSvc := cpuLLMInferenceService("opt-125m-cpu", "test-ns", "facebook", "opt-125m")
	recordLLMInferenceServiceInfo(cpuSvc)

	uri := testModelURI("facebook", "opt-125m")
	expectedURI := uri.String()
	gauge := llmInferenceServiceInfo.WithLabelValues("test-ns", "opt-125m-cpu", "cpu", "opt-125m-cpu", expectedURI)
	assert.Equal(t, float64(1), testutil.ToFloat64(gauge))

	llmInferenceServiceInfo.Reset()
}

func TestRecordReplacesStaleLabels(t *testing.T) {
	llmInferenceServiceInfo.Reset()

	svc := cpuLLMInferenceService("my-svc", "ns", "meta-llama", "Llama-3.2-1B")
	recordLLMInferenceServiceInfo(svc)
	assert.Equal(t, 1, testutil.CollectAndCount(llmInferenceServiceInfo))

	svc.Spec.Model.URI = testModelURI("facebook", "opt-125m")
	recordLLMInferenceServiceInfo(svc)
	assert.Equal(t, 1, testutil.CollectAndCount(llmInferenceServiceInfo))

	llmInferenceServiceInfo.Reset()
}

func TestResolveModelName(t *testing.T) {
	t.Run("uses spec.model.name when set", func(t *testing.T) {
		modelName := "my-custom-model"
		svc := cpuLLMInferenceService("svc-name", "ns", "facebook", "opt-125m")
		svc.Spec.Model.Name = &modelName
		assert.Equal(t, "my-custom-model", resolveModelName(svc))
	})

	t.Run("falls back to metadata.name", func(t *testing.T) {
		svc := cpuLLMInferenceService("svc-name", "ns", "facebook", "opt-125m")
		assert.Equal(t, "svc-name", resolveModelName(svc))
	})
}
