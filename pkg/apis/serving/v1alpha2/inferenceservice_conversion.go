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
			PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
				RuntimeVersion: &src.Spec.Default.Predictor.PyTorch.RuntimeVersion,
				StorageURI:     &src.Spec.Default.Predictor.PyTorch.StorageURI,
				Container: v1.Container{
					Resources: src.Spec.Default.Predictor.PyTorch.Resources,
				},
			},
		}
	}
	dst.Spec.Predictor.MinReplicas = src.Spec.Default.Predictor.MinReplicas
	dst.Spec.Predictor.MaxReplicas = src.Spec.Default.Predictor.MaxReplicas
	dst.Spec.Predictor.ContainerConcurrency = proto.Int64(int64(src.Spec.Default.Predictor.Parallelism))
	if src.Spec.Default.Predictor.ServiceAccountName != "" {
		dst.Spec.Predictor.Spec = v1.PodSpec{}
		dst.Spec.Predictor.Spec.ServiceAccountName = src.Spec.Default.Predictor.ServiceAccountName
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
		if src.Spec.Predictor.PyTorch.StorageURI != nil {
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
	} else if src.Spec.Predictor.XGBoost != nil {
		dst.Spec.Default.Predictor.XGBoost = &XGBoostSpec{
			RuntimeVersion: *src.Spec.Predictor.XGBoost.RuntimeVersion,
			Resources:      src.Spec.Predictor.XGBoost.Resources,
		}
		if src.Spec.Predictor.XGBoost.StorageURI != nil {
			dst.Spec.Default.Predictor.XGBoost.StorageURI = *src.Spec.Predictor.XGBoost.StorageURI
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
	}
	dst.Spec.Default.Predictor.MinReplicas = src.Spec.Predictor.MinReplicas
	dst.Spec.Default.Predictor.MaxReplicas = src.Spec.Predictor.MaxReplicas
	if src.Spec.Predictor.ContainerConcurrency != nil {
		dst.Spec.Default.Predictor.Parallelism = int(*src.Spec.Predictor.ContainerConcurrency)
	}
	if src.Spec.Predictor.CustomPredictor != nil {
		dst.Spec.Default.Predictor.ServiceAccountName = src.Spec.Predictor.CustomPredictor.Spec.ServiceAccountName
	}
	return nil
}
