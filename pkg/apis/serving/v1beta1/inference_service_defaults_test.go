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

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInferenceServiceDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Tensorflow: PredictorConfig{
				ContainerImage:         "tfserving",
				DefaultImageVersion:    "1.14.0",
				DefaultGpuImageVersion: "1.14.0-gpu",
				MultiModelServer:       false,
			},
		},
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
	resources := v1.ResourceRequirements{Requests: defaultResource, Limits: defaultResource}
	isvc.Spec.DeepCopy()
	isvc.DefaultInferenceService(config)
	g.Expect(*&isvc.Spec.Predictor.Tensorflow).To(gomega.BeNil())
	g.Expect(*&isvc.Spec.Predictor.Model).NotTo(gomega.BeNil())

	g.Expect(isvc.Spec.Transformer.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))

	g.Expect(*isvc.Spec.Explainer.Alibi.RuntimeVersion).To(gomega.Equal("v0.4.0"))
	g.Expect(isvc.Spec.Explainer.Alibi.Resources).To(gomega.Equal(resources))
}

func TestCustomPredictorDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Tensorflow: PredictorConfig{
				ContainerImage:         "tfserving",
				DefaultImageVersion:    "1.14.0",
				DefaultGpuImageVersion: "1.14.0-gpu",
				MultiModelServer:       false,
			},
		},
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
	isvc.DefaultInferenceService(config)
	g.Expect(isvc.Spec.Predictor.PodSpec.Containers[0].Resources).To(gomega.Equal(resources))
}

func TestInferenceServiceDefaultsModelMeshAnnotation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := &InferenceServicesConfig{}
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
	isvc.DefaultInferenceService(config)
	g.Expect(isvc.Spec.Predictor.Model).To(gomega.BeNil())
	g.Expect(isvc.Spec.Predictor.Tensorflow).ToNot(gomega.BeNil())
}
