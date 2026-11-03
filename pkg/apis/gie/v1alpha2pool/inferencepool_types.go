/*
Copyright 2025 The Kubernetes Authors.
Copyright 2026 The KServe Authors.

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

// Package v1alpha2pool contains InferencePool types from the
// inference.networking.x-k8s.io/v1alpha2 API group that were removed upstream
// in GIE v1.5.0. This local copy preserves backward compatibility with the
// old CRD while allowing KServe to upgrade the upstream GIE dependency.
//
// These types are vendored from sigs.k8s.io/gateway-api-inference-extension@v1.4.0/apix/v1alpha2/.
// They should be removed once v1alpha2 InferencePool support is fully deprecated.
package v1alpha2pool

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	upstream "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"
)

// Re-export shared types from upstream (still present in v1.5.0).
// LabelKey and LabelValue are defined as concrete types (not aliases) because
// controller-gen cannot resolve type aliases for map keys in CRD generation.
type (
	Group      = upstream.Group
	Kind       = upstream.Kind
	ObjectName = upstream.ObjectName
	Namespace  = upstream.Namespace
	PortNumber = upstream.PortNumber
)

// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
type LabelKey string

// +kubebuilder:validation:MinLength=0
// +kubebuilder:validation:MaxLength=63
type LabelValue string

// InferencePool is the Schema for the InferencePools API.
// +kubebuilder:object:root=true
type InferencePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InferencePoolSpec `json:"spec,omitempty"`

	Status InferencePoolStatus `json:"status,omitempty"`
}

// InferencePoolList contains a list of InferencePool.
//
// +kubebuilder:object:root=true
type InferencePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferencePool `json:"items"`
}

// InferencePoolSpec defines the desired state of InferencePool
type InferencePoolSpec struct {
	// Selector defines a map of labels to watch model server Pods
	// that should be included in the InferencePool.
	Selector map[LabelKey]LabelValue `json:"selector"`

	// TargetPortNumber defines the port number to access the selected model server Pods.
	TargetPortNumber int32 `json:"targetPortNumber"`

	// Extension configures an endpoint picker as an extension service.
	ExtensionRef Extension `json:"extensionRef,omitempty"`
}

// Extension specifies how to configure an extension that runs the endpoint picker.
type Extension struct {
	// Group is the group of the referent.
	Group *Group `json:"group,omitempty"`

	// Kind is the Kubernetes resource kind of the referent.
	Kind *Kind `json:"kind,omitempty"`

	// Name is the name of the referent.
	Name ObjectName `json:"name"`

	// The port number on the service running the extension.
	PortNumber *PortNumber `json:"portNumber,omitempty"`

	// Configures how the gateway handles the case when the extension is not responsive.
	FailureMode *ExtensionFailureMode `json:"failureMode"`
}

// ExtensionFailureMode defines the options for how the gateway handles the case when the extension is not
// responsive.
// +kubebuilder:validation:Enum=FailOpen;FailClose
type ExtensionFailureMode string

const (
	// FailOpen specifies that the proxy should forward the request to an endpoint of its picking when the Endpoint Picker fails.
	FailOpen ExtensionFailureMode = "FailOpen"
	// FailClose specifies that the proxy should drop the request when the Endpoint Picker fails.
	FailClose ExtensionFailureMode = "FailClose"
)

// InferencePoolStatus defines the observed state of InferencePool.
type InferencePoolStatus struct {
	// Parents is a list of parent resources (usually Gateways) that are
	// associated with the InferencePool, and the status of the InferencePool with respect to
	// each parent.
	Parents []PoolStatus `json:"parent,omitempty"`
}

// PoolStatus defines the observed state of InferencePool from a Gateway.
type PoolStatus struct {
	// GatewayRef indicates the gateway that observed state of InferencePool.
	GatewayRef ParentGatewayReference `json:"parentRef"`

	// Conditions track the state of the InferencePool.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// InferencePoolConditionType is a type of condition for the InferencePool
type InferencePoolConditionType string

// InferencePoolReason is the reason for a given InferencePoolConditionType
type InferencePoolReason string

const (
	InferencePoolConditionAccepted InferencePoolConditionType = "Accepted"

	InferencePoolReasonAccepted              InferencePoolReason = "Accepted"
	InferencePoolReasonNotSupportedByGateway InferencePoolReason = "NotSupportedByGateway"
	InferencePoolReasonHTTPRouteNotAccepted  InferencePoolReason = "HTTPRouteNotAccepted"
	InferencePoolReasonPending               InferencePoolReason = "Pending"
)

const (
	InferencePoolConditionResolvedRefs InferencePoolConditionType = "ResolvedRefs"

	InferencePoolReasonResolvedRefs        InferencePoolReason = "ResolvedRefs"
	InferencePoolReasonInvalidExtensionRef InferencePoolReason = "InvalidExtensionRef"
)

// ParentGatewayReference identifies an API object including its namespace,
// defaulting to Gateway.
type ParentGatewayReference struct {
	Group     *Group     `json:"group"`
	Kind      *Kind      `json:"kind"`
	Name      ObjectName `json:"name"`
	Namespace *Namespace `json:"namespace,omitempty"`
}
