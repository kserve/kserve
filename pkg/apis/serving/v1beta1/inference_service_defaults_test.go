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

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInferenceServiceDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "v0.4.0",
			},
		},
		"Triton": {
			predictor: PredictorsConfig{
				Triton: PredictorConfig{
					ContainerImage:      "tritonserver",
					DefaultImageVersion: "20.03-py3",
					MultiModelServer:    false,
				},
			},
			predictorSpec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
		"XGBoost": {
			predictor: PredictorsConfig{
				XGBoost: PredictorProtocols{
					V1: &PredictorConfig{
						ContainerImage:      "xgboost",
						DefaultImageVersion: "v0.4.0",
						MultiModelServer:    false,
					},
				},
			},
			predictorSpec: PredictorSpec{
				XGBoost: &XGBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
		"ONNX": {
			predictor: PredictorsConfig{
				ONNX: PredictorConfig{
					ContainerImage:      "onnxruntime",
					DefaultImageVersion: "v1.0.0",
					MultiModelServer:    false,
				},
			},
			predictorSpec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
		"PMML": {
			predictor: PredictorsConfig{
				PMML: PredictorConfig{
					ContainerImage:      "pmmlserver",
					DefaultImageVersion: "v0.4.0",
					MultiModelServer:    false,
				},
			},
			predictorSpec: PredictorSpec{
				PMML: &PMMLSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
		"LightGBM": {
			predictor: PredictorsConfig{
				LightGBM: PredictorConfig{
					ContainerImage:      "lightgbm",
					DefaultImageVersion: "v0.4.0",
					MultiModelServer:    false,
				},
			},
			predictorSpec: PredictorSpec{
				LightGBM: &LightGBMSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
		"Paddle": {
			predictor: PredictorsConfig{
				Paddle: PredictorConfig{
					ContainerImage:      "paddleserver",
					DefaultImageVersion: "latest",
					MultiModelServer:    false,
				},
			},
			predictorSpec: PredictorSpec{
				Paddle: &PaddleServerSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
	}

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Predictors: scenario.predictor,
			Explainers: ExplainersConfig{
				AlibiExplainer: ExplainerConfig{
					ContainerImage:      "alibi",
					DefaultImageVersion: "v0.4.0",
				},
			},
		}
		isvc := InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: InferenceServiceSpec{
				Predictor: scenario.predictorSpec,
				Transformer: &TransformerSpec{
					PodSpec: PodSpec{
						Containers: []v1.Container{
							{
								Env: []v1.EnvVar{
									{
										Name:  "STORAGE_URI",
										Value: "s3://transformer",
									},
								},
							},
						},
					},
				},
				Explainer: &ExplainerSpec{
					Alibi: &AlibiExplainerSpec{
						ExplainerExtensionSpec: ExplainerExtensionSpec{
							StorageURI: "gs://testbucket/testmodel",
						},
					},
				},
			},
		}

		resources := v1.ResourceRequirements{Requests: defaultResource, Limits: defaultResource}
		isvc.Spec.DeepCopy()
		isvc.DefaultInferenceService(config, deployConfig)

		switch name {
		case "Tensorflow":
			g.Expect(isvc.Spec.Predictor.Tensorflow).To(gomega.BeNil())

		case "SKLearn":
			g.Expect(isvc.Spec.Predictor.SKLearn).To(gomega.BeNil())

		case "PyTorch":
			g.Expect(isvc.Spec.Predictor.PyTorch).To(gomega.BeNil())

		case "Triton":
			g.Expect(isvc.Spec.Predictor.Triton).To(gomega.BeNil())

		case "XGBoost":
			g.Expect(isvc.Spec.Predictor.XGBoost).To(gomega.BeNil())

		case "ONNX":
			g.Expect(isvc.Spec.Predictor.ONNX).To(gomega.BeNil())

		case "PMML":
			g.Expect(isvc.Spec.Predictor.PMML).To(gomega.BeNil())

		case "LightGBM":
			g.Expect(isvc.Spec.Predictor.LightGBM).To(gomega.BeNil())

		case "Paddle":
			g.Expect(isvc.Spec.Predictor.Paddle).To(gomega.BeNil())
		}
		g.Expect(isvc.Spec.Predictor.Model).NotTo(gomega.BeNil())
		g.Expect(isvc.Spec.Transformer.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))
		g.Expect(*isvc.Spec.Explainer.Alibi.RuntimeVersion).To(gomega.Equal("v0.4.0"))
		g.Expect(isvc.Spec.Explainer.Alibi.Resources).To(gomega.Equal(resources))
	}
}

func TestCustomPredictorDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "v0.4.0",
			},
		},
	}
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://transformer",
								},
							},
						},
					},
				},
			},
		},
	}
	resources := v1.ResourceRequirements{Requests: defaultResource, Limits: defaultResource}
	isvc.Spec.DeepCopy()
	isvc.DefaultInferenceService(config, deployConfig)
	g.Expect(isvc.Spec.Predictor.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))
}

