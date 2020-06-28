package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InferenceRouterSpec is the top level type for this resource
// A router contains a set of strategies
type InferenceRouterSpec struct {
	// Routes is a list of route which can receive traffic
	// All routes are expected to have an equivalent data plane interface
	// +required
	Routes []RouteSpec `json:"routes"`
	// +optional
	Splitter *SplitterSpec `json:"splitter,omitempty"`
	// +optional
	ABTest *ABTestSpec `json:"abTest,omitempty"`
	// +optional
	MultiArmBandit *MultiArmBanditSpec `json:"multiArmBandit,omitempty"`
	// +optional
	Ensemble *EnsembleSpec `json:"ensemble,omitempty"`
	// +optional
	Pipeline *PipelineSpec `json:"pipeline,omitempty"`
}

// RouteSpec defines the available routes in this router. Route functions reference routes by Name
type RouteSpec struct {
	// The name for the route
	Name string `json:"name"`
	// The URL of the route
	URL string `json:"url"`
}

// InferenceRouter is the Schema for the routers API
// +k8s:openapi-gen=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferencerouters,shortName=irouter
type InferenceRouter struct {
	metav1.TypeMeta   `json:"inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceRouterSpec   `json:"spec,omitempty"`
	Status InferenceRouterStatus `json:"status,omitempty"`
}

// InferenceRouterList contains a list of Router
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type InferenceRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []InferenceRouter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceRouter{}, &InferenceRouterList{})
}
