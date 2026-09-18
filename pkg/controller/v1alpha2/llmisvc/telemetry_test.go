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

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
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

func metricLabels(t *testing.T, m dto.Metric) map[string]string {
	t.Helper()
	labels := make(map[string]string, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	return labels
}

func collectAndParse(t *testing.T, items []v1alpha2.LLMInferenceService) []dto.Metric {
	t.Helper()
	metrics := collectInfoMetrics(items)
	result := make([]dto.Metric, 0, len(metrics))
	for _, m := range metrics {
		var dm dto.Metric
		require.NoError(t, m.Write(&dm))
		result = append(result, dm)
	}
	return result
}

// withAcceleratorAnnotation simulates what RecordAcceleratorAnnotation does
// during reconciliation, setting the status annotation for the Collector.
func withAcceleratorAnnotation(svc *v1alpha2.LLMInferenceService) *v1alpha2.LLMInferenceService {
	RecordAcceleratorAnnotation(svc)
	return svc
}

func TestResolveAccelerator(t *testing.T) {
	tests := []struct {
		name     string
		isvc     *v1alpha2.LLMInferenceService
		expected string
	}{
		{
			name: "nil pod spec",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{Template: nil},
				},
			},
			expected: acceleratorCPU,
		},
		{
			name:     "cpu only",
			isvc:     cpuLLMInferenceService("test", "ns", "facebook", "opt-125m"),
			expected: acceleratorCPU,
		},
		{
			name: "nvidia gpu in requests",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "nvidia gpu in limits only",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "amd gpu",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("amd.com/gpu"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "intel gpu.intel.com/i915",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("gpu.intel.com/i915"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "intel gpu.intel.com/xe",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("gpu.intel.com/xe"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "habana gaudi",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("habana.ai/gaudi"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "gpu in second container",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
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
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "empty containers",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{},
						},
					},
				},
			},
			expected: acceleratorCPU,
		},
		{
			name: "gpu in worker spec",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
									},
								},
							}},
						},
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "gpu in prefill template",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
									},
								},
							}},
						},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "gpu in prefill worker",
			isvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
									},
								},
							}},
						},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
		{
			name: "DRA device class annotation",
			isvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
					},
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
									},
								},
							}},
						},
					},
				},
			},
			expected: acceleratorGPU,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAccelerator(tt.isvc)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRecordAcceleratorAnnotation(t *testing.T) {
	t.Run("sets cpu for cpu-only service", func(t *testing.T) {
		svc := cpuLLMInferenceService("test", "ns", "facebook", "opt-125m")
		RecordAcceleratorAnnotation(svc)
		assert.Equal(t, "cpu", svc.Status.Annotations[AcceleratorAnnotationKey])
	})

	t.Run("sets gpu for gpu service", func(t *testing.T) {
		svc := gpuLLMInferenceService("test", "ns", "facebook", "opt-125m")
		RecordAcceleratorAnnotation(svc)
		assert.Equal(t, "gpu", svc.Status.Annotations[AcceleratorAnnotationKey])
	})

	t.Run("initializes nil annotations map", func(t *testing.T) {
		svc := cpuLLMInferenceService("test", "ns", "facebook", "opt-125m")
		assert.Nil(t, svc.Status.Annotations)
		RecordAcceleratorAnnotation(svc)
		assert.NotNil(t, svc.Status.Annotations)
	})
}

func TestCollectInfoMetrics(t *testing.T) {
	cpuSvc := withAcceleratorAnnotation(cpuLLMInferenceService("opt-125m-cpu", "test-ns", "facebook", "opt-125m"))
	gpuSvc := withAcceleratorAnnotation(gpuLLMInferenceService("opt-125m-gpu", "test-ns", "facebook", "opt-125m"))

	t.Run("emits one metric per service", func(t *testing.T) {
		metrics := collectInfoMetrics([]v1alpha2.LLMInferenceService{*cpuSvc, *gpuSvc})
		assert.Len(t, metrics, 2)
	})

	t.Run("empty list produces no metrics", func(t *testing.T) {
		metrics := collectInfoMetrics([]v1alpha2.LLMInferenceService{})
		assert.Empty(t, metrics)
	})
}

func TestCollectInfoMetricsCPULabels(t *testing.T) {
	cpuSvc := withAcceleratorAnnotation(cpuLLMInferenceService("opt-125m-cpu", "test-ns", "facebook", "opt-125m"))
	parsed := collectAndParse(t, []v1alpha2.LLMInferenceService{*cpuSvc})

	require.Len(t, parsed, 1)
	labels := metricLabels(t, parsed[0])

	uri := testModelURI("facebook", "opt-125m")
	assert.Equal(t, "test-ns", labels["namespace"])
	assert.Equal(t, "opt-125m-cpu", labels["name"])
	assert.Equal(t, "cpu", labels["accelerator"])
	assert.Equal(t, "opt-125m-cpu", labels["model_name"])
	assert.Equal(t, uri.String(), labels["model_uri"])
	assert.Equal(t, float64(1), parsed[0].GetGauge().GetValue())
}

func TestCollectInfoMetricsGPULabels(t *testing.T) {
	gpuSvc := withAcceleratorAnnotation(gpuLLMInferenceService("opt-125m-gpu", "test-ns", "facebook", "opt-125m"))
	parsed := collectAndParse(t, []v1alpha2.LLMInferenceService{*gpuSvc})

	require.Len(t, parsed, 1)
	labels := metricLabels(t, parsed[0])

	assert.Equal(t, "gpu", labels["accelerator"])
	assert.Equal(t, "opt-125m-gpu", labels["name"])
}

func TestCollectInfoMetricsCustomModelName(t *testing.T) {
	svc := cpuLLMInferenceService("svc-name", "ns", "facebook", "opt-125m")
	modelName := "my-custom-model"
	svc.Spec.Model.Name = &modelName
	withAcceleratorAnnotation(svc)

	parsed := collectAndParse(t, []v1alpha2.LLMInferenceService{*svc})
	require.Len(t, parsed, 1)
	labels := metricLabels(t, parsed[0])

	assert.Equal(t, "my-custom-model", labels["model_name"])
}

func TestCollectInfoMetricsDRA(t *testing.T) {
	svc := cpuLLMInferenceService("dra-svc", "ns", "meta-llama", "Llama-3.2-1B")
	svc.Annotations = map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
	}
	withAcceleratorAnnotation(svc)

	parsed := collectAndParse(t, []v1alpha2.LLMInferenceService{*svc})
	require.Len(t, parsed, 1)
	labels := metricLabels(t, parsed[0])

	assert.Equal(t, "gpu", labels["accelerator"])
}

func TestCollectInfoMetricsFallbackWithoutAnnotation(t *testing.T) {
	svc := cpuLLMInferenceService("no-annotation", "ns", "facebook", "opt-125m")
	// No RecordAcceleratorAnnotation call — simulates a service that hasn't been reconciled yet
	parsed := collectAndParse(t, []v1alpha2.LLMInferenceService{*svc})
	require.Len(t, parsed, 1)
	labels := metricLabels(t, parsed[0])

	assert.Equal(t, "cpu", labels["accelerator"])
}

func TestDescribe(t *testing.T) {
	collector := &InfoMetricsCollector{}
	ch := make(chan *prometheus.Desc, 1)
	collector.Describe(ch)
	desc := <-ch
	assert.Equal(t, infoDesc, desc)
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
