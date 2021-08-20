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

package utils

import (
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	. "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsMMSPredictor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}
	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				LightGBM: PredictorConfig{
					ContainerImage:      "lightgbm",
					DefaultImageVersion: "v0.4.0",
					MultiModelServer:    mmsCase,
				},
				ONNX: PredictorConfig{
					ContainerImage:      "onnxruntime",
					DefaultImageVersion: "v1.0.0",
					MultiModelServer:    mmsCase,
				},
				PMML: PredictorConfig{
					ContainerImage:      "pmmlserver",
					DefaultImageVersion: "v0.4.0",
					MultiModelServer:    mmsCase,
				},
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
				PyTorch: PredictorConfig{
					ContainerImage:      "pytorch/torchserve-kfs",
					DefaultImageVersion: "0.4.1",
					MultiModelServer:    mmsCase,
				},
				Tensorflow: PredictorConfig{
					ContainerImage:         "tfserving",
					DefaultImageVersion:    "1.14.0",
					DefaultGpuImageVersion: "1.14.0-gpu",
					DefaultTimeout:         60,
					MultiModelServer:       mmsCase,
				},
				Triton: PredictorConfig{
					ContainerImage:      "tritonserver",
					DefaultImageVersion: "20.03-py3",
					MultiModelServer:    mmsCase,
				},
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

		scenarios := map[string]struct {
			isvc     InferenceService
			expected bool
		}{
			"LightGBM": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "lightgbm",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							LightGBM: &LightGBMSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Image:     "customImage:0.1.0",
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"LightGBMWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "lightgbmWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							LightGBM: &LightGBMSpec{
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
				expected: false,
			},
			"Onnx": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "onnx",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							ONNX: &ONNXRuntimeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Image:     "mcr.microsoft.com/onnxruntime/server:v0.5.0",
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"OnnxWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "onnxWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							ONNX: &ONNXRuntimeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Image:     "mcr.microsoft.com/onnxruntime/server:v0.5.0",
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"Pmml": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pmml",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PMML: &PMMLSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"PmmlWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pmmlWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PMML: &PMMLSpec{
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
				expected: false,
			},
			"SKLearnV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"SKLearnV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn2",
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
				expected: mmsCase,
			},
			"SKLearnV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearnWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"SKLearnV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"tfserving": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tfserving",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Tensorflow: &TFServingSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"TfservingWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tfservingWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Tensorflow: &TFServingSpec{
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
				expected: false,
			},
			"PyTorchV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"PyTorchV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
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
				expected: mmsCase,
			},
			"PyTorchV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorchWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"PyTorchV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"Triton": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "triton",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Triton: &TritonSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"TritonWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tritonWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Triton: &TritonSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"XGBoostV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									RuntimeVersion: proto.String("0.1.0"),
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"XGBoostV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									RuntimeVersion:  proto.String("0.1.0"),
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"XGBoostV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboostWithURI",
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
				expected: false,
			},
			"XGBoostV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									RuntimeVersion:  proto.String("0.1.0"),
									Container: v1.Container{
										Resources: requestedResource,
									},
								},
							},
						},
					},
				},
				expected: false,
			},
			"CustomSpec": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "CustomSpecMMS",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PodSpec: PodSpec{
								Containers: []v1.Container{
									{Name: "kfserving-container",
										Image: "some-image",
										Env:   []v1.EnvVar{{Name: constants.CustomSpecMultiModelServerEnvVarKey, Value: strconv.FormatBool(mmsCase)}},
									},
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"CustomSpecWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "CustomSpecMMSWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PodSpec: PodSpec{
								Containers: []v1.Container{
									{Name: "kfserving-container",
										Image: "some-image",
										Env:   []v1.EnvVar{{Name: constants.CustomSpecMultiModelServerEnvVarKey, Value: strconv.FormatBool(mmsCase)}, {Name: constants.CustomSpecStorageUriEnvVarKey, Value: "gs://some-uri"}},
									},
								},
							},
						},
					},
				},
				expected: false,
			},
		}

		for name, scenario := range scenarios {
			t.Run(name, func(t *testing.T) {
				res := IsMMSPredictor(&scenario.isvc.Spec.Predictor, &config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}

func TestIsMemoryResourceAvailable(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			LightGBM: PredictorConfig{
				ContainerImage:      "lightgbm",
				DefaultImageVersion: "v0.4.0",
			},
			ONNX: PredictorConfig{
				ContainerImage:      "onnxruntime",
				DefaultImageVersion: "v1.0.0",
			},
			PMML: PredictorConfig{
				ContainerImage:      "pmmlserver",
				DefaultImageVersion: "v0.4.0",
			},
			SKlearn: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "sklearnserver",
					DefaultImageVersion: "v0.4.0",
				},
				V2: &PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "0.1.2",
				},
			},
			PyTorch: PredictorConfig{
				ContainerImage:      "pytorch/torchserve-kfs",
				DefaultImageVersion: "0.4.1",
			},
			Tensorflow: PredictorConfig{
				ContainerImage:         "tfserving",
				DefaultImageVersion:    "1.14.0",
				DefaultGpuImageVersion: "1.14.0-gpu",
				DefaultTimeout:         60,
			},
			Triton: PredictorConfig{
				ContainerImage:      "tritonserver",
				DefaultImageVersion: "20.03-py3",
			},
			XGBoost: PredictorProtocols{
				V1: &PredictorConfig{
					ContainerImage:      "xgboost",
					DefaultImageVersion: "v0.4.0",
				},
				V2: &PredictorConfig{
					ContainerImage:      "mlserver",
					DefaultImageVersion: "v0.1.2",
				},
			},
		},
	}
	reqResourcesScenarios := map[string]struct {
		resource v1.ResourceRequirements
		expected bool
	}{
		"EnoughMemoryResource": {
			// Enough memory
			resource: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("100Gi"),
				},
				Requests: v1.ResourceList{
					"cpu":    resource.MustParse("90m"),
					"memory": resource.MustParse("100Gi"),
				},
			},
			expected: true,
		},
		"NotEnoughMemoryResource": {
			// Enough memory
			resource: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("1Mi"),
				},
				Requests: v1.ResourceList{
					"cpu":    resource.MustParse("90m"),
					"memory": resource.MustParse("1Mi"),
				},
			},
			expected: false,
		},
	}

	totalReqMemory := resource.MustParse("1Gi")

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	for _, reqResourcesScenario := range reqResourcesScenarios {
		scenarios := map[string]struct {
			isvc InferenceService
		}{
			"LightGBM": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "lightgbm",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							LightGBM: &LightGBMSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Image:     "customImage:0.1.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"LightGBMWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "lightgbmWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							LightGBM: &LightGBMSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Image:     "customImage:0.1.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"Onnx": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "onnx",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							ONNX: &ONNXRuntimeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Image:     "mcr.microsoft.com/onnxruntime/server:v0.5.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"OnnxWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "onnxWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							ONNX: &ONNXRuntimeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Image:     "mcr.microsoft.com/onnxruntime/server:v0.5.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"Pmml": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pmml",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PMML: &PMMLSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PmmlWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pmmlWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PMML: &PMMLSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearnWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"tfserving": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tfserving",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Tensorflow: &TFServingSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"TfservingWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tfservingWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Tensorflow: &TFServingSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorchWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"Triton": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "triton",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Triton: &TritonSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"TritonWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tritonWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Triton: &TritonSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									RuntimeVersion: proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									RuntimeVersion:  proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboostWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:     proto.String("gs://someUri"),
									RuntimeVersion: proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									RuntimeVersion:  proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
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
				scenario.isvc.Spec.Predictor.GetImplementation().Default(&config)
				res := IsMemoryResourceAvailable(&scenario.isvc, totalReqMemory, &config)
				if !g.Expect(res).To(gomega.Equal(reqResourcesScenario.expected)) {
					t.Errorf("got %t, want %t with memory %s vs %s", res, reqResourcesScenario.expected, reqResourcesScenario.resource.Limits.Memory().String(), totalReqMemory.String())
				}
			})
		}
	}
}
