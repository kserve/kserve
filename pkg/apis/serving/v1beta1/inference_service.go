/*
Copyright 2021 The KServe Authors.

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
	v2 "k8s.io/api/autoscaling/v2"
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

// LoggerType controls the scope of log publishing
// +kubebuilder:validation:Enum=all;request;response
type LoggerType string

// LoggerType Enum
const (
	// LogAll Logger mode to log both request and response
	LogAll LoggerType = "all"
	// LogRequest Logger mode to log only request
	LogRequest LoggerType = "request"
	// LogResponse Logger mode to log only response
	LogResponse LoggerType = "response"
)

// LoggerSpec specifies optional payload logging available for all components
type LoggerSpec struct {
	// URL to send logging events
	// +optional
	URL *string `json:"url,omitempty"`
	// Specifies the scope of the loggers. <br />
	// Valid values are: <br />
	// - "all" (default): log both request and response; <br />
	// - "request": log only request; <br />
	// - "response": log only response <br />
	// +optional
	Mode LoggerType `json:"mode,omitempty"`
	// Matched metadata HTTP headers for propagating to inference logger cloud events.
	// +optional
	MetadataHeaders []string `json:"metadataHeaders,omitempty"`
}

type ScalerSpec struct {
	// Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero.
	// +optional
	MinReplicas *int `json:"minReplicas,omitempty"`
	// Maximum number of replicas for autoscaling.
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for.
	// concurrency and rps targets are supported by Knative Pod Autoscaler
	// (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/).
	// +optional
	ScaleTarget *int `json:"scaleTarget,omitempty"`
	// ScaleMetric defines the scaling metric type watched by autoscaler
	// possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via
	// Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics).
	// +optional
	ScaleMetric *ScaleMetric `json:"scaleMetric,omitempty"`
	// Type of metric to use. Options are Utilization, or AverageValue.
	// +optional
	ScaleMetricType *v2.MetricTargetType `json:"scaleMetricType,omitempty"`
	// Address of Prometheus server.
	// +optional
	ServerAddress string `json:"serverAddress,omitempty"`
	// Query to run to get metrics from Prometheus
	// +optional
	MetricQuery string `json:"metricQuery,omitempty"`
	//  A comma-separated list of query Parameters to include while querying the Prometheus endpoint.
	// +optional
	QueryParameters string `json:"queryParameters,omitempty"`
}

// Batcher specifies optional payload batching available for all components
type Batcher struct {
	// Specifies the max number of requests to trigger a batch
	// +optional
	MaxBatchSize *int `json:"maxBatchSize,omitempty"`
	// Specifies the max latency to trigger a batch
	// +optional
	MaxLatency *int `json:"maxLatency,omitempty"`
	// Specifies the timeout of a batch
	// +optional
	Timeout *int `json:"timeout,omitempty"`
}

// InferenceService is the Schema for the InferenceServices API
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Prev",type="integer",JSONPath=".status.components.predictor.traffic[?(@.tag=='prev')].percent"
// +kubebuilder:printcolumn:name="Latest",type="integer",JSONPath=".status.components.predictor.traffic[?(@.latestRevision==true)].percent"
// +kubebuilder:printcolumn:name="PrevRolledoutRevision",type="string",JSONPath=".status.components.predictor.traffic[?(@.tag=='prev')].revisionName"
// +kubebuilder:printcolumn:name="LatestReadyRevision",type="string",JSONPath=".status.components.predictor.traffic[?(@.latestRevision==true)].revisionName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferenceservices,shortName=isvc
// +kubebuilder:storageversion
type InferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InferenceServiceSpec `json:"spec,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	Status InferenceServiceStatus `json:"status,omitempty"`
}

// InferenceServiceList contains a list of Service
// +k8s:openapi-gen=true
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
