/*
Copyright 2021 kubeflow.org.

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

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCatBoostValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"ValidStorageUri": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("hdfs://modelzoo"),
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidReplica": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas: GetIntReference(3),
					MaxReplicas: 2,
				},
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("hdfs://modelzoo"),
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidContainerConcurrency": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          GetIntReference(3),
					ContainerConcurrency: proto.Int64(-1),
				},
				CatBoost: &CatBoostSpec{
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
			res := scenario.spec.CatBoost.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestCatBoostDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			CatBoost: PredictorConfig{
				ContainerImage:      "mlserver",
				DefaultImageVersion: "v0.1.2",
				MultiModelServer:    true,
			},
		},
	}

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
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("v0.1.2"),
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
		"DefaultRuntimeVersionAndProtocol": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			expected: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("v0.1.2"),
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
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
					},
				},
			},
			expected: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("v0.3.0"),
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
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.CatBoost.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateCatBoostModelServingContainerV2(t *testing.T) {
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
			CatBoost: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
				MultiModelServer:    true,
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
					Name: "CatBoost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						CatBoost: &CatBoostSpec{
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
				Image:     "someOtherImage:0.1.0",
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
						Value: constants.MLServerCatBoostImplementation,
					},
					{
						Name:  constants.MLServerModelNameEnv,
						Value: "CatBoost",
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
					Name: "CatBoost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						CatBoost: &CatBoostSpec{
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
						Value: constants.MLServerCatBoostImplementation,
					},
					{
						Name:  constants.MLServerModelNameEnv,
						Value: "CatBoost",
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
					Name: "CatBoost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						CatBoost: &CatBoostSpec{
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
				Image:     "someOtherImage:0.1.0",
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
						Value: constants.MLServerCatBoostImplementation,
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

func TestCatBoostIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				CatBoost: PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "v0.1.2",
					MultiModelServer:    mmsCase,
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
					CatBoost: &CatBoostSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol": {
				spec: PredictorSpec{
					CatBoost: &CatBoostSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							ProtocolVersion: &protocolV1,
						},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol2": {
				spec: PredictorSpec{
					CatBoost: &CatBoostSpec{
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
				scenario.spec.CatBoost.Default(&config)
				res := scenario.spec.CatBoost.IsMMS(&config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}

func TestCatBoostIsFrameworkSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	CatBoost := "CatBoost"
	unsupportedFramework := "framework"
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			CatBoost: PredictorConfig{
				ContainerImage:      "mlserver",
				DefaultImageVersion: "0.1.2",
				SupportedFrameworks: []string{CatBoost},
			},
		},
	}

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
		"SupportedFrameworkV2": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			framework: CatBoost,
			expected:  true,
		},
		"UnsupportedFrameworkV2": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
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
			scenario.spec.CatBoost.Default(&config)
			res := scenario.spec.CatBoost.IsFrameworkSupported(scenario.framework, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
