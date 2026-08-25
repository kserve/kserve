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

	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
)

func TestLightGBMValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				LightGBM: &LightGBMSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.LightGBM.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestLightGBMDefaulter(t *testing.T) {
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
				LightGBM: &LightGBMSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
					},
				},
			},
			expected: PredictorSpec{
				LightGBM: &LightGBMSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
						Container: corev1.Container{
							Name: constants.InferenceServiceContainerName,
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
			scenario.spec.LightGBM.Default(config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateLightGBMModelServingContainer(t *testing.T) {
	requestedResource := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: corev1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}
	config := InferenceServicesConfig{}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *corev1.Container
	}{
		"ContainerSpecWithDefaultImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lightgbm",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						LightGBM: &LightGBMSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: corev1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lightgbm",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						LightGBM: &LightGBMSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: corev1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Image:     "customImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lightgbm",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(1),
						},
						LightGBM: &LightGBMSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: corev1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithWorker": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lightgbm",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(2),
						},
						LightGBM: &LightGBMSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: corev1.Container{
									Resources: requestedResource,
									Args: []string{
										constants.ArgumentWorkers + "=1",
									},
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &corev1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--workers=1",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			predictor := scenario.isvc.Spec.Predictor.GetImplementation()
			predictor.Default(&config)
			res := predictor.GetContainer(metav1.ObjectMeta{Name: "someName"}, &scenario.isvc.Spec.Predictor.ComponentExtensionSpec, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}

func TestLightGBMGetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{}
	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"DefaultProtocol": {
			spec: PredictorSpec{
				LightGBM: &LightGBMSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV1),
		},
		"ProtocolSpecified": {
			spec: PredictorSpec{
				LightGBM: &LightGBMSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI:      proto.String("s3://modelzoo"),
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV2))),
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV2),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.LightGBM.Default(&config)
			protocol := scenario.spec.LightGBM.GetProtocol()
			g.Expect(protocol).To(scenario.matcher)
		})
	}
}
