/*
Copyright 2022 The KServe Authors.

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
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// InferenceGraph is the Schema for the InferenceGraph API for multiple models
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferencegraphs,shortName=ig,singular=inferencegraph
type InferenceGraph struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InferenceGraphSpec   `json:"spec,omitempty"`
	Status            InferenceGraphStatus `json:"status,omitempty"`
}

// InferenceGraphSpec defines the InferenceGraph spec
// +k8s:openapi-gen=true
type InferenceGraphSpec struct {
	// Map of InferenceGraph router nodes
	// Each node defines the router which can be different routing types
	Nodes map[string]InferenceRouter `json:"nodes"`
}

// InferenceRouterType constant for inference routing types
// +k8s:openapi-gen=true
// +kubebuilder:validation:Enum=Sequence;Splitter;Ensemble;Switch
type InferenceRouterType string

// InferenceRouterType Enum
const (
	// Sequence Default type only route to one destination
	Sequence InferenceRouterType = "Sequence"

	// Splitter router randomly routes the requests to the named service according to the weight
	Splitter InferenceRouterType = "Splitter"

	// Ensemble router routes the requests to multiple models and then merge the responses
	Ensemble InferenceRouterType = "Ensemble"

	// Switch routes the request to the model based on certain condition
	Switch InferenceRouterType = "Switch"
)

const (
	// GraphRootNodeName is the root node name.
	GraphRootNodeName string = "root"
)

// +k8s:openapi-gen=true
// InferenceRouter defines the router for each InferenceGraph node with one or multiple steps
//
// ```yaml
// kind: InferenceGraph
// metadata:
//
//	name: canary-route
//
// spec:
//
//	nodes:
//	  root:
//	    routerType: Splitter
//	    routes:
//	    - service: mymodel1
//	      weight: 20
//	    - service: mymodel2
//	      weight: 80
//
// ```
//
// ```yaml
// kind: InferenceGraph
// metadata:
//
//	name: abtest
//
// spec:
//
//	nodes:
//	  mymodel:
//	    routerType: Switch
//	    routes:
//	    - service: mymodel1
//	      condition: "{ .input.userId == 1 }"
//	    - service: mymodel2
//	      condition: "{ .input.userId == 2 }"
//
// ```
//
// Scoring a case using a model ensemble consists of scoring it using each model separately,
// then combining the results into a single scoring result using one of the pre-defined combination methods.
//
// Tree Ensemble constitutes a case where simple algorithms for combining results of either classification or regression trees are well known.
// Multiple classification trees, for example, are commonly combined using a "majority-vote" method.
// Multiple regression trees are often combined using various averaging techniques.
// e.g tagging models with segment identifiers and weights to be used for their combination in these ways.
// ```yaml
// kind: InferenceGraph
// metadata:
//
//	name: ensemble
//
// spec:
//
//	nodes:
//	  root:
//	    routerType: Sequence
//	    routes:
//	    - service: feast
//	    - nodeName: ensembleModel
//	      data: $response
//	  ensembleModel:
//	    routerType: Ensemble
//	    routes:
//	    - service: sklearn-model
//	    - service: xgboost-model
//
// ```
//
// Scoring a case using a sequence, or chain of models allows the output of one model to be passed in as input to the subsequent models.
// ```yaml
// kind: InferenceGraph
// metadata:
//
//	name: model-chainer
//
// spec:
//
//	nodes:
//	  root:
//	    routerType: Sequence
//	    routes:
//	    - service: mymodel-s1
//	    - service: mymodel-s2
//	      data: $response
//	    - service: mymodel-s3
//	      data: $response
//
// ```
//
// In the flow described below, the pre_processing node base64 encodes the image and passes it to two model nodes in the flow.
// The encoded data is available to both these nodes for classification. The second node i.e. dog-breed-classification takes the
// original input from the pre_processing node along-with the response from the cat-dog-classification node to do further classification
// of the dog breed if required.
// ```yaml
// kind: InferenceGraph
// metadata:
//
//	name: dog-breed-classification
//
// spec:
//
//	nodes:
//	  root:
//	    routerType: Sequence
//	    routes:
//	    - service: cat-dog-classifier
//	    - nodeName: breed-classifier
//	      data: $request
//	  breed-classifier:
//	    routerType: Switch
//	    routes:
//	    - service: dog-breed-classifier
//	      condition: { .predictions.class == "dog" }
//	    - service: cat-breed-classifier
//	      condition: { .predictions.class == "cat" }
//
// ```
type InferenceRouter struct {
	// RouterType
	//
	// - `Sequence:` chain multiple inference steps with input/output from previous step
	//
	// - `Splitter:` randomly routes to the target service according to the weight
	//
	// - `Ensemble:` routes the request to multiple models and then merge the responses
	//
	// - `Switch:` routes the request to one of the steps based on condition
	//
	RouterType InferenceRouterType `json:"routerType"`

	// Steps defines destinations for the current router node
	// +optional
	Steps []InferenceStep `json:"steps,omitempty"`
}

// +k8s:openapi-gen=true
// Exactly one InferenceTarget field must be specified
type InferenceTarget struct {
	// The node name for routing as next step
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// named reference for InferenceService
	ServiceName string `json:"serviceName,omitempty"`

	// InferenceService URL, mutually exclusive with ServiceName
	// +optional
	ServiceURL string `json:"serviceUrl,omitempty"`
}

// InferenceStep defines the inference target of the current step with condition, weights and data.
// +k8s:openapi-gen=true
type InferenceStep struct {
	// Unique name for the step within this node
	// +optional
	StepName string `json:"name,omitempty"`

	// Node or service used to process this step
	InferenceTarget `json:",inline"`

	// request data sent to the next route with input/output from the previous step
	// $request
	// $response.predictions
	// +optional
	Data string `json:"data,omitempty"`

	// the weight for split of the traffic, only used for Split Router
	// when weight is specified all the routing targets should be sum to 100
	// +optional
	Weight *int64 `json:"weight,omitempty"`

	// routing based on the condition
	// +optional
	Condition string `json:"condition,omitempty"`
}

// InferenceGraphStatus defines the InferenceGraph conditions and status
// +k8s:openapi-gen=true
type InferenceGraphStatus struct {
	// Conditions for InferenceGraph
	duckv1.Status `json:",inline"`
	// Url for the InferenceGraph
	// +optional
	URL *apis.URL `json:"url,omitempty"`
}

// InferenceGraphList contains a list of InferenceGraph
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type InferenceGraphList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []InferenceGraph `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceGraph{}, &InferenceGraphList{})
}
