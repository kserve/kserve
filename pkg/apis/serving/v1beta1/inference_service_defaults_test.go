/*

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
	"github.com/golang/protobuf/proto"
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
		Defaults: IsvcDefaultConfig{
			Request: map[v1.ResourceName]resource.Quantity{
				"cpu":    resource.MustParse("1"),
				"memory": resource.MustParse("2Gi"),
			},
			Limit: map[v1.ResourceName]resource.Quantity{
				"cpu":    resource.MustParse("1"),
				"memory": resource.MustParse("2Gi"),
			},
		},
	}
	defaultResource := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
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
	g.Expect(*isvc.Spec.Predictor.Tensorflow.RuntimeVersion).To(gomega.Equal("1.14.0"))
	g.Expect(isvc.Spec.Predictor.Tensorflow.Resources).To(gomega.Equal(resources))

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
		Defaults: IsvcDefaultConfig{
			Request: map[v1.ResourceName]resource.Quantity{
				"cpu":    resource.MustParse("1"),
				"memory": resource.MustParse("2Gi"),
			},
			Limit: map[v1.ResourceName]resource.Quantity{
				"cpu":    resource.MustParse("1"),
				"memory": resource.MustParse("2Gi"),
			},
		},
	}
	defaultResource := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
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
