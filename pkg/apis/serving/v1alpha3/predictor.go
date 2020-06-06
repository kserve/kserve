package v1alpha3

import (
	v1 "k8s.io/api/core/v1"
)

// Predictor is an abstraction over machine learning server frameworks
// +kubebuilder:object:generate=false
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
	// Passthrough Pod fields or specify a custom container spec
	*CustomPredictor `json:",inline"`
}

// PredictorExtensionSpec defines configuration shared across all predictor frameworks
type PredictorExtensionSpec struct {
	// User must pick StorageURI or ConfigMap.
	// This field points to the location of the trained model which is mounted onto the pod.
	StorageURI *string `json:"storageUri"`

	// User must pick StorageURI or ConfigMap. Escape hatch to configure a model server.
	// Data in provided configmap must be compatible with the specified model server.
	// May be used for multi-model serving or low level traffic splitting.
	ConfigMap *string `json:"modelServerConfig"`

	// Enables multi-model serving with three separate modes: manual, auto, or isolated.
	// ModelServerConfig is not supported with TenancyKey.
	// If tenancyKey is not provided, the system will use "single tenant mode" or 1 model per pod.
	// If tenancyKey is specified, the system will co-locate all models using that key.
	// If tenancyKey is `auto`, the system will intelligently generate a tenancyKey. This may change over time.
	TenancyKey *string `json:"tenancyKey"`

	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	v1.Container `json:"inline"`
}

// GetPredictor returns the framework for the Predictor
func (i *InferenceService) GetPredictor() Predictor {
	for _, f := range []Predictor{
		i.Spec.Predictor.KFServer,
		i.Spec.Predictor.ONNXRuntime,
		i.Spec.Predictor.TFServing,
		i.Spec.Predictor.TorchServe,
		i.Spec.Predictor.Triton,
	} {
		if f != nil {
			return f
		}
	}
	return i.Spec.Predictor.CustomPredictor
}

// GetPredictorPodSpec returns the PodSpec for the Predictor
func (i *InferenceService) GetPredictorPodSpec() v1.PodSpec {
	p := i.Spec.Predictor.CustomPredictor.PodSpec
	p.Containers = i.GetPredictor().GetContainers()
	return p
}
