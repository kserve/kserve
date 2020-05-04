package v1alpha3

import v1 "k8s.io/api/core/v1"

// ExplainerSpec defines the arguments for a model explanation server,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type ExplainerSpec struct {
	// Spec for alibi explainer
	Alibi *AlibiExplainerSpec `json:"alibi,omitempty"`
	// Passthrough to underlying Pods
	*v1.PodSpec `json:",inline"`
	// Extensions available in all components
	*ComponentExtensionSpec `json:",inline"`
}
