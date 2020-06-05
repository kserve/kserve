package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InferenceServiceSpec is the top level type for this resource
type InferenceServiceSpec struct {
	// Predictor defines the model serving spec
	// +required
	Predictor PredictorSpec `json:"predictor"`
	// Explainer defines the model explanation service spec,
	// explainer service calls to predictor or transformer if it is specified.
	// +optional
	Explainer *ExplainerSpec `json:"explainer,omitempty"`
	// Transformer defines the pre/post processing before and after the predictor call,
	// transformer service calls to predictor service.
	// +optional
	Transformer *TransformerSpec `json:"transformer,omitempty"`
}

// ComponentExtensionSpec defines the configuration for a given inferenceservice component
type ComponentExtensionSpec struct {
	// Minimum number of replicas, defaults to 1 but can be set to enable scale-to-zero.
	// +optional
	MinReplicas *int `json:"minReplicas,omitempty"`
	// Maximum number of replicas for autoscaling.
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// Concurrency specifies how many requests can be processed concurrently, this sets the target
	// concurrency for Autoscaling(KPA). For model servers that support tuning parallelism will use this value,
	// by default the parallelism is the number of the CPU cores for most of the model servers.
	// +optional
	ContainerConcurrency int `json:"parallelism,omitempty"`
	// TimeoutSeconds specifies the numberof seconds to wait before timing out a request to the component.
	// +optional
	TimeoutSeconds int `json:"timeout,omitempty"`
	// Specify request and response logging
	// +optional
	LoggerSpec *LoggerSpec `json:"logger,omitempty"`
}

// LoggerType controls the scope of log publishing
type LoggerType string

// LoggerType Enum
const (
	LogAll      LoggerType = "all"
	LogRequest  LoggerType = "request"
	LogResponse LoggerType = "response"
)

// LoggerSpec provides optional payload logging for all endpoints
// +experimental
type LoggerSpec struct {
	// URL to send logging events
	// +optional
	URL *string `json:"url,omitempty"`
	// See Enum: LoggerType
	Mode LoggerType `json:"mode,omitempty"`
}

// ComponentType contains the different types of components of the inferenceservice
type ComponentType string

// ComponentType Enum
const (
	PredictorComponent   ComponentType = "predictor"
	ExplainerComponent   ComponentType = "explainer"
	TransformerComponent ComponentType = "transformer"
)

// InferenceService is the Schema for the inferenceservices API
// +k8s:openapi-gen=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferenceservices,shortName=isvc
type InferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceServiceSpec   `json:"spec,omitempty"`
	Status InferenceServiceStatus `json:"status,omitempty"`
}

// InferenceServiceList contains a list of Service
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type InferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []InferenceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceService{}, &InferenceServiceList{})
}
