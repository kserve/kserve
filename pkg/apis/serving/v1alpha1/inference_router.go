package v1alpha1

import (
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// InferenceRouter is the Schema for the Routing API for multiple models
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferencerouters,shortName=ir,singular=inferencerouter
type InferenceRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InferenceRouterSpec   `json:"spec,omitempty"`
	Status            InferenceRouterStatus `json:"status,omitempty"`
}

// InferenceRouterSpec defines the InferenceRouter spec
type InferenceRouterSpec struct {
    Steps map[string]InferenceStep `json:"steps"`
}

type InferenceStep struct{
    Routes []InferenceRoute `json:"routes,omitempty"`
	nextRoutes []NextStep   `json:"nextRoutes,omitempty"`
}

type InferenceRoute struct {
	// named reference for InferenceService
	// +optional
	Service string `json:"service"`
	// the weight for split of the traffic
	// when weight is specified all the routing targets should be sum to 100
	Weight *int64 `json:"weight,omitempty"`
	// routing based on the headers
	Headers []string `json:"headers"`
}

// +k8s:openapi-gen=true
type NextStep struct {
	// next named step
	// +required
	StepName string `json:"stepName"`
	// when the condition validates the request is then sent to the corresponding step
	// e.g
	// allOf
	//  - required: ["class"]
	//    properties:
	//      class:
	//        pattern: "1"
	// +optional
	Condition v1.JSONSchemaDefinitions `json:"condition,omitempty"`

}

type InferenceRouterStatus struct {
	// Conditions for InferenceGraph
	duckv1.Status `json:",inline"`
}