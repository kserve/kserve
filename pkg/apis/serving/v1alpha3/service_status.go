package v1alpha3

import (
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

// ComponentStatusMap defines the observed state of service endpoints
type ComponentStatusMap map[ComponentType]ComponentStatusSpec

// ServiceStatus defines the observed state of service
type ServiceStatus struct {
	duckv1beta1.Status `json:",inline"`
	// Statuses for the components of the service
	Components *ComponentStatusMap `json:"components,omitempty"`
	// Addressable endpoint for the service
	Address *duckv1beta1.Addressable `json:"address,omitempty"`
}

// ComponentStatusSpec describes the state of the component
type ComponentStatusSpec struct {
	// Latest revision name that is in ready state
	Name string `json:"name,omitempty"`
	// Addressable endpoint for the service
	Address *duckv1beta1.Addressable `json:"address,omitempty"`
}
