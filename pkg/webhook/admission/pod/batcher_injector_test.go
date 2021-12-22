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

	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/kmp"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BatcherDefaultCPURequest    = "100m"
	BatcherDefaultCPULimit      = "1"
	BatcherDefaultMemoryRequest = "200Mi"
	BatcherDefaultMemoryLimit   = "1Gi"
)

var (
	batcherConfig = &BatcherConfig{
		Image:         "gcr.io/kfserving/batcher:latest",
		CpuRequest:    BatcherDefaultCPURequest,
		CpuLimit:      BatcherDefaultCPULimit,
		MemoryRequest: BatcherDefaultMemoryRequest,
		MemoryLimit:   BatcherDefaultMemoryLimit,
	}

	batcherResourceRequirement = v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(BatcherDefaultCPULimit),
			v1.ResourceMemory: resource.MustParse(BatcherDefaultMemoryLimit),
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(BatcherDefaultCPURequest),
			v1.ResourceMemory: resource.MustParse(BatcherDefaultMemoryRequest),
		},
	}
)

func TestBatcherInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"AddBatcher": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: "32",
						constants.BatcherMaxLatencyInternalAnnotationKey:   "5000",
						constants.BatcherTimeoutInternalAnnotationKey:      "60",
					},
					Labels: map[string]string{
						"serving.kserve.io/inferenceservice": "sklearn",
						constants.KServiceModelLabel:         "sklearn",
						constants.KServiceEndpointLabel:      "default",
						constants.KServiceComponentLabel:     "predictor",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: "32",
						constants.BatcherMaxLatencyInternalAnnotationKey:   "5000",
						constants.BatcherTimeoutInternalAnnotationKey:      "60",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
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
								BatcherArgumentTimeout,
								"60",
							},
							Resources: batcherResourceRequirement,
						},
					},
				},
			},
		},
		"DoNotAddBatcher": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
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
