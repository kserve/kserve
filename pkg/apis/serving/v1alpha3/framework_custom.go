package v1alpha3

import v1 "k8s.io/api/core/v1"

// CustomFramework defines arguments for configuring ONNX model serving.
type CustomFramework struct {
	v1.PodSpec `json:",inline"`
}

// Validate returns an error if invalid
func (c *CustomFramework) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (c *CustomFramework) Default() {}

// GetContainers transforms the resource into a container spec
func (c *CustomFramework) GetContainers() []v1.Container {
	return c.Containers
}
