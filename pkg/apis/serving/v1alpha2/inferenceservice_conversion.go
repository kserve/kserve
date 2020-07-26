package v1alpha2

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// Convert to hub version v1beta1
func (src *InferenceService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.InferenceService)
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.Default.Predictor.Tensorflow != nil {
		dst.Spec.Predictor.Tensorflow.RuntimeVersion = src.Spec.Default.Predictor.Tensorflow.RuntimeVersion
		dst.Spec.Predictor.Tensorflow.StorageURI = &src.Spec.Default.Predictor.Tensorflow.StorageURI
		dst.Spec.Predictor.Tensorflow.Resources = src.Spec.Default.Predictor.Tensorflow.Resources
	} else if src.Spec.Default.Predictor.SKLearn != nil {
		dst.Spec.Predictor.SKLearn.RuntimeVersion = src.Spec.Default.Predictor.SKLearn.RuntimeVersion
		dst.Spec.Predictor.SKLearn.StorageURI = &src.Spec.Default.Predictor.SKLearn.StorageURI
		dst.Spec.Predictor.SKLearn.Resources = src.Spec.Default.Predictor.SKLearn.Resources
	} else if src.Spec.Default.Predictor.XGBoost != nil {
		dst.Spec.Predictor.XGBoost.RuntimeVersion = src.Spec.Default.Predictor.XGBoost.RuntimeVersion
		dst.Spec.Predictor.XGBoost.StorageURI = &src.Spec.Default.Predictor.XGBoost.StorageURI
		dst.Spec.Predictor.XGBoost.Resources = src.Spec.Default.Predictor.XGBoost.Resources
	} else if src.Spec.Default.Predictor.Triton != nil {
		dst.Spec.Predictor.Triton.RuntimeVersion = src.Spec.Default.Predictor.Triton.RuntimeVersion
		dst.Spec.Predictor.Triton.StorageURI = &src.Spec.Default.Predictor.Triton.StorageURI
		dst.Spec.Predictor.Triton.Resources = src.Spec.Default.Predictor.Triton.Resources
	} else if src.Spec.Default.Predictor.ONNX != nil {
		dst.Spec.Predictor.ONNXRuntime.RuntimeVersion = src.Spec.Default.Predictor.ONNX.RuntimeVersion
		dst.Spec.Predictor.ONNXRuntime.StorageURI = &src.Spec.Default.Predictor.ONNX.StorageURI
		dst.Spec.Predictor.ONNXRuntime.Resources = src.Spec.Default.Predictor.ONNX.Resources
	} else if src.Spec.Default.Predictor.PyTorch != nil {
		dst.Spec.Predictor.PyTorch.RuntimeVersion = src.Spec.Default.Predictor.PyTorch.RuntimeVersion
		dst.Spec.Predictor.PyTorch.StorageURI = &src.Spec.Default.Predictor.PyTorch.StorageURI
		dst.Spec.Predictor.PyTorch.Resources = src.Spec.Default.Predictor.PyTorch.Resources
	}
	dst.Spec.Predictor.MinReplicas = src.Spec.Default.Predictor.MinReplicas
	dst.Spec.Predictor.MaxReplicas = src.Spec.Default.Predictor.MaxReplicas
	dst.Spec.Predictor.ContainerConcurrency = src.Spec.Default.Predictor.Parallelism
	dst.Spec.Predictor.Spec.ServiceAccountName = src.Spec.Default.Predictor.ServiceAccountName
	return nil
}

// Convert from hub version
func (dst *InferenceService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.InferenceService)
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.Predictor.Tensorflow != nil {
		dst.Spec.Default.Predictor.Tensorflow.RuntimeVersion = src.Spec.Predictor.Tensorflow.RuntimeVersion
		if src.Spec.Predictor.PyTorch.StorageURI != nil {
			dst.Spec.Default.Predictor.Tensorflow.StorageURI = *src.Spec.Predictor.Tensorflow.StorageURI
		}
		dst.Spec.Default.Predictor.Tensorflow.Resources = src.Spec.Predictor.Tensorflow.Resources
	} else if src.Spec.Predictor.SKLearn != nil {
		dst.Spec.Default.Predictor.SKLearn.RuntimeVersion = src.Spec.Predictor.SKLearn.RuntimeVersion
		if src.Spec.Predictor.SKLearn.StorageURI != nil {
			dst.Spec.Default.Predictor.SKLearn.StorageURI = *src.Spec.Predictor.SKLearn.StorageURI
		}
		dst.Spec.Default.Predictor.SKLearn.Resources = src.Spec.Predictor.SKLearn.Resources
	} else if src.Spec.Predictor.XGBoost != nil {
		dst.Spec.Default.Predictor.XGBoost.RuntimeVersion = src.Spec.Predictor.XGBoost.RuntimeVersion
		if src.Spec.Predictor.XGBoost.StorageURI != nil {
			dst.Spec.Default.Predictor.XGBoost.StorageURI = *src.Spec.Predictor.XGBoost.StorageURI
		}
		dst.Spec.Default.Predictor.XGBoost.Resources = src.Spec.Predictor.XGBoost.Resources
	} else if src.Spec.Predictor.Triton != nil {
		dst.Spec.Default.Predictor.Triton.RuntimeVersion = src.Spec.Predictor.Triton.RuntimeVersion
		if src.Spec.Predictor.Triton.StorageURI != nil {
			dst.Spec.Default.Predictor.Triton.StorageURI = *src.Spec.Predictor.Triton.StorageURI
		}
		dst.Spec.Default.Predictor.Triton.Resources = src.Spec.Predictor.Triton.Resources
	} else if src.Spec.Predictor.ONNXRuntime != nil {
		dst.Spec.Default.Predictor.ONNX.RuntimeVersion = src.Spec.Predictor.ONNXRuntime.RuntimeVersion
		if src.Spec.Predictor.ONNXRuntime.StorageURI != nil {
			dst.Spec.Default.Predictor.ONNX.StorageURI = *src.Spec.Predictor.ONNXRuntime.StorageURI
		}
		dst.Spec.Default.Predictor.ONNX.Resources = src.Spec.Predictor.ONNXRuntime.Resources
	} else if src.Spec.Predictor.PyTorch != nil {
		dst.Spec.Default.Predictor.PyTorch.RuntimeVersion = src.Spec.Predictor.PyTorch.RuntimeVersion
		if src.Spec.Predictor.PyTorch.StorageURI != nil {
			dst.Spec.Default.Predictor.PyTorch.StorageURI = *src.Spec.Predictor.PyTorch.StorageURI
		}
		dst.Spec.Default.Predictor.PyTorch.Resources = src.Spec.Predictor.PyTorch.Resources
	}
	dst.Spec.Default.Predictor.MinReplicas = src.Spec.Predictor.MinReplicas
	dst.Spec.Default.Predictor.MaxReplicas = src.Spec.Predictor.MaxReplicas
	dst.Spec.Default.Predictor.Parallelism = src.Spec.Predictor.ContainerConcurrency
	dst.Spec.Default.Predictor.ServiceAccountName = src.Spec.Predictor.Spec.ServiceAccountName
	return nil
}
