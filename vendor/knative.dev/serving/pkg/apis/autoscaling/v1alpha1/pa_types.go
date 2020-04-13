/*
Copyright 2018 The Knative Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"
	net "knative.dev/serving/pkg/apis/networking"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodAutoscaler is a Knative abstraction that encapsulates the interface by which Knative
// components instantiate autoscalers.  This definition is an abstraction that may be backed
// by multiple definitions.  For more information, see the Knative Pluggability presentation:
// https://docs.google.com/presentation/d/10KWynvAJYuOEWy69VBa6bHJVCqIsz1TNdEKosNvcpPY/edit
type PodAutoscaler struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the PodAutoscaler (from the client).
	// +optional
	Spec PodAutoscalerSpec `json:"spec,omitempty"`

	// Status communicates the observed state of the PodAutoscaler (from the controller).
	// +optional
	Status PodAutoscalerStatus `json:"status,omitempty"`
}

// Verify that PodAutoscaler adheres to the appropriate interfaces.
var (
	// Check that PodAutoscaler can be validated and can be defaulted.
	_ apis.Validatable = (*PodAutoscaler)(nil)
	_ apis.Defaultable = (*PodAutoscaler)(nil)

	// Check that we can create OwnerReferences to a PodAutoscaler.
	_ kmeta.OwnerRefable = (*PodAutoscaler)(nil)
)

// ReachabilityType is the enumeration type for the different states of reachability
// to the `ScaleTarget` of a `PodAutoscaler`
type ReachabilityType string

const (
	// ReachabilityUnknown means the reachability of the `ScaleTarget` is unknown.
	// Used when the reachability cannot be determined, eg. during activation.
	ReachabilityUnknown ReachabilityType = ""

	// ReachabilityReachable means the `ScaleTarget` is reachable, ie. it has an active route.
	ReachabilityReachable ReachabilityType = "Reachable"

	// ReachabilityReachable means the `ScaleTarget` is not reachable, ie. it does not have an active route.
	ReachabilityUnreachable ReachabilityType = "Unreachable"
)

// PodAutoscalerSpec holds the desired state of the PodAutoscaler (from the client).
type PodAutoscalerSpec struct {
	// DeprecatedGeneration was used prior in Kubernetes versions <1.11
	// when metadata.generation was not being incremented by the api server
	//
	// This property will be dropped in future Knative releases and should
	// not be used - use metadata.generation
	//
	// Tracking issue: https://knative.dev/serving/issues/643
	//
	// +optional
	DeprecatedGeneration int64 `json:"generation,omitempty"`

	// ContainerConcurrency specifies the maximum allowed
	// in-flight (concurrent) requests per container of the Revision.
	// Defaults to `0` which means unlimited concurrency.
	// +optional
	ContainerConcurrency int64 `json:"containerConcurrency,omitempty"`

	// ScaleTargetRef defines the /scale-able resource that this PodAutoscaler
	// is responsible for quickly right-sizing.
	ScaleTargetRef corev1.ObjectReference `json:"scaleTargetRef"`

	// Reachable specifies whether or not the `ScaleTargetRef` can be reached (ie. has a route).
	// Defaults to `ReachabilityUnknown`
	// +optional
	Reachability ReachabilityType `json:"reachability,omitempty"`

	// The application-layer protocol. Matches `ProtocolType` inferred from the revision spec.
	ProtocolType net.ProtocolType `json:"protocolType"`
}

const (
	// PodAutoscalerConditionReady is set when the revision is starting to materialize
	// runtime resources, and becomes true when those resources are ready.
	PodAutoscalerConditionReady = apis.ConditionReady
	// PodAutoscalerConditionActive is set when the PodAutoscaler's ScaleTargetRef is receiving traffic.
	PodAutoscalerConditionActive apis.ConditionType = "Active"
)

// PodAutoscalerStatus communicates the observed state of the PodAutoscaler (from the controller).
type PodAutoscalerStatus struct {
	duckv1.Status `json:",inline"`

	// ServiceName is the K8s Service name that serves the revision, scaled by this PA.
	// The service is created and owned by the ServerlessService object owned by this PA.
	ServiceName string `json:"serviceName"`

	// MetricsServiceName is the K8s Service name that provides revision metrics.
	// The service is managed by the PA object.
	MetricsServiceName string `json:"metricsServiceName"`

	// DesiredScale shows the current desired number of replicas for the revision.
	DesiredScale *int32 `json:"desiredScale,omitempty"`

	// ActualScale shows the actual number of replicas for the revision.
	ActualScale *int32 `json:"actualScale,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodAutoscalerList is a list of PodAutoscaler resources
type PodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []PodAutoscaler `json:"items"`
}
