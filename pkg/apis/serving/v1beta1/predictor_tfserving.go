package v1beta1

import v1 "k8s.io/api/core/v1"

// TFServingSpec defines arguments for configuring Tensorflow model serving.
type TFServingSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TFServingSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TFServingSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (t *TFServingSpec) GetContainers() []v1.Container {
	return []v1.Container{}
}
