package v1beta1

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
)

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOnePredictorViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, Triton, SKLearn, XGBoost] must be specified in PredictorSpec"
)

// Predictor is an abstraction over machine learning server frameworks
// +kubebuilder:object:generate=false
type Predictor interface {
	GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container
	Validate() error
	Default(config *InferenceServicesConfig)
	GetStorageUri() *string
}

// PredictorSpec defines the configuration for a predictor,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type PredictorSpec struct {
	// Spec for SKLearn model server
	SKLearn *SKLearnSpec `json:"sklearn,omitempty"`
	// Spec for XGBoost model server
	XGBoost *XGBoostSpec `json:"xgboost,omitempty"`
	// Spec for TFServing (https://github.com/tensorflow/serving)
	Tensorflow *TensorflowSpec `json:"tensorflow,omitempty"`
	// Spec for TorchServe
	PyTorch *TorchServeSpec `json:"pytorch,omitempty"`
	// Spec for Triton Inference Server (https://github.com/NVIDIA/triton-inference-server)
	Triton *TritonSpec `json:"triton,omitempty"`
	// Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)
	ONNXRuntime *ONNXRuntimeSpec `json:"onnxruntime,omitempty"`
	// Passthrough Pod fields or specify a custom container spec
	*CustomPredictor `json:",inline"`
	// Extensions available in all components
	ComponentExtensionSpec `json:",inline"`
}

// PredictorExtensionSpec defines configuration shared across all predictor frameworks
type PredictorExtensionSpec struct {
	// This field points to the location of the trained model which is mounted onto the pod.
	StorageURI *string `json:"storageUri"`
	// Runtime version of the predictor docker image
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	// +optional
	v1.Container `json:",inline"`
}

// GetPredictor returns the framework for the Predictor
func (i *InferenceService) GetPredictor() (Predictor, error) {
	if i.Spec.Predictor.Tensorflow != nil {
		return i.Spec.Predictor.Tensorflow, nil
	}
	if i.Spec.Predictor.SKLearn != nil {
		return i.Spec.Predictor.SKLearn, nil
	}
	if i.Spec.Predictor.ONNXRuntime != nil {
		return i.Spec.Predictor.ONNXRuntime, nil
	}
	if i.Spec.Predictor.PyTorch != nil {
		return i.Spec.Predictor.PyTorch, nil
	}
	if i.Spec.Predictor.Triton != nil {
		return i.Spec.Predictor.Triton, nil
	}
	if i.Spec.Predictor.CustomPredictor != nil {
		return i.Spec.Predictor.CustomPredictor, nil
	}
	err := fmt.Errorf(ExactlyOnePredictorViolatedError)
	return nil, err
}

// GetPredictorPodSpec returns the PodSpec for the Predictor
func (i *InferenceService) GetPredictorPodSpec() v1.PodSpec {
	p := i.Spec.Predictor.CustomPredictor.Spec
	//p.Containers = i.GetPredictor().
	return p
}
