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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// InferenceRouterSpec is the top level type for this resource
// A router contains a set of strategies
type InferenceRouterSpec struct {
	// Routes is a list of route which can receive traffic
	// All routes are expected to have an equivalent data plane interface
	// RouteSpec is validated based on RouterType
	// +required
	Routes []RouteSpec `json:"routes"`
	// Splitter splits the traffic among multiple routes, the number of routes should be equal to the
	// the elements in weight array and the weights should sum to 100.
	// +optional
	Splitter *SplitterSpec `json:"splitter,omitempty"`
	// AB testing between new and old model and compare the performance based on predefined metric.
	// +optional
	ABTest *ABTestSpec `json:"abTest,omitempty"`
	// MultiArmBandit is an inference router node which routes traffic to the best model most of time,
	// it also explores the other models, e.g epsilon greedy strategy routes to a random model with
	// probability e and best model with probability 1-e.
	// +optional
	MultiArmBandit *MultiArmBanditSpec `json:"multiArmBandit,omitempty"`
	// Ensemble is an inference router node which can combine results from previous steps
	// +optional
	Ensemble *EnsembleSpec `json:"ensemble,omitempty"`
	// Pipeline defines a series of steps which are depending on previous step.
	// Only one root node is allowed and all other routes should specify dependencies with the consumes field.
	// The graph should form a valid DAG, no cycles are allowed, branches are allowed but they needs
	// be mutually exclusive. Each route on the pipeline can be either an InferenceService or an Inference Router.
	// +optional
	Pipeline *PipelineSpec `json:"pipeline,omitempty"`
}

// RouteSpec defines the available routes in this router. Route functions reference routes by Name
type RouteSpec struct {
	// The name for the route
	Name string `json:"name"`
	// The destination of the route
	Destination duckv1.Destination `json:",inline"`
	// The header keys must be lowercase and use hyphen as the separator,
	// e.g. _x-request-id_.
	//
	// Header values are case-sensitive and formatted as follows:
	// - `exact: "value"` for exact string match
	// - `prefix: "value"` for prefix-based match
	// - `regex: "value"` for regex-based match
	Headers map[string]*StringMatch `json:"headers,omitempty"`
	// This is a list of pipeline route names that should be executed before the current route.
	// The field is only valid for pipeline routing type
	Consumes []string `json:"consumes,omitempty"`
}

// Describes how to match a given string in HTTP headers. Match is
// case-sensitive and you can only do one-of the following string matching type
type StringMatch struct {
	// Exact string match
	Exact string `json:"exact,omitempty"`
	// Prefix-based string match
	Prefix string `json:"prefix,omitempty"`
	// Regex-based string match
	Regex string `json:"regex,omitempty"`
}

// InferenceRouter is the Schema for the routers API
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
