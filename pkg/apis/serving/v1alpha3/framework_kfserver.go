package v1alpha3

import v1 "k8s.io/api/core/v1"

// KFServerSpec defines arguments for configuring PyTorch model serving.
type KFServerSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults PyTorch model class name to 'PyTorchModel'
	ModelClassName string `json:"modelClassName,omitempty"`
	// Allowed runtime versions are specified in the service config map
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
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
