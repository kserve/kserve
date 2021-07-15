package v1alpha1

import (
	istio_networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"net/url"
)

// InferenceGraph is the Schema for the InferenceGraph API for multiple models
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferencegraphs,shortName=ir,singular=inferencegraph
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

type InferenceRouterType string

const (
	// Split router randomly routes the requests to the named service according to the weight
	Splitter InferenceRouterType = "Splitter"

	// Ensemble router routes the requests to multiple models and then merge the responses
	Ensemble InferenceRouterType = "Ensemble"

	// ABNTest routes the request to two or more models with specified routing rule
	ABNTest InferenceRouterType = "ABNTest"
)

// +k8s:openapi-gen=true
// InferenceRouter defines the router for each InferenceGraph node and where it routes to as next step
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
//       routerType: ABNTest
//       routes:
//       - service: mymodel1
//         headers:
//         - userid:
//             exact: 1
//       - service: mymodel2
//         headers:
//         - userid:
//             exact: 2
// ```
//
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
//         data: $response
//     ensembleModel:
//       routerType: Ensemble
//       routes:
//       - service: sklearn-model
//       - service: xgboost-model
// ```
//
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
//         data: $response
//     mymodel-s2:
//       routes:
//       - service: mymodel-s2
// ```
//
// ```yaml
// kind: InferenceGraph
// metadata:
//   name: conditional-model-chainer
// spec:
//   nodes:
//     news-classifier:
//       routerType: Splitter
//       routes:
//       - service: top-level-classifier
//       nextRoutes:
//       - nodeName: sports-categorizer
//         AllOf:
//         - required: ["class"]
//           properties:
//             class:
//               pattern: "sports"
//         data: $request
//       - nodeName: sports-categorizer
//         AllOf:
//         - required: ["class"]
//           properties:
//             class:
//               pattern: "stock"
//         data: $request
//     sports-categorizer:
//       routerType: Splitter
//       routes:
//       - service: sports-model
//     stock-categorizer:
//       routerType: Splitter
//       routes:
//       - service: stock-model
// ```
type InferenceRouter struct {
	// RouterType
	//
	// - `Splitter:` randomly routes to the named service according to the weight, the default router type
	//
	// - `Ensemble:` routes the request to multiple models and then merge the responses
	//
	// - `ABNTest:` routes the request to two or more services with specified routing rule
	//
	RouterType InferenceRouterType `json:"routerType"`

	// Routes defines destinations for the current router node
	// +optional
	Routes []InferenceRoute `json:"routes,omitempty"`

	// nextRoutes defines where to route to as next step
	// +optional
	NextRoutes []RouteTo `json:"nextRoutes,omitempty"`
}

// +k8s:openapi-gen=true
type InferenceRoute struct {
	// named reference for InferenceService
	// +optional
	Service string `json:"service"`

	// InferenceService URL, mutually exclusive with Service
	// +optional
	ServiceUrl *url.URL `json:"serviceUrl"`

	// the weight for split of the traffic, only used for Split Router
	// when weight is specified all the routing targets should be sum to 100
	// +optional
	Weight *int64 `json:"weight,omitempty"`

	// routing based on the headers
	// +optional
	Headers map[string]istio_networking.StringMatch `json:"headers,omitempty"`
}

// +k8s:openapi-gen=true
// RouteTo defines the outgoing route for the current InferenceGraph node
type RouteTo struct {
	// next named router node
	// +required
	NodeName string `json:"nodeName"`
	// when the condition validates the request data is then sent to the next router
	// e.g
	// allOf
	//  - required: ["class"]
	//    properties:
	//      class:
	//        pattern: "1"
	// +optional
	Condition v1.JSONSchemaDefinitions `json:"condition,omitempty"`
	// request data sent to the next route specified with jsonpath of the request or response json data
	// from the current step
	// e.g
	// $request
	// $response.predictions
	// +required
	Data string `json:"data"`
}

type InferenceGraphStatus struct {
	// Conditions for InferenceGraph
	duckv1.Status `json:",inline"`
}
