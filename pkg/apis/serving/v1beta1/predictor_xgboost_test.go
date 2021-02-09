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

func TestXGBoostValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				XGBoost: &XGBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"ValidStorageUri": {
			spec: PredictorSpec{
				XGBoost: &XGBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				XGBoost: &XGBoostSpec{
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
				XGBoost: &XGBoostSpec{
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
				XGBoost: &XGBoostSpec{
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
			res := scenario.spec.XGBoost.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestXGBoostDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			XGBoost: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "xgboost",
					DefaultImageVersion: "v0.4.0",
					MultiModelServer:    true,
				},
				V2: &PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "v0.1.2",
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
				XGBoost: &XGBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				XGBoost: &XGBoostSpec{
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
				XGBoost: &XGBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			expected: PredictorSpec{
				XGBoost: &XGBoostSpec{
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
				XGBoost: &XGBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
					},
				},
			},
			expected: PredictorSpec{
				XGBoost: &XGBoostSpec{
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
			scenario.spec.XGBoost.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateXGBoostModelServingContainerV1(t *testing.T) {

	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}
	var config = InferenceServicesConfig{
		Predictors: PredictorsConfig{
			XGBoost: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "someOtherImage",
					DefaultImageVersion: "0.1.0",
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
					Name: "xgboost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
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
					"--nthread=1",
				},
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "xgboost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
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
					"--nthread=1",
				},
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "xgboost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(1),
						},
						XGBoost: &XGBoostSpec{
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
					"--nthread=1",
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

func TestCreateXGBoostModelServingContainerV2(t *testing.T) {
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
			XGBoost: PredictorProtocols{
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
					Name: "xgboost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
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
						Value: constants.MLServerXGBoostImplementation,
					},
					{
						Name:  constants.MLServerModelNameEnv,
						Value: "xgboost",
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
					Name: "xgboost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
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
						Value: constants.MLServerXGBoostImplementation,
					},
					{
						Name:  constants.MLServerModelNameEnv,
						Value: "xgboost",
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
					Name: "xgboost",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
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
						Value: constants.MLServerXGBoostImplementation,
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

func TestXGBoostIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				XGBoost: PredictorProtocols{
					V1: &PredictorConfig{
						ContainerImage:      "xgboost",
						DefaultImageVersion: "v0.4.0",
						MultiModelServer:    mmsCase,
					},
					V2: &PredictorConfig{
						ContainerImage:      "mlserver",
						DefaultImageVersion: "v0.1.2",
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
					XGBoost: &XGBoostSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol": {
				spec: PredictorSpec{
					XGBoost: &XGBoostSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							ProtocolVersion: &protocolV1,
						},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol2": {
				spec: PredictorSpec{
					XGBoost: &XGBoostSpec{
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
				scenario.spec.XGBoost.Default(&config)
				res := scenario.spec.XGBoost.IsMMS(&config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}
