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
}

func TestCustomPredictorDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}
	config := &InferenceServicesConfig{
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
	resources := corev1.ResourceRequirements{Requests: defaultResource, Limits: defaultResource}
	isvc.Spec.DeepCopy()
	isvc.DefaultInferenceService(config, deployConfig, nil, nil)
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
	localModelName := "iris"
	gpu1, gpu2 := "gpu1", "gpu2"
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
								StorageURI:      proto.String("gs://testbucket/testmodel"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			labelMatcher:      gomega.HaveKeyWithValue(constants.LocalModelLabel, localModelName),
			annotationMatcher: gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, localModelName+"-"+gpu1),
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
								StorageURI:      proto.String("gs://testbucket/testmodel"),
								ProtocolVersion: &protocolVersion,
							},
						},
					},
				},
			},
			labelMatcher:      gomega.HaveKeyWithValue(constants.LocalModelLabel, localModelName),
			annotationMatcher: gomega.HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, localModelName+"-"+gpu2),
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
								StorageURI: proto.String("gs://testbucket2/testmodel"),
							},
						},
					},
				},
			},
			labelMatcher:      gomega.Not(gomega.HaveKeyWithValue(constants.LocalModelLabel, localModelName)),
			annotationMatcher: gomega.Not(gomega.HaveKey(constants.LocalModelPVCNameAnnotationKey)),
		},
	}
	localModel := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: localModelName,
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://testbucket/testmodel",
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}
	localModels := &v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{*localModel}}
	for _, scenario := range scenarios {
		scenario.isvc.DefaultInferenceService(scenario.config, deployConfig, nil, localModels)
		g.Expect(scenario.isvc.ObjectMeta.Labels).To(scenario.labelMatcher)
		g.Expect(scenario.isvc.ObjectMeta.Annotations).To(scenario.annotationMatcher)
	}
}