func TestInferenceServiceDefaultsModelMeshAnnotation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{}
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			Annotations: map[string]string{
				constants.DeploymentMode: string(constants.ModelMeshDeployment),
			},
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
	}
	isvc.Spec.DeepCopy()
	isvc.DefaultInferenceService(config, deployConfig)
	g.Expect(isvc.Spec.Predictor.Model).To(gomega.BeNil())
	g.Expect(isvc.Spec.Predictor.Tensorflow).ToNot(gomega.BeNil())
}

func TestDefault(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
			Transformer: &TransformerSpec{
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://transformer",
								},
							},
						},
					},
				},
			},
			Explainer: &ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						StorageURI: "gs://testbucket/testmodel",
					},
				},
			},
		},
	}
	isvc.Default()
	g.Expect(isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
	g.Expect(isvc.Spec.Predictor.Tensorflow).To(gomega.BeNil())
}

func TestRuntimeDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		runtime string
		matcher types.GomegaMatcher
	}{
		"PyTorch": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					PyTorch: PredictorConfig{
						ContainerImage:      "pytorch/torchserve-kfs",
						DefaultImageVersion: "0.4.1",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			runtime: constants.TorchServe,
			matcher: gomega.Equal(constants.ProtocolV1),
		},
		"Triton": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					Triton: PredictorConfig{
						ContainerImage:      "tritonserver",
						DefaultImageVersion: "20.03-py3",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Triton: &TritonSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			runtime: constants.TritonServer,
			matcher: gomega.Equal(constants.ProtocolV2),
		},
		"MlServer": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					XGBoost: PredictorProtocols{
						V1: &PredictorConfig{
							ContainerImage:      "xgboost",
							DefaultImageVersion: "v0.4.0",
							MultiModelServer:    false,
						},
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			runtime: constants.MLServer,
			matcher: gomega.Equal(constants.ProtocolV2),
		},
	}
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}

	for name, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig)
		scenario.isvc.Spec.Predictor.Model.Runtime = &scenario.runtime
		scenario.isvc.SetRuntimeDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		switch name {

		case "PyTorch":
			g.Expect(scenario.isvc.Spec.Predictor.PyTorch).To(gomega.BeNil())

		case "Triton":
			g.Expect(scenario.isvc.Spec.Predictor.Triton).To(gomega.BeNil())

		case "MlServer":
			g.Expect(scenario.isvc.Spec.Predictor.XGBoost).To(gomega.BeNil())
		}
		g.Expect(*scenario.isvc.Spec.Predictor.Model.ProtocolVersion).To(scenario.matcher)
	}
}

func TestTorchServeDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	protocolVersion := constants.ProtocolV2
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		matcher types.GomegaMatcher
	}{
		"pytorch with protocol version 2": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					PyTorch: PredictorConfig{
						ContainerImage:      "pytorch/torchserve-kfs",
						DefaultImageVersion: "0.4.1",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://testbucket/testmodel"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			matcher: gomega.HaveKeyWithValue(constants.ServiceEnvelope, constants.ServiceEnvelopeKServeV2),
		},
		"pytorch with labels": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					PyTorch: PredictorConfig{
						ContainerImage:      "pytorch/torchserve-kfs",
						DefaultImageVersion: "0.4.1",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Labels: map[string]string{
						"Purpose": "Testing",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			matcher: gomega.HaveKeyWithValue("Purpose", "Testing"),
		},
	}
	runtime := constants.TorchServe
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}

	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig)
		scenario.isvc.Spec.Predictor.Model.Runtime = &runtime
		scenario.isvc.SetTorchServeDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.PyTorch).To(gomega.BeNil())
		g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.matcher)
	}
}

func TestSetTritonDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		matcher types.GomegaMatcher
	}{
		"Storage URI is nil": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					Triton: PredictorConfig{
						ContainerImage:      "tritonserver",
						DefaultImageVersion: "20.03-py3",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Triton: &TritonSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{},
						},
					},
				},
			},
			matcher: gomega.ContainElement("--model-control-mode=explicit"),
		},
	}
	runtime := constants.TritonServer
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}

	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig)
		scenario.isvc.Spec.Predictor.Model.Runtime = &runtime
		scenario.isvc.SetTritonDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.Triton).To(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.Model.Args).To(scenario.matcher)
	}
}

func TestMlServerDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		matcher map[string]types.GomegaMatcher
	}{
		"Storage URI is nil": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					SKlearn: PredictorProtocols{
						V1: &PredictorConfig{
							ContainerImage:      "sklearnserver",
							DefaultImageVersion: "v0.4.0",
							MultiModelServer:    false,
						},
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"env": gomega.ContainElement(v1.EnvVar{
					Name:  constants.MLServerLoadModelsStartupEnv,
					Value: strconv.FormatBool(false),
				}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue(constants.ModelClassLabel, constants.MLServerModelClassSKLearn),
			},
		},
		"XGBoost model": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					XGBoost: PredictorProtocols{
						V1: &PredictorConfig{
							ContainerImage:      "xgboost",
							DefaultImageVersion: "v0.4.0",
							MultiModelServer:    false,
						},
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						XGBoost: &XGBoostSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"env": gomega.ContainElements(
					v1.EnvVar{
						Name:  constants.MLServerModelNameEnv,
						Value: "foo",
					},
					v1.EnvVar{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue(constants.ModelClassLabel, constants.MLServerModelClassXGBoost),
			},
		},
		"LightGBM model": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					LightGBM: PredictorConfig{
						ContainerImage:      "lightgbm",
						DefaultImageVersion: "v0.4.0",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						LightGBM: &LightGBMSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"env": gomega.ContainElements(
					v1.EnvVar{
						Name:  constants.MLServerModelNameEnv,
						Value: "foo",
					},
					v1.EnvVar{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue(constants.ModelClassLabel, constants.MLServerModelClassLightGBM),
			},
		},
		"LightGBM model with labels": {
			config: &InferenceServicesConfig{
				Predictors: PredictorsConfig{
					LightGBM: PredictorConfig{
						ContainerImage:      "lightgbm",
						DefaultImageVersion: "v0.4.0",
						MultiModelServer:    false,
					},
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Labels: map[string]string{
						"Purpose": "Testing",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						LightGBM: &LightGBMSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"env": gomega.ContainElements(
					v1.EnvVar{
						Name:  constants.MLServerModelNameEnv,
						Value: "foo",
					},
					v1.EnvVar{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue("Purpose", "Testing"),
			},
		},
	}
	runtime := constants.MLServer
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}

	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig)
		scenario.isvc.Spec.Predictor.Model.Runtime = &runtime
		scenario.isvc.SetMlServerDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.Model.Env).To(scenario.matcher["env"])
		g.Expect(*scenario.isvc.Spec.Predictor.Model.ProtocolVersion).To(scenario.matcher["protocolVersion"])
		g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.matcher["labels"])
	}
}
