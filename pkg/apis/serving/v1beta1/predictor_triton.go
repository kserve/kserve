package v1beta1

import v1 "k8s.io/api/core/v1"

// TritonSpec defines arguments for configuring Triton model serving.
type TritonSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TritonSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TritonSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (t *TritonSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	return &v1.Container{}
}
