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
type InferenceGraphSpec struct {
	// Map of InferenceGraph router nodes
	// Each node defines the routes for the current node and next routes
	Nodes map[string]InferenceRouter `json:"nodes"`
}

// InferenceRouterType constant for inference routing types
// +k8s:openapi-gen=true
// +kubebuilder:validation:Enum=Single;Splitter;Ensemble;Switch
type InferenceRouterType string

// InferenceRouterType Enum
const (
	// Single Default type only route to one destination
	Single InferenceRouterType = "Single"

	// Splitter router randomly routes the requests to the named service according to the weight
	Splitter InferenceRouterType = "Splitter"

	// Ensemble router routes the requests to multiple models and then merge the responses
	Ensemble InferenceRouterType = "Ensemble"

	// Switch routes the request to the model based on certain condition
	Switch InferenceRouterType = "Switch"
)

// +k8s:openapi-gen=true

// InferenceRouter defines the router for each InferenceGraph node with one or multiple models
// and where it routes to as next step
//
// ```yaml
// kind: InferenceGraph
// metadata:
//   name: canary-route
// spec:
//   nodes:
//     mymodel:
//       routerType: Splitter
//       routes:
//       - service: mymodel1
//         weight: 20
//       - service: mymodel2
//         weight: 80
// ```
//
// ```yaml
// kind: InferenceGraph
// metadata:
//   name: abtest
// spec:
//   nodes:
//     mymodel:
//       routerType: Switch
//       routes:
//       - service: mymodel1
//         condition: "{ $input.userId == 1 }"
//       - service: mymodel2
//         condition: "{ $input.userId == 2 }"
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
//   name: ensemble
// spec:
//   nodes:
//     transformer:
//       routes:
//       - service: feast
//       nextRoutes:
//       - nodeName: ensembleModel
//         data: "{ $output }"
//     ensembleModel:
//       routerType: Ensemble
//       routes:
//       - service: sklearn-model
//       - service: xgboost-model
// ```
//
// Scoring a case using a sequence, or chain, of models allows the output of one model to be passed in as input to subsequent models.
// ```yaml
// kind: InferenceGraph
// metadata:
//   name: model-chainer
// spec:
//   nodes:
//     mymodel-s1:
//       routes:
//       - service: mymodel-s1
//       nextRoutes:
//       - nodeName: mymodel-s2
//         data: "{ $output }"
//     mymodel-s2:
//       routes:
//       - service: mymodel-s2
// ```
//
// In the flow described below, the pre_processing node base64 encodes the image and passes it to two model nodes in the flow.
// The encoded data is available to both these nodes for classification. The second node i.e. dog-breed-classification takes the
// original input from the pre_processing node along-with the response from the cat-dog-classification node to do further classification
// of the dog breed if required.
// ```yaml
// kind: InferenceGraph
// metadata:
//   name: dog-breed-classification
// spec:
//   nodes:
//     top-level-classifier:
//       routes:
//       - service: cat-dog-classifier
//       nextRoutes:
//         nodeName: breed-classifier
//         data: "{ $input }"
//     breed-classifier:
//       routerType: Switch
//       routes:
//       - service: dog-breed-classifier
//         condition: { $output.predictions.class == "dog" }
//       - service: cat-breed-classifier
//         condition: { $output.predictions.class == "cat" }
// ```
type InferenceRouter struct {
	// RouterType
	//
	// - `Single:`: routes to a single service, the default router type
	//
	// - `Splitter:` randomly routes to the named service according to the weight
	//
	// - `Ensemble:` routes the request to multiple models and then merge the responses
	//
	// - `Switch:` routes the request to two or more services with specified routing rule
	//
	RouterType InferenceRouterType `json:"routerType"`

	// Routes defines destinations for the current router node
	// +optional
	Routes []InferenceRoute `json:"routes,omitempty"`

	// nextRoute defines where to route to as next step
	// +optional
	NextRoutes []RouteTo `json:"nextRoutes,omitempty"`
}

// +k8s:openapi-gen=true

type RouteTo struct {
	// The node name for routing as next step
	// +optional
	NodeName string `json:"nodeName"`

	// request data sent to the next route specified with jsonpath of the request or response json data
	// from the current step
	// $request
	// $response.predictions
	// +required
	Data string `json:"data"`
}

// +k8s:openapi-gen=true

type InferenceRoute struct {
	// named reference for InferenceService
	Service string `json:"service,omitempty"`

	// InferenceService URL, mutually exclusive with Service
	// +optional
	ServiceUrl string `json:"serviceUrl,omitempty"`

	// the weight for split of the traffic, only used for Split Router
	// when weight is specified all the routing targets should be sum to 100
	// +optional
	Weight *int64 `json:"weight,omitempty"`

	// routing based on the condition
	// +optional
	Condition string `json:"condition,omitempty"`
}

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
