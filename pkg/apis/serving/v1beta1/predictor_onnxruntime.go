package v1beta1

import v1 "k8s.io/api/core/v1"

// ONNXRuntimeSpec defines arguments for configuring ONNX model serving.
type ONNXRuntimeSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (o *ONNXRuntimeSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (o *ONNXRuntimeSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (o *ONNXRuntimeSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	return &v1.Container{
	}
}
