package v1alpha3

import v1 "k8s.io/api/core/v1"

// ONNXRuntimeSpec defines arguments for configuring ONNX model serving.
type ONNXRuntimeSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Allowed runtime versions are specified in the service config map
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// Validate returns an error if invalid
func (o *ONNXRuntimeSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (o *ONNXRuntimeSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (o *ONNXRuntimeSpec) GetContainers() []v1.Container {
	return []v1.Container{}
}
