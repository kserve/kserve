package v1alpha3

import (
	v1 "k8s.io/api/core/v1"
)

// Predictor is an abstraction over machine learning server frameworks
type Predictor interface {
	GetContainers() []v1.Container
	Validate() error
	Default()
}

// PredictorSpec defines the configuration for a predictor,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type PredictorSpec struct {
	// Spec for KFServer
	KFServer *KFServerSpec `json:"kfserver,omitempty"`
	// Spec for TFServing (https://github.com/tensorflow/serving)
	TFServing *TFServingSpec `json:"tfserving,omitempty"`
	// Spec for PyTorch predictor
	TorchServe *TorchServeSpec `json:"torchserve,omitempty"`
	// Spec for Triton Inference Server (https://github.com/NVIDIA/triton-inference-server)
	Triton *TritonSpec `json:"triton,omitempty"`
	// Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)
	ONNXRuntime *ONNXRuntimeSpec `json:"onnxruntime,omitempty"`
	// Passthrough to underlying Pods
	*CustomPredictor `json:",inline"`
}

// PredictorExtensionSpec defines configuration shared across all predictor frameworks
type PredictorExtensionSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Container enables overrides for the predictor. Each framework will have different defaults.
	v1.Container `json:"inline"`
}

// GetPredictor returns the framework for the Predictor
func (s *Service) GetPredictor() Predictor {
	for _, f := range []Predictor{
		s.Spec.Predictor.KFServer,
		s.Spec.Predictor.ONNXRuntime,
		s.Spec.Predictor.TFServing,
		s.Spec.Predictor.TorchServe,
		s.Spec.Predictor.Triton,
	} {
		if f != nil {
			return f
		}
	}
	return s.Spec.Predictor.CustomPredictor
}

// GetPredictorPodSpec returns the PodSpec for the Predictor
func (s *Service) GetPredictorPodSpec() v1.PodTemplateSpec {
	p := s.Spec.Predictor.CustomPredictor.PodTemplateSpec
	p.Spec.Containers = s.GetPredictor().GetContainers()
	return p
}
