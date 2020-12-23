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

package v1alpha2

import (
	"github.com/gogo/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// Convert to hub version from v1alpha2 to v1beta1
func (src *InferenceService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.InferenceService)
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.Default.Predictor.Tensorflow != nil {
		dst.Spec.Predictor.Tensorflow = &v1beta1.TFServingSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.Tensorflow.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.Tensorflow.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.SKLearn != nil {
		dst.Spec.Predictor.SKLearn = &v1beta1.SKLearnSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.SKLearn.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.SKLearn.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.SKLearn.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.PMML != nil {
		dst.Spec.Predictor.PMML = &v1beta1.PMMLSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.PMML.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.PMML.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.PMML.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.XGBoost != nil {
		dst.Spec.Predictor.XGBoost = &v1beta1.XGBoostSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.XGBoost.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.XGBoost.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.XGBoost.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.LightGBM != nil {
		dst.Spec.Predictor.LightGBM = &v1beta1.LightGBMSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.LightGBM.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.LightGBM.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.LightGBM.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.Triton != nil {
		dst.Spec.Predictor.Triton = &v1beta1.TritonSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.Triton.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.Triton.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.Triton.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.ONNX != nil {
		dst.Spec.Predictor.ONNX = &v1beta1.ONNXRuntimeSpec{
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.ONNX.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.ONNX.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.ONNX.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.PyTorch != nil {
		dst.Spec.Predictor.PyTorch = &v1beta1.TorchServeSpec{
			ModelClassName: src.Spec.Default.Predictor.PyTorch.ModelClassName,
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.PyTorch.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.PyTorch.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.PyTorch.Resources,
				},
			},
		}
	} else if src.Spec.Default.Predictor.Custom != nil {
		dst.Spec.Predictor.PodSpec = v1beta1.PodSpec{
			Containers: []v1.Container{
				src.Spec.Default.Predictor.Custom.Container,
			},
		}
	}
	dst.Spec.Predictor.MinReplicas = src.Spec.Default.Predictor.MinReplicas
	dst.Spec.Predictor.MaxReplicas = src.Spec.Default.Predictor.MaxReplicas
	dst.Spec.Predictor.ContainerConcurrency = proto.Int64(int64(src.Spec.Default.Predictor.Parallelism))
	if src.Spec.CanaryTrafficPercent != nil {
		dst.Spec.Predictor.CanaryTrafficPercent = proto.Int64(int64(*src.Spec.CanaryTrafficPercent))
	}
	if src.Spec.Default.Predictor.Batcher != nil {
		dst.Spec.Predictor.Batcher = &v1beta1.Batcher{
			MaxBatchSize: src.Spec.Default.Predictor.Batcher.MaxBatchSize,
			MaxLatency:   src.Spec.Default.Predictor.Batcher.MaxLatency,
			Timeout:      src.Spec.Default.Predictor.Batcher.Timeout,
		}
	}
	if src.Spec.Default.Predictor.Logger != nil {
		dst.Spec.Predictor.Logger = &v1beta1.LoggerSpec{
			URL:  src.Spec.Default.Predictor.Logger.Url,
			Mode: v1beta1.LoggerType(src.Spec.Default.Predictor.Logger.Mode),
		}
	}
	if src.Spec.Default.Predictor.ServiceAccountName != "" {
		dst.Spec.Predictor.PodSpec.ServiceAccountName = src.Spec.Default.Predictor.ServiceAccountName
	}

	if src.Spec.Default.Transformer != nil {
		if src.Spec.Default.Transformer.Custom != nil {
			dst.Spec.Transformer = &v1beta1.TransformerSpec{
				ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
					MinReplicas:          src.Spec.Default.Transformer.MinReplicas,
					MaxReplicas:          src.Spec.Default.Transformer.MaxReplicas,
					ContainerConcurrency: proto.Int64(int64(src.Spec.Default.Transformer.Parallelism)),
				},
				PodSpec: v1beta1.PodSpec{
					Containers: []v1.Container{
						src.Spec.Default.Transformer.Custom.Container,
					},
				},
			}
		}
	}
	if src.Spec.Default.Explainer != nil {
		if src.Spec.Default.Explainer.Alibi != nil {
			dst.Spec.Explainer = &v1beta1.ExplainerSpec{
				ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
					MinReplicas:          src.Spec.Default.Explainer.MinReplicas,
					MaxReplicas:          src.Spec.Default.Explainer.MaxReplicas,
					ContainerConcurrency: proto.Int64(int64(src.Spec.Default.Explainer.Parallelism)),
				},
				Alibi: &v1beta1.AlibiExplainerSpec{
					Type:           v1beta1.AlibiExplainerType(src.Spec.Default.Explainer.Alibi.Type),
					StorageURI:     src.Spec.Default.Explainer.Alibi.StorageURI,
					RuntimeVersion: proto.String(src.Spec.Default.Explainer.Alibi.RuntimeVersion),
					Container: v1.Container{
						Resources: src.Spec.Default.Explainer.Alibi.Resources,
					},
				},
			}
		}
		if src.Spec.Default.Explainer.AIX != nil {
			dst.Spec.Explainer = &v1beta1.ExplainerSpec{
				ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
					MinReplicas:          src.Spec.Default.Explainer.MinReplicas,
					MaxReplicas:          src.Spec.Default.Explainer.MaxReplicas,
					ContainerConcurrency: proto.Int64(int64(src.Spec.Default.Explainer.Parallelism)),
				},
				AIX: &v1beta1.AIXExplainerSpec{
					Type:           v1beta1.AIXExplainerType(src.Spec.Default.Explainer.AIX.Type),
					StorageURI:     src.Spec.Default.Explainer.AIX.StorageURI,
					RuntimeVersion: proto.String(src.Spec.Default.Explainer.AIX.RuntimeVersion),
					Container: v1.Container{
						Resources: src.Spec.Default.Explainer.AIX.Resources,
					},
				},
			}
		}
		if src.Spec.Default.Explainer.Custom != nil {
			dst.Spec.Explainer = &v1beta1.ExplainerSpec{
				PodSpec: v1beta1.PodSpec{
					Containers: []v1.Container{
						src.Spec.Default.Explainer.Custom.Container,
					},
				},
			}
		}
	}
	return nil
}

// Convert from hub version v1beta1 to v1alpha2
func (dst *InferenceService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.InferenceService)
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.Predictor.Tensorflow != nil {
		dst.Spec.Default.Predictor.Tensorflow = &TensorflowSpec{
			RuntimeVersion: *src.Spec.Predictor.Tensorflow.RuntimeVersion,
			Resources:      src.Spec.Predictor.Tensorflow.Resources,
		}
		if src.Spec.Predictor.Tensorflow.StorageURI != nil {
			dst.Spec.Default.Predictor.Tensorflow.StorageURI = *src.Spec.Predictor.Tensorflow.StorageURI
		}
	} else if src.Spec.Predictor.SKLearn != nil {
		dst.Spec.Default.Predictor.SKLearn = &SKLearnSpec{
			RuntimeVersion: *src.Spec.Predictor.SKLearn.RuntimeVersion,
			Resources:      src.Spec.Predictor.SKLearn.Resources,
		}
		if src.Spec.Predictor.SKLearn.StorageURI != nil {
			dst.Spec.Default.Predictor.SKLearn.StorageURI = *src.Spec.Predictor.SKLearn.StorageURI
		}
	} else if src.Spec.Predictor.PMML != nil {
		dst.Spec.Default.Predictor.PMML = &PMMLSpec{
			RuntimeVersion: *src.Spec.Predictor.PMML.RuntimeVersion,
			Resources:      src.Spec.Predictor.PMML.Resources,
		}
		if src.Spec.Predictor.PMML.StorageURI != nil {
			dst.Spec.Default.Predictor.PMML.StorageURI = *src.Spec.Predictor.PMML.StorageURI
		}
	} else if src.Spec.Predictor.XGBoost != nil {
		dst.Spec.Default.Predictor.XGBoost = &XGBoostSpec{
			RuntimeVersion: *src.Spec.Predictor.XGBoost.RuntimeVersion,
			Resources:      src.Spec.Predictor.XGBoost.Resources,
		}
		if src.Spec.Predictor.XGBoost.StorageURI != nil {
			dst.Spec.Default.Predictor.XGBoost.StorageURI = *src.Spec.Predictor.XGBoost.StorageURI
		}
	} else if src.Spec.Predictor.LightGBM != nil {
		dst.Spec.Default.Predictor.LightGBM = &LightGBMSpec{
			RuntimeVersion: *src.Spec.Predictor.LightGBM.RuntimeVersion,
			Resources:      src.Spec.Predictor.LightGBM.Resources,
		}
		if src.Spec.Predictor.LightGBM.StorageURI != nil {
			dst.Spec.Default.Predictor.LightGBM.StorageURI = *src.Spec.Predictor.LightGBM.StorageURI
		}
	} else if src.Spec.Predictor.Triton != nil {
		dst.Spec.Default.Predictor.Triton = &TritonSpec{
			RuntimeVersion: *src.Spec.Predictor.Triton.RuntimeVersion,
			Resources:      src.Spec.Predictor.Triton.Resources,
		}
		if src.Spec.Predictor.Triton.StorageURI != nil {
			dst.Spec.Default.Predictor.Triton.StorageURI = *src.Spec.Predictor.Triton.StorageURI
		}
	} else if src.Spec.Predictor.ONNX != nil {
		dst.Spec.Default.Predictor.ONNX = &ONNXSpec{
			RuntimeVersion: *src.Spec.Predictor.ONNX.RuntimeVersion,
			Resources:      src.Spec.Predictor.ONNX.Resources,
		}
		if src.Spec.Predictor.ONNX.StorageURI != nil {
			dst.Spec.Default.Predictor.ONNX.StorageURI = *src.Spec.Predictor.ONNX.StorageURI
		}
	} else if src.Spec.Predictor.PyTorch != nil {
		dst.Spec.Default.Predictor.PyTorch = &PyTorchSpec{
			RuntimeVersion: *src.Spec.Predictor.PyTorch.RuntimeVersion,
			Resources:      src.Spec.Predictor.PyTorch.Resources,
		}
		if src.Spec.Predictor.PyTorch.StorageURI != nil {
			dst.Spec.Default.Predictor.PyTorch.StorageURI = *src.Spec.Predictor.PyTorch.StorageURI
		}
	} else if len(src.Spec.Predictor.PodSpec.Containers) != 0 {
		dst.Spec.Default.Predictor.ServiceAccountName = src.Spec.Predictor.PodSpec.ServiceAccountName
		dst.Spec.Default.Predictor.Custom = &CustomSpec{
			src.Spec.Predictor.PodSpec.Containers[0],
		}
	}

	dst.Spec.Default.Predictor.MinReplicas = src.Spec.Predictor.MinReplicas
	dst.Spec.Default.Predictor.MaxReplicas = src.Spec.Predictor.MaxReplicas
	if src.Spec.Predictor.ContainerConcurrency != nil {
		dst.Spec.Default.Predictor.Parallelism = int(*src.Spec.Predictor.ContainerConcurrency)
	}
	if src.Spec.Predictor.CanaryTrafficPercent != nil {
		dst.Spec.CanaryTrafficPercent = GetIntReference(int(*src.Spec.Predictor.CanaryTrafficPercent))
	}
	if src.Spec.Predictor.Batcher != nil {
		dst.Spec.Default.Predictor.Batcher = &Batcher{
			MaxBatchSize: src.Spec.Predictor.Batcher.MaxBatchSize,
			MaxLatency:   src.Spec.Predictor.Batcher.MaxLatency,
			Timeout:      src.Spec.Predictor.Batcher.Timeout,
		}
	}
	if src.Spec.Predictor.Logger != nil {
		dst.Spec.Default.Predictor.Logger = &Logger{
			Url:  src.Spec.Predictor.Logger.URL,
			Mode: LoggerMode(src.Spec.Predictor.Logger.Mode),
		}
	}
	//Transformer
	if src.Spec.Transformer != nil {
		if len(src.Spec.Transformer.PodSpec.Containers) != 0 {
			dst.Spec.Default.Transformer = &TransformerSpec{
				Custom: &CustomSpec{
					Container: src.Spec.Transformer.PodSpec.Containers[0],
				},
			}
		}
		dst.Spec.Default.Transformer.MinReplicas = src.Spec.Transformer.MinReplicas
		dst.Spec.Default.Transformer.MaxReplicas = src.Spec.Transformer.MaxReplicas
		dst.Spec.Default.Transformer.ServiceAccountName = src.Spec.Transformer.PodSpec.ServiceAccountName
		if src.Spec.Transformer.ContainerConcurrency != nil {
			dst.Spec.Default.Transformer.Parallelism = int(*src.Spec.Transformer.ContainerConcurrency)
		}
	}
	//Explainer
	if src.Spec.Explainer != nil {
		if src.Spec.Explainer.Alibi != nil {
			dst.Spec.Default.Explainer = &ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type:           AlibiExplainerType(src.Spec.Explainer.Alibi.Type),
					StorageURI:     src.Spec.Explainer.Alibi.StorageURI,
					RuntimeVersion: *src.Spec.Explainer.Alibi.RuntimeVersion,
					Resources:      src.Spec.Explainer.Alibi.Resources,
				},
			}
		} else if src.Spec.Explainer.AIX != nil {
			dst.Spec.Default.Explainer = &ExplainerSpec{
				AIX: &AIXExplainerSpec{
					Type:           AIXExplainerType(src.Spec.Explainer.AIX.Type),
					StorageURI:     src.Spec.Explainer.AIX.StorageURI,
					RuntimeVersion: *src.Spec.Explainer.AIX.RuntimeVersion,
					Resources:      src.Spec.Explainer.AIX.Resources,
				},
			}
		} else if len(src.Spec.Explainer.PodSpec.Containers) != 0 {
			dst.Spec.Default.Explainer = &ExplainerSpec{
				Custom: &CustomSpec{
					Container: src.Spec.Explainer.PodSpec.Containers[0],
				},
			}
		}
		dst.Spec.Default.Explainer.ServiceAccountName = src.Spec.Explainer.PodSpec.ServiceAccountName
		dst.Spec.Default.Explainer.MinReplicas = src.Spec.Explainer.MinReplicas
		dst.Spec.Default.Explainer.MaxReplicas = src.Spec.Explainer.MaxReplicas
		if src.Spec.Explainer.ContainerConcurrency != nil {
			dst.Spec.Default.Explainer.Parallelism = int(*src.Spec.Explainer.ContainerConcurrency)
		}
	}
	return nil
}
