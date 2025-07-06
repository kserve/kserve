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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInferenceServiceDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		config       *InferenceServicesConfig
		deployConfig *DeployConfig
		isvc         InferenceService
		runtime      string
		matcher      map[string]types.GomegaMatcher
	}{
		"Serverless": {
			config: &InferenceServicesConfig{
				Explainers: ExplainersConfig{
					ARTExplainer: ExplainerConfig{
						ContainerImage:      "art",
						DefaultImageVersion: "v0.4.0",
					},
				},
				Resource: ResourceConfig{
					CPULimit:      "1",
					MemoryLimit:   "2Gi",
					CPURequest:    "1",
					MemoryRequest: "2Gi",
				},
			},
			deployConfig: &DeployConfig{
				DefaultDeploymentMode: "Serverless",
			},
			isvc: InferenceService{
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
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
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
						ART: &ARTExplainerSpec{
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI: "gs://testbucket/testmodel",
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"Annotations": gomega.BeNil(),
			},
		},
		"When annotations is nil in raw deployment": {
			config: &InferenceServicesConfig{
				Explainers: ExplainersConfig{
					ARTExplainer: ExplainerConfig{
						ContainerImage:      "art",
						DefaultImageVersion: "v0.4.0",
					},
				},
				Resource: ResourceConfig{
					CPULimit:      "1",
					MemoryLimit:   "2Gi",
					CPURequest:    "1",
					MemoryRequest: "2Gi",
				},
			},
			deployConfig: &DeployConfig{
				DefaultDeploymentMode: string(constants.RawDeployment),
			},
			isvc: InferenceService{
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
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
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
						ART: &ARTExplainerSpec{
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI: "gs://testbucket/testmodel",
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"Annotations": gomega.Equal(map[string]string{constants.DeploymentMode: string(constants.RawDeployment)}),
			},
		},
		"ONNX": {
			config: &InferenceServicesConfig{
				Explainers: ExplainersConfig{
					ARTExplainer: ExplainerConfig{
						ContainerImage:      "art",
						DefaultImageVersion: "v0.4.0",
					},
				},
				Resource: ResourceConfig{
					CPULimit:      "1",
					MemoryLimit:   "2Gi",
					CPURequest:    "1",
					MemoryRequest: "2Gi",
				},
			},
			deployConfig: &DeployConfig{
				DefaultDeploymentMode: "Serverless",
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ONNX: &ONNXRuntimeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
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
						ART: &ARTExplainerSpec{
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI: "gs://testbucket/testmodel",
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"Annotations": gomega.BeNil(),
			},
		},
		"PMML": {
			config: &InferenceServicesConfig{
				Explainers: ExplainersConfig{
					ARTExplainer: ExplainerConfig{
						ContainerImage:      "art",
						DefaultImageVersion: "v0.4.0",
					},
				},
				Resource: ResourceConfig{
					CPULimit:      "1",
					MemoryLimit:   "2Gi",
					CPURequest:    "1",
					MemoryRequest: "2Gi",
				},
			},
			deployConfig: &DeployConfig{
				DefaultDeploymentMode: "Serverless",
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PMML: &PMMLSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
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
						ART: &ARTExplainerSpec{
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI: "gs://testbucket/testmodel",
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"Annotations": gomega.BeNil(),
			},
		},
		"Paddle": {
			config: &InferenceServicesConfig{
				Explainers: ExplainersConfig{
					ARTExplainer: ExplainerConfig{
						ContainerImage:      "art",
						DefaultImageVersion: "v0.4.0",
					},
				},
				Resource: ResourceConfig{
					CPULimit:      "1",
					MemoryLimit:   "2Gi",
					CPURequest:    "1",
					MemoryRequest: "2Gi",
				},
			},
			deployConfig: &DeployConfig{
				DefaultDeploymentMode: "Serverless",
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Paddle: &PaddleServerSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
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
						ART: &ARTExplainerSpec{
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI: "gs://testbucket/testmodel",
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"Annotations": gomega.BeNil(),
			},
		},
	}

	for _, scenario := range scenarios {
		resources := corev1.ResourceRequirements{Requests: defaultResource, Limits: defaultResource}
		scenario.isvc.Spec.DeepCopy()
		scenario.isvc.DefaultInferenceService(scenario.config, scenario.deployConfig, nil, nil)

		g.Expect(scenario.isvc.Spec.Predictor.Tensorflow).To(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.ONNX).To(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.PMML).To(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.Paddle).To(gomega.BeNil())
		g.Expect(scenario.isvc.ObjectMeta.Annotations).To(scenario.matcher["Annotations"])
		g.Expect(scenario.isvc.Spec.Predictor.Model).NotTo(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Transformer.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))
		g.Expect(*scenario.isvc.Spec.Explainer.ART.RuntimeVersion).To(gomega.Equal("v0.4.0"))
		g.Expect(scenario.isvc.Spec.Explainer.ART.Resources).To(gomega.Equal(resources))
	}
}

func TestCustomPredictorDefaultsConfig(t *testing.T) {
	expectedResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{
		Explainers: ExplainersConfig{
			ARTExplainer: ExplainerConfig{
				ContainerImage:      "art",
				DefaultImageVersion: "v0.4.0",
			},
		},
		Resource: ResourceConfig{
			CPULimit:      "2",
			MemoryLimit:   "4Gi",
			CPURequest:    "2",
			MemoryRequest: "4Gi",
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
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []corev1.EnvVar{
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
	resources := corev1.ResourceRequirements{Requests: expectedResource, Limits: expectedResource}
	isvc.Spec.DeepCopy()
	isvc.DefaultInferenceService(config, deployConfig, nil, nil)
	g.Expect(isvc.Spec.Predictor.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))

	isvcWithoutContainerName := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
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
	isvcWithoutContainerName.Spec.DeepCopy()
	isvcWithoutContainerName.DefaultInferenceService(config, deployConfig, nil, nil)
	g.Expect(isvcWithoutContainerName.Spec.Predictor.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))
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
	isvc.DefaultInferenceService(config, deployConfig, nil, nil)
	g.Expect(isvc.Spec.Predictor.Model).To(gomega.BeNil())
	g.Expect(isvc.Spec.Predictor.Tensorflow).ToNot(gomega.BeNil())
}

func TestRuntimeDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		runtime string
		matcher types.GomegaMatcher
	}{
		"PyTorch": {
			config: &InferenceServicesConfig{},
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
			config: &InferenceServicesConfig{},
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
			config: &InferenceServicesConfig{},
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
	for name, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig, nil, nil)
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

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	protocolVersion := constants.ProtocolV2
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		matcher types.GomegaMatcher
	}{
		"pytorch with protocol version 2": {
			config: &InferenceServicesConfig{},
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
			config: &InferenceServicesConfig{},
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
	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig, nil, nil)
		scenario.isvc.Spec.Predictor.Model.Runtime = &runtime
		scenario.isvc.SetTorchServeDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.PyTorch).To(gomega.BeNil())
		g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.matcher)
	}
}

func TestSetTritonDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		matcher types.GomegaMatcher
	}{
		"Storage URI is nil": {
			config: &InferenceServicesConfig{},
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
		"Default Protocol": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat:            ModelFormat{Name: "triton"},
							PredictorExtensionSpec: PredictorExtensionSpec{},
						},
					},
				},
			},
			matcher: gomega.ContainElement("--model-control-mode=explicit"),
		},
	}
	runtime := constants.TritonServer
	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig, nil, nil)
		scenario.isvc.Spec.Predictor.Model.Runtime = &runtime
		scenario.isvc.SetTritonDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		g.Expect(*scenario.isvc.Spec.Predictor.Model.ProtocolVersion).To(gomega.Equal(constants.ProtocolV2))
		g.Expect(scenario.isvc.Spec.Predictor.Triton).To(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.Model.Args).To(scenario.matcher)
	}
}

func TestMlServerDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	scenarios := map[string]struct {
		config  *InferenceServicesConfig
		isvc    InferenceService
		matcher map[string]types.GomegaMatcher
	}{
		"Storage URI is nil": {
			config: &InferenceServicesConfig{},
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
				"env": gomega.ContainElement(corev1.EnvVar{
					Name:  constants.MLServerLoadModelsStartupEnv,
					Value: strconv.FormatBool(false),
				}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue(constants.ModelClassLabel, constants.MLServerModelClassSKLearn),
			},
		},
		"XGBoost model": {
			config: &InferenceServicesConfig{},
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
					corev1.EnvVar{
						Name:  constants.MLServerModelNameEnv,
						Value: "foo",
					},
					corev1.EnvVar{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue(constants.ModelClassLabel, constants.MLServerModelClassXGBoost),
			},
		},
		"LightGBM model": {
			config: &InferenceServicesConfig{},
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
					corev1.EnvVar{
						Name:  constants.MLServerModelNameEnv,
						Value: "foo",
					},
					corev1.EnvVar{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue(constants.ModelClassLabel, constants.MLServerModelClassLightGBM),
			},
		},
		"LightGBM model with labels": {
			config: &InferenceServicesConfig{},
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
					corev1.EnvVar{
						Name:  constants.MLServerModelNameEnv,
						Value: "foo",
					},
					corev1.EnvVar{
						Name:  constants.MLServerModelURIEnv,
						Value: constants.DefaultModelLocalMountPath,
					}),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"labels":          gomega.HaveKeyWithValue("Purpose", "Testing"),
			},
		},
	}
	runtime := constants.MLServer
	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig, nil, nil)
		scenario.isvc.Spec.Predictor.Model.Runtime = &runtime
		scenario.isvc.SetMlServerDefaults()
		g.Expect(scenario.isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
		g.Expect(scenario.isvc.Spec.Predictor.Model.Env).To(scenario.matcher["env"])
		g.Expect(*scenario.isvc.Spec.Predictor.Model.ProtocolVersion).To(scenario.matcher["protocolVersion"])
		g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.matcher["labels"])
	}
}

