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

	"github.com/golang/protobuf/proto"
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
		"ValidStorageUri": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("hdfs://modelzoo"),
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
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

	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			SKlearn: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "sklearnserver",
					DefaultImageVersion: "v0.4.0",
					MultiModelServer:    true,
				},
				V2: &PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "0.1.2",
					MultiModelServer:    true,
				},
			},
		},
	}

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultRuntimeVersion": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("v0.4.0"),
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
						RuntimeVersion:  proto.String("0.1.2"),
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
			scenario.spec.SKLearn.Default(&config)
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
	var config = InferenceServicesConfig{
		Predictors: PredictorsConfig{
			SKlearn: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "someOtherImage",
					DefaultImageVersion: "0.2.0",
					MultiModelServer:    true,
				},
			},
		},
	}
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
				Image:     "someOtherImage:0.2.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name=someName",
					"--model_dir=/mnt/models",
					"--http_port=8080",
				},
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
				Image:     "someOtherImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name=someName",
					"--model_dir=/mnt/models",
					"--http_port=8080",
				},
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
				Args: []string{
					"--model_name=someName",
					"--model_dir=/mnt/models",
					"--http_port=8080",
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
				Image:     "someOtherImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name=someName",
					"--model_dir=/mnt/models",
					"--http_port=8080",
					"--workers=1",
				},
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
				Image:     "someOtherImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name=someName",
					"--model_dir=/mnt/models",
					"--http_port=8080",
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
	var config = InferenceServicesConfig{
		Predictors: PredictorsConfig{
			SKlearn: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "someOtherImage",
					DefaultImageVersion: "0.1.0",
					MultiModelServer:    true,
				},
				V2: &PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "0.1.2",
					MultiModelServer:    true,
				},
			},
		},
	}
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
				Image:     "mlserver:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Env: []v1.EnvVar{
					{
						Name:  constants.MLServerHTTPPortEnv,
						Value: fmt.Sprint(constants.MLServerISRestPort),
					},
					{
						Name:  constants.MLServerGRPCPortEnv,
						Value: fmt.Sprint(constants.MLServerISGRPCPort),
					},
					{
						Name:  constants.MLServerModelsDirEnv,
						Value: constants.DefaultModelLocalMountPath,
					},
					{
						Name:  constants.MLServerModelImplementationEnv,
						Value: constants.MLServerSKLearnImplementation,
					},
					{
						Name:  constants.MLServerModelNameEnv,
						Value: "sklearn",
					},
					{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					},
				},
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
				Env: []v1.EnvVar{
					{
						Name:  constants.MLServerHTTPPortEnv,
						Value: fmt.Sprint(constants.MLServerISRestPort),
					},
					{
						Name:  constants.MLServerGRPCPortEnv,
						Value: fmt.Sprint(constants.MLServerISGRPCPort),
					},
					{
						Name:  constants.MLServerModelsDirEnv,
						Value: constants.DefaultModelLocalMountPath,
					},
					{
						Name:  constants.MLServerModelImplementationEnv,
						Value: constants.MLServerSKLearnImplementation,
					},
					{
						Name:  constants.MLServerModelNameEnv,
						Value: "sklearn",
					},
					{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					},
				},
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
				Image:     "mlserver:0.1.2",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Env: []v1.EnvVar{
					{
						Name:  constants.MLServerHTTPPortEnv,
						Value: fmt.Sprint(constants.MLServerISRestPort),
					},
					{
						Name:  constants.MLServerGRPCPortEnv,
						Value: fmt.Sprint(constants.MLServerISGRPCPort),
					},
					{
						Name:  constants.MLServerModelsDirEnv,
						Value: constants.DefaultModelLocalMountPath,
					},
					{
						Name:  constants.MLServerLoadModelsStartupEnv,
						Value: fmt.Sprint(false),
					},
					{
						Name:  constants.MLServerModelImplementationEnv,
						Value: constants.MLServerSKLearnImplementation,
					},
				},
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

func TestSKLearnIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				SKlearn: PredictorProtocols{
					V1: &PredictorConfig{
						ContainerImage:      "sklearnserver",
						DefaultImageVersion: "v0.4.0",
						MultiModelServer:    mmsCase,
					},
					V2: &PredictorConfig{
						ContainerImage:      "mlserver",
						DefaultImageVersion: "0.1.2",
						MultiModelServer:    mmsCase,
					},
				},
			},
		}

		protocolV1 := constants.ProtocolV1
		protocolV2 := constants.ProtocolV2

		defaultResource = v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1"),
			v1.ResourceMemory: resource.MustParse("2Gi"),
		}
		scenarios := map[string]struct {
			spec     PredictorSpec
			expected bool
		}{
			"DefaultRuntimeVersion": {
				spec: PredictorSpec{
					SKLearn: &SKLearnSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol": {
				spec: PredictorSpec{
					SKLearn: &SKLearnSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							ProtocolVersion: &protocolV1,
						},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol2": {
				spec: PredictorSpec{
					SKLearn: &SKLearnSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							ProtocolVersion: &protocolV2,
						},
					},
				},
				expected: mmsCase,
			},
		}

		for name, scenario := range scenarios {
			t.Run(name, func(t *testing.T) {
				scenario.spec.SKLearn.Default(&config)
				res := scenario.spec.SKLearn.IsMMS(&config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}

func TestSKLearnIsFrameworkSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	sklearn := "sklearn"
	unsupportedFramework := "framework"
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			SKlearn: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "sklearnserver",
					DefaultImageVersion: "latest",
					SupportedFrameworks: []string{sklearn},
				},
				V2: &PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "0.1.2",
					SupportedFrameworks: []string{sklearn},
				},
			},
		},
	}

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec      PredictorSpec
		framework string
		expected  bool
	}{
		"SupportedFrameworkV1": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
					},
				},
			},
			framework: sklearn,
			expected:  true,
		},
		"SupportedFrameworkV2": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			framework: sklearn,
			expected:  true,
		},
		"UnsupportedFrameworkV1": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
					},
				},
			},
			framework: unsupportedFramework,
			expected:  false,
		},
		"UnsupportedFrameworkV2": {
			spec: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			framework: unsupportedFramework,
			expected:  false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.SKLearn.Default(&config)
			res := scenario.spec.SKLearn.IsFrameworkSupported(scenario.framework, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
