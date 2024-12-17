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
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestSKLearnValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
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
			res := scenario.spec.SKLearn.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestSKLearnDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	defaultResource := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
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
		"DefaultRuntimeVersionAndProtocol": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			expected: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
						Container: v1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: v1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
		"DefaultResources": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
					},
				},
			},
			expected: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("v0.3.0"),
						ProtocolVersion: &protocolV1,
						Container: v1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: v1.ResourceRequirements{
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
			scenario.spec.SKLearn.Default(config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateSKLearnModelServingContainerV1(t *testing.T) {
	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "100",
			},
		},
		Requests: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "90",
			},
		},
	}
	var config = InferenceServicesConfig{}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *v1.Container
	}{
		"ContainerSpecWithoutRuntime": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithDefaultImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
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
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "customImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
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
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithWorkerArg": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(4),
						},
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: v1.Container{
									Resources: requestedResource,
									Args: []string{
										"--workers=1",
									},
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
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

func TestCreateSKLearnModelServingContainerV2(t *testing.T) {
	protocolV2 := constants.ProtocolV2

	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "100",
			},
		},
		Requests: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "90",
			},
		},
	}
	var config = InferenceServicesConfig{}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *v1.Container
	}{
		"ContainerSpecWithDefaultImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://someUri"),
								RuntimeVersion:  proto.String("0.1.0"),
								ProtocolVersion: &protocolV2,
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://someUri"),
								ProtocolVersion: &protocolV2,
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "customImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
		"ContainerSpecWithoutStorageURI": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								ProtocolVersion: &protocolV2,
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			predictor := scenario.isvc.Spec.Predictor.GetImplementation()
			predictor.Default(&config)
			res := predictor.GetContainer(scenario.isvc.ObjectMeta, &scenario.isvc.Spec.Predictor.ComponentExtensionSpec, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}

func TestSKLearnGetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"DefaultProtocol": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV1),
		},
		"ProtocolV2": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV2))),
					},
				},
			},
			matcher: gomega.Equal(constants.ProtocolV2),
		},
	}
	for _, scenario := range scenarios {
		protocol := scenario.spec.SKLearn.GetProtocol()
		g.Expect(protocol).To(scenario.matcher)
	}
}

func TestSKLearnSpec_GetDefaultsV2(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	metadata := metav1.ObjectMeta{
		Name: "test",
	}
	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"storage uri is nil": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			matcher: gomega.Equal([]v1.EnvVar{
				{
					Name:  constants.MLServerModelImplementationEnv,
					Value: constants.MLServerSKLearnImplementation,
				},
			}),
		},
		"storage uri is not nil": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://kserve/model"),
					},
				},
			},
			matcher: gomega.Equal([]v1.EnvVar{
				{
					Name:  constants.MLServerModelImplementationEnv,
					Value: constants.MLServerSKLearnImplementation,
				},
				{
					Name:  constants.MLServerModelNameEnv,
					Value: metadata.Name,
				},
				{
					Name:  constants.MLServerModelURIEnv,
					Value: constants.DefaultModelLocalMountPath,
				},
			}),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.SKLearn.getDefaultsV2(metadata)
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestSKLearnSpec_GetEnvVarsV2(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"storage uri is nil": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			matcher: gomega.Equal([]v1.EnvVar{
				{
					Name:  constants.MLServerHTTPPortEnv,
					Value: strconv.Itoa(int(constants.MLServerISRestPort)),
				},
				{
					Name:  constants.MLServerGRPCPortEnv,
					Value: strconv.Itoa(int(constants.MLServerISGRPCPort)),
				},
				{
					Name:  constants.MLServerModelsDirEnv,
					Value: constants.DefaultModelLocalMountPath,
				},
				{
					Name:  constants.MLServerLoadModelsStartupEnv,
					Value: strconv.FormatBool(false),
				},
			}),
		},
		"storage uri is not nil": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://kserve/model"),
					},
				},
			},
			matcher: gomega.Equal([]v1.EnvVar{
				{
					Name:  constants.MLServerHTTPPortEnv,
					Value: strconv.Itoa(int(constants.MLServerISRestPort)),
				},
				{
					Name:  constants.MLServerGRPCPortEnv,
					Value: strconv.Itoa(int(constants.MLServerISGRPCPort)),
				},
				{
					Name:  constants.MLServerModelsDirEnv,
					Value: constants.DefaultModelLocalMountPath,
				},
			}),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.SKLearn.getEnvVarsV2()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}