func TestLocalModelAnnotation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	protocolVersion := constants.ProtocolV2
	gpu1, gpu2 := "gpu1", "gpu2"
	model1 := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model1",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://bucket/model",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}
	model2 := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model2",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://bucket/model2",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}
	localModels := &v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{*model1, *model2}}

	scenarios := map[string]struct {
		config            *InferenceServicesConfig
		isvc              InferenceService
		labelMatcher      types.GomegaMatcher
		annotationMatcher types.GomegaMatcher
	}{
		"isvc without node group annotation with LocalModelCache": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://bucket/model"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			labelMatcher:      gomega.HaveKeyWithValue(constants.LocalModelLabel, model1.Name),
			annotationMatcher: gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, model1.Name+"-"+gpu1),
		},
		"isvc with node group annotation with LocalModelCache": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						constants.NodeGroupAnnotationKey: gpu2, // should append this to PVC name
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://bucket/model"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			labelMatcher:      gomega.HaveKeyWithValue(constants.LocalModelLabel, model1.Name),
			annotationMatcher: gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, model1.Name+"-"+gpu2),
		},
		"isvc with overlapping storage URIs": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						constants.NodeGroupAnnotationKey: gpu2, // should append this to PVC name
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://bucket/model2"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			labelMatcher:      gomega.HaveKeyWithValue(constants.LocalModelLabel, model2.Name),
			annotationMatcher: gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, model2.Name+"-"+gpu2),
		},
		"isvc without LocalModelCache": {
			config: &InferenceServicesConfig{},
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
								// This is not considered a match for "gs://bucket/model" on the LocalModelCache
								StorageURI: proto.String("gs://bucket/model3"),
							},
						},
					},
				},
			},
			labelMatcher:      gomega.Not(gomega.HaveKey(constants.LocalModelLabel)),
			annotationMatcher: gomega.Not(gomega.HaveKey(constants.LocalModelPVCNameAnnotationKey)),
		},
		"isvc with node group annotation without LocalModelCache": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						constants.NodeGroupAnnotationKey: "some-random-gpu", // should not match any local model cache
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://bucket/model"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			labelMatcher:      gomega.Not(gomega.HaveKey(constants.LocalModelLabel)),
			annotationMatcher: gomega.Not(gomega.HaveKey(constants.LocalModelPVCNameAnnotationKey)),
		},
	}

	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig, nil, localModels)
		g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.labelMatcher)
		g.Expect(scenario.isvc.ObjectMeta.Annotations).To(scenario.annotationMatcher)
	}
}

