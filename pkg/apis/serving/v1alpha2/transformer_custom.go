package v1alpha2

import (
	v1 "k8s.io/api/core/v1"
)

var _ Transformer = (*CustomSpec)(nil)

// GetContainerSpec for the CustomSpec
func (c *CustomSpec) GetTransformerContainer() *v1.Container {
	return &c.Container
}
