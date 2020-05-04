package v1alpha3

import v1 "k8s.io/api/core/v1"

// TritonSpec defines arguments for configuring Triton model serving.
type TritonSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Allowed runtime versions are specified in the service config map
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// Validate returns an error if invalid
func (t *TritonSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TritonSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (t *TritonSpec) GetContainers() []v1.Container {
	return []v1.Container{}
}