func TestLocalModelAnnotationWithTensorflow(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	gpu1, gpu2 := "gpu1", "gpu2"
	model1 := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model1",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://bucket/model",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}
	model2 := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model2",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://bucket/model2",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}
	localModels := &v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{*model1, *model2}}

	scenarios := map[string]struct {
		isvc    InferenceService
		matcher map[string]types.GomegaMatcher
	}{
		"Match model1": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/model"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"labels":              gomega.HaveKeyWithValue(constants.LocalModelLabel, "model1"),
				"sourceUriAnnotation": gomega.HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "gs://bucket/model"),
				"pvcNameAnnotation":   gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "model1-gpu1"),
			},
		},
		"Match model2": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/model2"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"labels":              gomega.HaveKeyWithValue(constants.LocalModelLabel, "model2"),
				"sourceUriAnnotation": gomega.HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "gs://bucket/model2"),
				"pvcNameAnnotation":   gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "model2-gpu1"),
			},
		},
		"With node group annotation": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						constants.NodeGroupAnnotationKey: gpu2,
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/model"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"labels":              gomega.HaveKeyWithValue(constants.LocalModelLabel, "model1"),
				"sourceUriAnnotation": gomega.HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "gs://bucket/model"),
				"pvcNameAnnotation":   gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "model1-gpu2"),
			},
		},
		"No model match": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/model3"),
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"labels": gomega.Not(gomega.HaveKey(constants.LocalModelLabel)),
			},
		},
		"No storage URI": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
										{
											Name:  "MODEL_NAME",
											Value: "test-model",
										},
									},
								},
							},
						},
					},
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"labels": gomega.Not(gomega.HaveKey(constants.LocalModelLabel)),
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.isvc.DefaultInferenceService(nil, deployConfig, nil, localModels)
			g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.matcher["labels"])
			if _, ok := scenario.matcher["sourceUriAnnotation"]; ok {
				g.Expect(scenario.isvc.ObjectMeta.Annotations).To(scenario.matcher["sourceUriAnnotation"])
				g.Expect(scenario.isvc.ObjectMeta.Annotations).To(scenario.matcher["pvcNameAnnotation"])
			}
		})
	}
}

func TestDisableAutomountServiceAccountToken(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	securityConfig := &SecurityConfig{
		AutoMountServiceAccountToken: false,
	}

	scenarios := map[string]struct {
		isvc    InferenceService
		matcher map[string]types.GomegaMatcher
	}{
		"Predictor only": {
			isvc: InferenceService{
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
				},
			},
			matcher: map[string]types.GomegaMatcher{
				"predictor": gomega.Equal(false),
			},
		},
		"With transformer": {
			isvc: InferenceService{
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
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
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
			},
			matcher: map[string]types.GomegaMatcher{
				"predictor":   gomega.Equal(false),
				"transformer": gomega.Equal(false),
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.isvc.DefaultInferenceService(nil, deployConfig, securityConfig, nil)
			g.Expect(*scenario.isvc.Spec.Predictor.AutomountServiceAccountToken).To(scenario.matcher["predictor"])

			if scenario.isvc.Spec.Transformer != nil {
				g.Expect(*scenario.isvc.Spec.Transformer.AutomountServiceAccountToken).To(scenario.matcher["transformer"])
			}

			if scenario.isvc.Spec.Explainer != nil {
				g.Expect(*scenario.isvc.Spec.Explainer.AutomountServiceAccountToken).To(scenario.matcher["explainer"])
			}
		})
	}
}

