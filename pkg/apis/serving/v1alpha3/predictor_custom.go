package v1alpha3

import v1 "k8s.io/api/core/v1"

// CustomPredictor defines arguments for configuring a custom server.
type CustomPredictor struct {
	// This spec is dual purpose.
	// 1) Users may choose to provide a full PodSpec for their predictor.
	// The field PodSpec.Containers is mutually exclusive with other Predictors (i.e. TFServing).
	// 2) Users may choose to provide a Predictor (i.e. TFServing) and specify PodSpec
	// overrides in the CustomPredictor PodSpec. They must not provide PodSpec.Containers in this case.
	v1.PodSpec `json:",inline"`
}

// Validate returns an error if invalid
func (c *CustomPredictor) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (c *CustomPredictor) Default() {}

// GetContainers transforms the resource into a container spec
func (c *CustomPredictor) GetContainers() []v1.Container {
	return c.Containers
}
