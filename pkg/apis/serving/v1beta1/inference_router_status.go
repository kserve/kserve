package v1beta1

import (
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

// InferenceRouterStatus defines the observed state of resource
type InferenceRouterStatus struct {
	duckv1beta1.Status `json:",inline"`
	// Addressable endpoint for the router
	Address *duckv1beta1.Addressable `json:"address,omitempty"`
}