func TestDefault(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// defaulter := &InferenceServiceDefaulter{}

	scenarios := map[string]struct {
		isvc       InferenceService
		mutateFunc func(*InferenceService) *InferenceService
		verify     func(g *gomega.WithT, isvc *InferenceService)
	}{
		"DefaultWithTensorflowPredictor": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tensorflow-model",
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
				},
			},
			verify: func(g *gomega.WithT, isvc *InferenceService) {
				g.Expect(isvc.Spec.Predictor.Tensorflow).To(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model).NotTo(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model.ModelFormat.Name).To(gomega.Equal(constants.SupportedModelTensorflow))
				g.Expect(isvc.Spec.Predictor.Model.StorageURI).To(gomega.Equal(proto.String("gs://testbucket/testmodel")))
			},
		},
		"DefaultWithTritonPredictor": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "triton-model",
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
			verify: func(g *gomega.WithT, isvc *InferenceService) {
				g.Expect(isvc.Spec.Predictor.Triton).To(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model).NotTo(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model.ModelFormat.Name).To(gomega.Equal(constants.SupportedModelTriton))
				g.Expect(isvc.Spec.Predictor.Model.StorageURI).To(gomega.Equal(proto.String("gs://testbucket/testmodel")))
				g.Expect(*isvc.Spec.Predictor.Model.ProtocolVersion).To(gomega.Equal(constants.ProtocolV2))
			},
		},
		"DefaultWithSecurityDisabledServiceAccountToken": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "security-model",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			mutateFunc: func(isvc *InferenceService) *InferenceService {
				// Simulate a security config with AutoMountServiceAccountToken set to false
				isvc.DefaultInferenceService(nil, nil, &SecurityConfig{AutoMountServiceAccountToken: false}, nil)
				return isvc
			},
			verify: func(g *gomega.WithT, isvc *InferenceService) {
				g.Expect(isvc.Spec.Predictor.AutomountServiceAccountToken).NotTo(gomega.BeNil())
				g.Expect(*isvc.Spec.Predictor.AutomountServiceAccountToken).To(gomega.BeFalse())
			},
		},
		"DefaultWithModelMeshDeployment": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "modelmesh-model",
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
			},
			verify: func(g *gomega.WithT, isvc *InferenceService) {
				// For ModelMesh deployment, predictor shouldn't be converted to Model type
				g.Expect(isvc.Spec.Predictor.Tensorflow).NotTo(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model).To(gomega.BeNil())
			},
		},
		"DefaultWithRawDeploymentMode": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "raw-deployment-model",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ONNX: &ONNXRuntimeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/testmodel"),
							},
						},
					},
				},
			},
			mutateFunc: func(isvc *InferenceService) *InferenceService {
				// Simulate a DeployConfig with RawDeployment as default
				deployConfig := &DeployConfig{
					DefaultDeploymentMode: string(constants.RawDeployment),
				}
				isvc.DefaultInferenceService(nil, deployConfig, nil, nil)
				return isvc
			},
			verify: func(g *gomega.WithT, isvc *InferenceService) {
				g.Expect(isvc.ObjectMeta.Annotations).NotTo(gomega.BeNil())
				g.Expect(isvc.ObjectMeta.Annotations[constants.DeploymentMode]).To(gomega.Equal(string(constants.RawDeployment)))

				// Should still convert ONNX to Model
				g.Expect(isvc.Spec.Predictor.ONNX).To(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model).NotTo(gomega.BeNil())
				g.Expect(isvc.Spec.Predictor.Model.ModelFormat.Name).To(gomega.Equal(constants.SupportedModelONNX))
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			isvc := scenario.isvc.DeepCopy()

			// If there's a custom mutation function, use it
			if scenario.mutateFunc != nil {
				isvc = scenario.mutateFunc(isvc)
			} else {
				// Otherwise apply default settings
				isvc.DefaultInferenceService(nil, &DeployConfig{DefaultDeploymentMode: "Serverless"}, nil, nil)
			}

			// Verify the results
			scenario.verify(g, isvc)
		})
	}
}

func TestLocalModelLabelAssignment(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	gpu1, gpu2 := "gpu1", "gpu2"
	model1 := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model1",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://bucket/model",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}

	model2 := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model2",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://bucket/model2",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}

	localModels := &v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{*model1, *model2}}

	scenarios := map[string]struct {
		isvc         InferenceService
		expectMatch  bool
		matchedModel string
		nodeGroup    string
	}{
		"ModelMatches": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matching-model",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/model"),
							},
						},
					},
				},
			},
			expectMatch:  true,
			matchedModel: "model1",
			nodeGroup:    gpu1,
		},
		"ModelMatchesWithNodeGroupAnnotation": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matching-model-with-annotation",
					Namespace: "default",
					Annotations: map[string]string{
						constants.NodeGroupAnnotationKey: gpu2,
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/model"),
							},
						},
					},
				},
			},
			expectMatch:  true,
			matchedModel: "model1",
			nodeGroup:    gpu2,
		},
		"ModelDoesNotMatch": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-matching-model",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://bucket/non-matching-model"),
							},
						},
					},
				},
			},
			expectMatch: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			isvc := scenario.isvc.DeepCopy()

			// Apply defaults first (converts to Model)
			isvc.DefaultInferenceService(nil, &DeployConfig{DefaultDeploymentMode: "Serverless"}, nil, nil)

			// Set local model label
			isvc.setLocalModelLabel(localModels)

			if scenario.expectMatch {
				g.Expect(isvc.Labels).NotTo(gomega.BeNil())
				g.Expect(isvc.Labels[constants.LocalModelLabel]).To(gomega.Equal(scenario.matchedModel))
				g.Expect(isvc.Annotations).NotTo(gomega.BeNil())
				g.Expect(isvc.Annotations[constants.LocalModelSourceUriAnnotationKey]).To(gomega.Equal(model1.Spec.SourceModelUri))

				expectedPVC := scenario.matchedModel + "-" + scenario.nodeGroup
				g.Expect(isvc.Annotations[constants.LocalModelPVCNameAnnotationKey]).To(gomega.Equal(expectedPVC))
			} else {
				if isvc.Labels != nil {
					g.Expect(isvc.Labels[constants.LocalModelLabel]).To(gomega.BeEmpty())
				}
				if isvc.Annotations != nil {
					g.Expect(isvc.Annotations[constants.LocalModelSourceUriAnnotationKey]).To(gomega.BeEmpty())
					g.Expect(isvc.Annotations[constants.LocalModelPVCNameAnnotationKey]).To(gomega.BeEmpty())
				}
			}
		})
	}
}

