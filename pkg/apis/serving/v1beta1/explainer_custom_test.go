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

package v1beta1

import (
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func TestCustomExplainerDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{},
		Resource: ResourceConfig{
			CPULimit:      "1",
			MemoryLimit:   "2Gi",
			CPURequest:    "1",
			MemoryRequest: "2Gi",
		},
	}
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec     ExplainerSpec
		expected ExplainerSpec
	}{
		"DefaultResources": {
			spec: ExplainerSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
						},
					},
				},
			},
			expected: ExplainerSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			explainer := CustomExplainer{PodSpec: corev1.PodSpec(scenario.spec.PodSpec)}
			explainer.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateCustomExplainerContainer(t *testing.T) {
	requestedResource := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			"cpu": resource.Quantity{
				Format: "100",
			},
			"memory": resource.MustParse("1Gi"),
		},
		Requests: corev1.ResourceList{
			"cpu": resource.Quantity{
				Format: "90",
			},
			"memory": resource.MustParse("1Gi"),
		},
	}
	config := InferenceServicesConfig{}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *corev1.Container
	}{
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: corev1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Image: "explainer:0.1.0",
									Env: []corev1.EnvVar{
										{
											Name:  "STORAGE_URI",
											Value: "hdfs://modelzoo",
										},
									},
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Image:     "explainer:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--predictor_host",
					fmt.Sprintf("%s.%s", constants.PredictorServiceName("someName"), "default"),
					"--http_port",
					"8080",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_URI",
						Value: "hdfs://modelzoo",
					},
				},
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(1),
						},
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: corev1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(2),
						},
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Image: "explainer:0.1.0",
									Env: []corev1.EnvVar{
										{
											Name:  "STORAGE_URI",
											Value: "hdfs://modelzoo",
										},
									},
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Image:     "explainer:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--predictor_host",
					fmt.Sprintf("%s.%s", constants.PredictorServiceName("someName"), "default"),
					"--http_port",
					"8080",
					"--workers",
					"2",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_URI",
						Value: "hdfs://modelzoo",
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			explainer := scenario.isvc.Spec.Explainer.GetImplementation()
			explainer.Default(&config)
			res := explainer.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, &scenario.isvc.Spec.Explainer.ComponentExtensionSpec,
				&config, constants.PredictorServiceName("someName"))
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}

func TestCustomExplainerIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{},
	}
	mmsCase := false
	scenarios := map[string]struct {
		spec     ExplainerSpec
		expected bool
	}{
		"DefaultResources": {
			spec: ExplainerSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
						},
					},
				},
			},
			expected: mmsCase,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			explainer := CustomExplainer{PodSpec: corev1.PodSpec(scenario.spec.PodSpec)}
			res := explainer.IsMMS(&config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}

func TestCustomExplainerValidate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		explainer CustomExplainer
		expected  error
	}{
		"ValidCustomExplainer": {
			explainer: CustomExplainer{
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "custom-explainer",
							Image: "explainer:latest",
						},
					},
				},
			},
			expected: nil,
		},
		"EmptyPodSpec": {
			explainer: CustomExplainer{
				PodSpec: corev1.PodSpec{},
			},
			expected: nil,
		},
		"NoContainers": {
			explainer: CustomExplainer{
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
			expected: nil,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.explainer.Validate()
			if scenario.expected == nil {
				g.Expect(res).ToNot(gomega.HaveOccurred())
			} else {
				g.Expect(res.Error()).To(gomega.Equal(scenario.expected.Error()))
			}
		})
	}
}

func TestCustomExplainerGetStorageUri(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		explainer      CustomExplainer
		expectedResult *string
	}{
		"WithStorageUriEnvVar": {
			explainer: CustomExplainer{
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "gs://my-model-storage",
								},
							},
						},
					},
				},
			},
			expectedResult: proto.String("gs://my-model-storage"),
		},
		"WithoutStorageUriEnvVar": {
			explainer: CustomExplainer{
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "OTHER_ENV_VAR",
									Value: "some-value",
								},
							},
						},
					},
				},
			},
			expectedResult: nil,
		},
		"WithEmptyEnvVars": {
			explainer: CustomExplainer{
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{},
						},
					},
				},
			},
			expectedResult: nil,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// Skip the test if there are no containers to avoid index out of range error
			if len(scenario.explainer.Containers) == 0 {
				result := scenario.explainer.GetStorageUri()
				g.Expect(result).To(gomega.BeNil())
				return
			}

			result := scenario.explainer.GetStorageUri()
			if scenario.expectedResult == nil {
				g.Expect(result).To(gomega.BeNil())
			} else {
				g.Expect(result).NotTo(gomega.BeNil())
				g.Expect(*result).To(gomega.Equal(*scenario.expectedResult))
			}
		})
	}
}
