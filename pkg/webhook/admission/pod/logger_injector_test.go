/*
Copyright 2020 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/kmp"
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LoggerDefaultCPURequest    = "100m"
	LoggerDefaultCPULimit      = "1"
	LoggerDefaultMemoryRequest = "200Mi"
	LoggerDefaultMemoryLimit   = "1Gi"
)

var (
	loggerConfig = &LoggerConfig{
		Image:         "gcr.io/kfserving/logger:latest",
		CpuRequest:    LoggerDefaultCPURequest,
		CpuLimit:      LoggerDefaultCPULimit,
		MemoryRequest: LoggerDefaultMemoryRequest,
		MemoryLimit:   LoggerDefaultMemoryLimit,
	}

	loggerResourceRequirement = v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(LoggerDefaultCPULimit),
			v1.ResourceMemory: resource.MustParse(LoggerDefaultMemoryLimit),
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(LoggerDefaultCPURequest),
			v1.ResourceMemory: resource.MustParse(LoggerDefaultMemoryRequest),
		},
	}
)

func TestLoggerInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"AddLogger": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey:        "true",
						constants.LoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
						constants.LoggerModeInternalAnnotationKey:    string(v1alpha2.LogAll),
					},
					Labels: map[string]string{
						"serving.kubeflow.org/inferenceservice": "sklearn",
						constants.KServiceModelLabel:            "sklearn",
						constants.KServiceEndpointLabel:         "default",
						constants.KServiceComponentLabel:        "predictor",
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
						constants.LoggerInternalAnnotationKey:        "true",
						constants.LoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
						constants.LoggerModeInternalAnnotationKey:    string(v1alpha2.LogAll),
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  LoggerContainerName,
							Image: loggerConfig.Image,
							Args: []string{
								LoggerArgumentLogUrl,
								"http://httpbin.org/",
								LoggerArgumentSourceUri,
								"deployment",
								LoggerArgumentMode,
								"all",
								LoggerArgumentInferenceService,
								"sklearn",
								LoggerArgumentNamespace,
								"default",
								LoggerArgumentEndpoint,
								"default",
							},
							Resources: loggerResourceRequirement,
						},
					},
				},
			},
		},
		"DoNotAddLogger": {
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
		injector := &LoggerInjector{
			loggerConfig,
		}
		injector.InjectLogger(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
