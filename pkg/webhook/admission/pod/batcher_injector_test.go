/*
Copyright 2021 The KServe Authors.

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

package pod

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmp"

	"github.com/kserve/kserve/pkg/constants"
)

const (
	BatcherDefaultCPURequest    = "100m"
	BatcherDefaultCPULimit      = "1"
	BatcherDefaultMemoryRequest = "200Mi"
	BatcherDefaultMemoryLimit   = "1Gi"
	BatcherDefaultMaxBatchSize  = "16"
	BatcherDefaultMaxLatency    = "2000"
)

var (
	batcherConfig = &BatcherConfig{
		Image:         "gcr.io/kfserving/batcher:latest",
		CpuRequest:    BatcherDefaultCPURequest,
		CpuLimit:      BatcherDefaultCPULimit,
		MemoryRequest: BatcherDefaultMemoryRequest,
		MemoryLimit:   BatcherDefaultMemoryLimit,
		MaxBatchSize:  BatcherDefaultMaxBatchSize,
		MaxLatency:    BatcherDefaultMaxLatency,
	}

	batcherResourceRequirement = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(BatcherDefaultCPULimit),
			corev1.ResourceMemory: resource.MustParse(BatcherDefaultMemoryLimit),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(BatcherDefaultCPURequest),
			corev1.ResourceMemory: resource.MustParse(BatcherDefaultMemoryRequest),
		},
	}
)

func TestBatcherInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *corev1.Pod
		expected *corev1.Pod
	}{
		"AddBatcher": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: "32",
						constants.BatcherMaxLatencyInternalAnnotationKey:   "5000",
					},
					Labels: map[string]string{
						"serving.kserve.io/inferenceservice": "sklearn",
						constants.KServiceModelLabel:         "sklearn",
						constants.KServiceEndpointLabel:      "default",
						constants.KServiceComponentLabel:     "predictor",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: "32",
						constants.BatcherMaxLatencyInternalAnnotationKey:   "5000",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  BatcherContainerName,
							Image: batcherConfig.Image,
							Args: []string{
								BatcherArgumentMaxBatchSize,
								"32",
								BatcherArgumentMaxLatency,
								"5000",
							},
							Resources: batcherResourceRequirement,
						},
					},
				},
			},
		},
		"AddDefaultBatcherConfig": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey: "true",
					},
					Labels: map[string]string{
						"serving.kserve.io/inferenceservice": "sklearn",
						constants.KServiceModelLabel:         "sklearn",
						constants.KServiceEndpointLabel:      "default",
						constants.KServiceComponentLabel:     "predictor",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: BatcherDefaultMaxBatchSize,
						constants.BatcherMaxLatencyInternalAnnotationKey:   BatcherDefaultMaxLatency,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  BatcherContainerName,
							Image: batcherConfig.Image,
							Args: []string{
								BatcherArgumentMaxBatchSize,
								BatcherDefaultMaxBatchSize,
								BatcherArgumentMaxLatency,
								BatcherDefaultMaxLatency,
							},
							Resources: batcherResourceRequirement,
						},
					},
				},
			},
		},
		"DoNotAddBatcher": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "sklearn",
					}},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		injector := &BatcherInjector{
			batcherConfig,
		}
		injector.InjectBatcher(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestGetBatcherConfigs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name      string
		configMap *corev1.ConfigMap
		matchers  []types.GomegaMatcher
	}{
		{
			name: "Valid Batcher Config",
			configMap: &corev1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					BatcherConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/batcher:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200Mi",
						"MemoryLimit":   "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&BatcherConfig{
					Image:         "gcr.io/kfserving/batcher:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200Mi",
					MemoryLimit:   "1Gi",
				}),
				gomega.BeNil(),
			},
		},
		{
			name: "Default Batcher Config",
			configMap: &corev1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					BatcherConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/batcher:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200Mi",
						"MemoryLimit":   "1Gi",
						"MaxBatchSize":  "32",
						"MaxLatency":    "5000"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&BatcherConfig{
					Image:         "gcr.io/kfserving/batcher:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200Mi",
					MemoryLimit:   "1Gi",
					MaxBatchSize:  "32",
					MaxLatency:    "5000",
				}),
				gomega.BeNil(),
			},
		},
		{
			name: "Invalid Resource Value",
			configMap: &corev1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					BatcherConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/batcher:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200mc",
						"MemoryLimit":   "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&BatcherConfig{
					Image:         "gcr.io/kfserving/batcher:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200mc",
					MemoryLimit:   "1Gi",
				}),
				gomega.HaveOccurred(),
			},
		},
	}

	for _, tc := range cases {
		loggerConfigs, err := getBatcherConfigs(tc.configMap)
		g.Expect(err).Should(tc.matchers[1])
		g.Expect(loggerConfigs).Should(tc.matchers[0])
	}
}
