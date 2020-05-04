package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RouterSpec is the top level type for this resource
type RouterSpec struct {
	// Routes is a list of route which can receive traffic
	// All routes are expected to have an equivalent data plane interface
	// +required
	Routes *map[string]int `json:"routes,omitempty"`
	// +optional
	ABTest *ABTestSpec `json:"abTest,omitempty"`
	// +optional
	MultiArmBandit *MultiArmBanditSpec `json:"multiArmBandit,omitempty"`
}

// ABTestSpec is TODO
type ABTestSpec struct{}

// MultiArmBanditSpec is TODO
type MultiArmBanditSpec struct{}

// Router is the Schema for the routers API
// +k8s:openapi-gen=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=routers,shortName=kfrouter
type Router struct {
	metav1.TypeMeta   `json:"inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouterSpec   `json:"spec,omitempty"`
	Status RouterStatus `json:"status,omitempty"`
}

// RouterList contains a list of Router
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type RouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []Router `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Router{}, &RouterList{})
}
