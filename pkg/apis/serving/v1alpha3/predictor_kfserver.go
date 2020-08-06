package v1alpha3

import v1 "k8s.io/api/core/v1"

// KFServerSpec defines arguments for configuring KFServer model serving.
type KFServerSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:"inline"`
}

// Validate returns an error if invalid
func (k *KFServerSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (k *KFServerSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (k *KFServerSpec) GetContainers() []v1.Container {
	return []v1.Container{}
}
