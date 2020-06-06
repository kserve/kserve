package v1beta1

import v1 "k8s.io/api/core/v1"

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	// Passthrough to underlying Pods
	*v1.PodTemplateSpec `json:",inline"`
	// Extensions available in all components
	*ComponentExtensionSpec `json:",inline"`
}
