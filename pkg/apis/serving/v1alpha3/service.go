package v1alpha3

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceSpec is the top level type for this resource
type ServiceSpec struct {
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

// PredictorSpec defines the configuration for a predictor,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type PredictorSpec struct {
	// Spec for KFServer
	KFServer *KFServerSpec `json:"kfserver,omitempty"`
	// Spec for TFServing (https://github.com/tensorflow/serving)
	TFServing *TFServingSpec `json:"tfserving,omitempty"`
	// Spec for PyTorch predictor
	TorchServe *TorchServeSpec `json:"torchserve,omitempty"`
	// Spec for Triton Inference Server (https://github.com/NVIDIA/triton-inference-server)
	Triton *TritonSpec `json:"triton,omitempty"`
	// Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)
	ONNXRuntime *ONNXRuntimeSpec `json:"onnxruntime,omitempty"`
	// Passthrough to underlying Pods
	*CustomFramework `json:",inline"`
}

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

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	// Passthrough to underlying Pods
	*v1.PodSpec `json:",inline"`
	// Extensions available in all components
	*ComponentExtensionSpec `json:",inline"`
}

// ComponentExtensionSpec defines the configuration for a given service service component
type ComponentExtensionSpec struct {
	// Minimum number of replicas, defaults to 1 but can be set to enable scale-to-zero.
	// +optional
	MinReplicas *int `json:"minReplicas,omitempty"`
	// Maximum number of replicas for autoscaling.
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// Parallelism specifies how many requests can be processed concurrently, this sets the target
	// concurrency for Autoscaling(KPA). For model servers that support tuning parallelism will use this value,
	// by default the parallelism is the number of the CPU cores for most of the model servers.
	// +optional
	Parallelism int `json:"parallelism,omitempty"`
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

// ComponentType contains the different types of components of the service
type ComponentType string

// ComponentType Enum
const (
	Predictor   ComponentType = "predictor"
	Explainer   ComponentType = "explainer"
	Transformer ComponentType = "transformer"
)

// Service is the Schema for the services API
// +k8s:openapi-gen=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=services,shortName=kfsvc
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// ServiceList contains a list of Service
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
