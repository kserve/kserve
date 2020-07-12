package v1beta1

import v1 "k8s.io/api/core/v1"

// TorchServeSpec defines arguments for configuring PyTorch model serving.
type TorchServeSpec struct {
	// Defaults PyTorch model class name to 'PyTorchModel'
	ModelClassName string `json:"modelClassName,omitempty"`
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TorchServeSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TorchServeSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (t *TorchServeSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	return &v1.Container{

	}
}