func TestAssignHuggingFaceRuntime(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}

	scenarios := map[string]struct {
		config   *InferenceServicesConfig
		isvc     InferenceService
		matchers map[string]types.GomegaMatcher
	}{
		"HuggingFace without protocol version": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "huggingface-model",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						HuggingFace: &HuggingFaceRuntimeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/huggingface-model"),
							},
						},
					},
				},
			},
			matchers: map[string]types.GomegaMatcher{
				"modelFormat":     gomega.Equal(constants.SupportedModelHuggingFace),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"storageURI":      gomega.Equal("gs://testbucket/huggingface-model"),
				"huggingFaceSpec": gomega.BeNil(),
			},
		},
		"HuggingFace with explicit protocol version": {
			config: &InferenceServicesConfig{},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "huggingface-model-v1",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						HuggingFace: &HuggingFaceRuntimeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://testbucket/huggingface-model"),
								ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV1))),
							},
						},
					},
				},
			},
			matchers: map[string]types.GomegaMatcher{
				"modelFormat":     gomega.Equal(constants.SupportedModelHuggingFace),
				"protocolVersion": gomega.Equal(constants.ProtocolV1),
				"storageURI":      gomega.Equal("gs://testbucket/huggingface-model"),
				"huggingFaceSpec": gomega.BeNil(),
			},
		},
		"HuggingFace with resource config": {
			config: &InferenceServicesConfig{
				Resource: ResourceConfig{
					CPULimit:      "2",
					MemoryLimit:   "4Gi",
					CPURequest:    "1",
					MemoryRequest: "2Gi",
				},
			},
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "huggingface-with-resources",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						HuggingFace: &HuggingFaceRuntimeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://testbucket/huggingface-model"),
							},
						},
					},
				},
			},
			matchers: map[string]types.GomegaMatcher{
				"modelFormat":     gomega.Equal(constants.SupportedModelHuggingFace),
				"protocolVersion": gomega.Equal(constants.ProtocolV2),
				"storageURI":      gomega.Equal("gs://testbucket/huggingface-model"),
				"huggingFaceSpec": gomega.BeNil(),
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			isvc := scenario.isvc.DeepCopy()
			isvc.DefaultInferenceService(scenario.config, deployConfig, nil, nil)

			g.Expect(isvc.Spec.Predictor.Model).ToNot(gomega.BeNil())
			g.Expect(isvc.Spec.Predictor.HuggingFace).To(scenario.matchers["huggingFaceSpec"])
			g.Expect(isvc.Spec.Predictor.Model.ModelFormat.Name).To(scenario.matchers["modelFormat"])
			g.Expect(*isvc.Spec.Predictor.Model.ProtocolVersion).To(scenario.matchers["protocolVersion"])
			g.Expect(*isvc.Spec.Predictor.Model.StorageURI).To(scenario.matchers["storageURI"])
		})
	}
}

func TestDefaultInferenceServiceWithLocalModel(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	localModel := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-model",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://testbucket/testmodel",
			NodeGroups:     []string{"node-group-1"},
		},
	}
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{localModel},
	}

	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-isvc",
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
		},
	}

	isvc.DefaultInferenceService(nil, nil, nil, models)

	// Verify local model labels and annotations
	g.Expect(isvc.ObjectMeta.Labels).To(gomega.HaveKeyWithValue(constants.LocalModelLabel, "local-model"))
	g.Expect(isvc.ObjectMeta.Annotations).To(gomega.HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "gs://testbucket/testmodel"))
	g.Expect(isvc.ObjectMeta.Annotations).To(gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "local-model-node-group-1"))
}
