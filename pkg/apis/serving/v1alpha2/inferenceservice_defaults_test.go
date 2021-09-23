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

package v1alpha2

import (
	"testing"

	"github.com/kserve/kserve/pkg/constants"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTensorflowDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					Tensorflow: &TensorflowSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.Tensorflow.RuntimeVersion = "1.11"
	isvc.Spec.Canary.Predictor.Tensorflow.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion).To(gomega.Equal(DefaultTensorflowRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Canary.Predictor.Tensorflow.RuntimeVersion).To(gomega.Equal("1.11"))
	g.Expect(isvc.Spec.Canary.Predictor.Tensorflow.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.Tensorflow.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}

func TestTensorflowGPUDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					Tensorflow: &TensorflowSpec{
						StorageURI: "gs://testbucket/testmodel",
						Resources: v1.ResourceRequirements{
							Limits: map[v1.ResourceName]resource.Quantity{
								constants.NvidiaGPUResourceType: resource.MustParse("1"),
							},
						},
					},
				},
			},
		},
	}
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion).To(gomega.Equal(DefaultTensorflowRuntimeVersionGPU))
}

func TestPyTorchDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					PyTorch: &PyTorchSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.PyTorch.RuntimeVersion = constants.KServeDefaultVersion
	isvc.Spec.Canary.Predictor.PyTorch.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)
	g.Expect(isvc.Spec.Default.Predictor.PyTorch.RuntimeVersion).To(gomega.Equal(DefaultPyTorchRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.PyTorch.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.PyTorch.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.PyTorch.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.PyTorch.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Canary.Predictor.PyTorch.RuntimeVersion).To(gomega.Equal(constants.KServeDefaultVersion))
	g.Expect(isvc.Spec.Canary.Predictor.PyTorch.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.PyTorch.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}

func TestSKLearnDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					SKLearn: &SKLearnSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.SKLearn.RuntimeVersion = constants.KServeDefaultVersion
	isvc.Spec.Canary.Predictor.SKLearn.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)

	g.Expect(isvc.Spec.Default.Predictor.SKLearn.RuntimeVersion).To(gomega.Equal(DefaultSKLearnRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.SKLearn.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.SKLearn.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.SKLearn.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.SKLearn.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Canary.Predictor.SKLearn.RuntimeVersion).To(gomega.Equal(constants.KServeDefaultVersion))
	g.Expect(isvc.Spec.Canary.Predictor.SKLearn.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.SKLearn.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}

func TestXGBoostDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					XGBoost: &XGBoostSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.XGBoost.RuntimeVersion = constants.KServeDefaultVersion
	isvc.Spec.Canary.Predictor.XGBoost.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)

	g.Expect(isvc.Spec.Default.Predictor.XGBoost.RuntimeVersion).To(gomega.Equal(DefaultXGBoostRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.XGBoost.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.XGBoost.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.XGBoost.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.XGBoost.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))

	defaultCpu := defaultResource[v1.ResourceCPU]
	g.Expect(isvc.Spec.Default.Predictor.XGBoost.NThread).To(gomega.Equal(int(defaultCpu.Value())))
	g.Expect(isvc.Spec.Canary.Predictor.XGBoost.RuntimeVersion).To(gomega.Equal(constants.KServeDefaultVersion))
	g.Expect(isvc.Spec.Canary.Predictor.XGBoost.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.XGBoost.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}

func TestLightGBMDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					LightGBM: &LightGBMSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.LightGBM.RuntimeVersion = constants.KServeDefaultVersion
	isvc.Spec.Canary.Predictor.LightGBM.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)

	g.Expect(isvc.Spec.Default.Predictor.LightGBM.RuntimeVersion).To(gomega.Equal(DefaultLightGBMRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.LightGBM.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.LightGBM.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.LightGBM.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.LightGBM.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))

	defaultCpu := defaultResource[v1.ResourceCPU]
	g.Expect(isvc.Spec.Default.Predictor.LightGBM.NThread).To(gomega.Equal(int(defaultCpu.Value())))
	g.Expect(isvc.Spec.Canary.Predictor.LightGBM.RuntimeVersion).To(gomega.Equal(constants.KServeDefaultVersion))
	g.Expect(isvc.Spec.Canary.Predictor.LightGBM.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.LightGBM.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}
func TestONNXDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					ONNX: &ONNXSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.ONNX.RuntimeVersion = "0.6.0"
	isvc.Spec.Canary.Predictor.ONNX.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)

	g.Expect(isvc.Spec.Default.Predictor.ONNX.RuntimeVersion).To(gomega.Equal(DefaultONNXRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.ONNX.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.ONNX.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.ONNX.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.ONNX.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Canary.Predictor.ONNX.RuntimeVersion).To(gomega.Equal("0.6.0"))
	g.Expect(isvc.Spec.Canary.Predictor.ONNX.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.ONNX.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}

func TestTritonISDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					Triton: &TritonSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.Triton.RuntimeVersion = "19.09"
	isvc.Spec.Canary.Predictor.Triton.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)

	g.Expect(isvc.Spec.Default.Predictor.Triton.RuntimeVersion).To(gomega.Equal(DefaultTritonISRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.Triton.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.Triton.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Predictor.Triton.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Predictor.Triton.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Canary.Predictor.Triton.RuntimeVersion).To(gomega.Equal("19.09"))
	g.Expect(isvc.Spec.Canary.Predictor.Triton.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Predictor.Triton.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}

func TestAlibiExplainerDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					Tensorflow: &TensorflowSpec{
						StorageURI: "gs://testbucket/testmodel",
					},
				},
				Explainer: &ExplainerSpec{
					Alibi: &AlibiExplainerSpec{
						StorageURI: "gs://testbucket/testmodel",
					},
				},
			},
		},
	}
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Explainer.Alibi.RuntimeVersion = "0.2.4"
	isvc.Spec.Canary.Explainer.Alibi.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.applyDefaultsEndpoint(isvc.Spec.Canary, c)
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)

	g.Expect(isvc.Spec.Default.Explainer.Alibi.RuntimeVersion).To(gomega.Equal(DefaultAlibiExplainerRuntimeVersion))
	g.Expect(isvc.Spec.Default.Explainer.Alibi.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Explainer.Alibi.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Default.Explainer.Alibi.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Default.Explainer.Alibi.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(defaultResource[v1.ResourceMemory]))
	g.Expect(isvc.Spec.Canary.Explainer.Alibi.RuntimeVersion).To(gomega.Equal("0.2.4"))
	g.Expect(isvc.Spec.Canary.Explainer.Alibi.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(defaultResource[v1.ResourceCPU]))
	g.Expect(isvc.Spec.Canary.Explainer.Alibi.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}
