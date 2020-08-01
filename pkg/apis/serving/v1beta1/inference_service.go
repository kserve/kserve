/*
Copyright 2020 kubeflow.org.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

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

// ComponentExtensionSpec defines the deployment configuration for a given InferenceService component
type ComponentExtensionSpec struct {
	// Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero.
	// +optional
	MinReplicas *int `json:"minReplicas,omitempty"`
	// Maximum number of replicas for autoscaling.
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container
	// concurrency(https://knative.dev/docs/serving/autoscaling/concurrency).
	// +optional
	ContainerConcurrency int `json:"containerConcurrency,omitempty"`
	// TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component.
	// +optional
	TimeoutSeconds int `json:"timeout,omitempty"`
	// Activate request/response logging and configurations
	// +optional
	LoggerSpec *LoggerSpec `json:"logger,omitempty"`
	// Activate request batching and batching configurations
	// +optional
	Batcher *Batcher `json:"batcher,omitempty"`
}

// +kubebuilder:validation:Enum=all;request;response
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
	// LoggerType [all, request, response]
	Mode LoggerType `json:"mode,omitempty"`
}

// Batcher provides optional payload batcher for all endpoints
// +experimental
type Batcher struct {
	// MaxBatchSize of batcher service
	// +optional
	MaxBatchSize *int `json:"maxBatchSize,omitempty"`
	// MaxLatency of batcher service
	// +optional
	MaxLatency *int `json:"maxLatency,omitempty"`
	// Timeout of batcher service
	// +optional
	Timeout *int `json:"timeout,omitempty"`
}

// InferenceService is the Schema for the inferenceservices API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferenceservices,shortName=isvc
// +kubebuilder:storageversion
type InferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceServiceSpec   `json:"spec,omitempty"`
	Status InferenceServiceStatus `json:"status,omitempty"`
}

// InferenceServiceList contains a list of Service
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
