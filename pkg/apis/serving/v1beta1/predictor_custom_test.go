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
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/constants"
)

func TestCustomPredictorValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"ValidProtocolV1": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          ptr.To(int32(3)),
					ContainerConcurrency: proto.Int64(-1),
				},
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "PROTOCOL",
									Value: "v1",
								},
							},
						},
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"ValidProtocolV2": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          ptr.To(int32(3)),
					ContainerConcurrency: proto.Int64(-1),
				},
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "PROTOCOL",
									Value: "v2",
								},
							},
						},
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidValidProtocol": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          ptr.To(int32(3)),
					ContainerConcurrency: proto.Int64(-1),
				},
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "PROTOCOL",
									Value: "unknown",
								},
							},
						},
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			customPredictor := NewCustomPredictor(&scenario.spec.PodSpec)
			res := customPredictor.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestCustomPredictorDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}
	config := &InferenceServicesConfig{
		Resource: ResourceConfig{
			CPULimit:      "1",
			MemoryLimit:   "2Gi",
			CPURequest:    "1",
			MemoryRequest: "2Gi",
		},
	}

	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultResources": {
			spec: PredictorSpec{
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
			expected: PredictorSpec{
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
		}, "PredictorContainerWithoutName": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Image: "custom-predictor:0.1.0",
							Args: []string{
								"--model_name",
								"someName",
								"--http_port",
								"8080",
							},
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
			expected: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.InferenceServiceContainerName,
							Image: "custom-predictor:0.1.0",
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
							Args: []string{
								"--model_name",
								"someName",
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
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			customPredictor := NewCustomPredictor(&scenario.spec.PodSpec)
			customPredictor.Default(config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateCustomPredictorContainer(t *testing.T) {
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
					Name: "custom-predictor",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "custom-predictor:0.1.0",
									Args: []string{
										"--model_name",
										"someName",
										"--http_port",
										"8080",
									},
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
				Image:     "custom-predictor:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
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
		}, "CustomPredictorContainerWithTransformer": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "custom-predictor-transformer-collocation",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "custom-predictor:0.1.0",
									Args: []string{
										"--model_name",
										"someName",
										"--http_port",
										"8080",
									},
									Env: []corev1.EnvVar{
										{
											Name:  "STORAGE_URI",
											Value: "hdfs://modelzoo",
										},
									},
									Resources: requestedResource,
								},
								{
									Name:      constants.TransformerContainerName,
									Image:     "kserve/transformer:1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Image:     "custom-predictor:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
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
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			predictor := scenario.isvc.Spec.Predictor.GetImplementation()
			predictor.Default(&config)
			res := predictor.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, &scenario.isvc.Spec.Predictor.ComponentExtensionSpec, &config)
			g.Expect(res).To(gomega.BeComparableTo(scenario.expectedContainerSpec))
		})
	}
}

func TestCustomPredictorGetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"Default protocol": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
							},
						},
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV1),
		},
		"protocol v2": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
								{
									Name:  constants.CustomSpecProtocolEnvVarKey,
									Value: string(constants.ProtocolV2),
								},
							},
						},
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV2),
		},
		"Collocation With Protocol Specified": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.InferenceServiceContainerName,
							Image: "kserve/testImage:1.0",
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
							},
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "kserve/transformer:1.0",
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
								{
									Name:  constants.CustomSpecProtocolEnvVarKey,
									Value: string(constants.ProtocolV2),
								},
							},
						},
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV2),
		},
		"Collocation Default Protocol": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.InferenceServiceContainerName,
							Image: "kserve/testImage:1.0",
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
							},
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "kserve/transformer:1.0",
							Env: []corev1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
							},
						},
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV1),
		},
	}
	for _, scenario := range scenarios {
		customPredictor := NewCustomPredictor(&scenario.spec.PodSpec)
		protocol := customPredictor.GetProtocol()
		g.Expect(protocol).To(scenario.matcher)
	}
}

func TestCustomPredictorGetStorageUri(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		spec     PredictorSpec
		expected *string
	}{
		"StorageUriSet": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []corev1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "s3://modelzoo",
								},
							},
						},
					},
				},
			},
			expected: ptr.To("s3://modelzoo"),
		},
		"StorageUriNotSet": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env:  []corev1.EnvVar{},
						},
					},
				},
			},
			expected: nil,
		},
		"DifferentContainerName": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: "different-container",
							Env: []corev1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "s3://modelzoo",
								},
							},
						},
					},
				},
			},
			expected: nil,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			customPredictor := NewCustomPredictor(&scenario.spec.PodSpec)
			storageUri := customPredictor.GetStorageUri()
			g.Expect(storageUri).To(gomega.BeComparableTo(scenario.expected))
		})
	}
}
