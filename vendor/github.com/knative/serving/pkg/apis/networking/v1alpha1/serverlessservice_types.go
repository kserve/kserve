/*
Copyright 2019 The Knative Authors.

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
	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
	networking "github.com/knative/serving/pkg/apis/networking"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServerlessService is a proxy for the K8s service objects containing the
// endpoints for the revision, whether those are endpoints of the activator or
// revision pods.
// See: https://knative.page.link/naxz for details.
type ServerlessService struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the ServerlessService.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec ServerlessServiceSpec `json:"spec,omitempty"`

	// Status is the current state of the ServerlessService.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Status ServerlessServiceStatus `json:"status,omitempty"`
}

// Verify that ServerlessService adheres to the appropriate interfaces.
var (
	// Check that ServerlessService may be validated and defaulted.
	_ apis.Validatable = (*ServerlessService)(nil)
	_ apis.Defaultable = (*ServerlessService)(nil)

	// Check that we can create OwnerReferences to a ServerlessService.
	_ kmeta.OwnerRefable = (*ServerlessService)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServerlessServiceList is a collection of ServerlessService.
type ServerlessServiceList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of ServerlessService.
	Items []ServerlessService `json:"items"`
}

// ServerlessServiceOperationMode is an enumeration of the modes of operation
// for the ServerlessService.
type ServerlessServiceOperationMode string

const (
	// SKSOperationModeServe is reserved for the state when revision
	// pods are serving using traffic.
	SKSOperationModeServe ServerlessServiceOperationMode = "Serve"

	// SKSOperationModeProxy is reserved for the state when activator
	// pods are serving using traffic.
	SKSOperationModeProxy ServerlessServiceOperationMode = "Proxy"
)

// ServerlessServiceSpec describes the ServerlessService.
type ServerlessServiceSpec struct {
	// Mode describes the mode of operation of the ServerlessService.
	Mode ServerlessServiceOperationMode `json:"mode,omitempty"`

	// ObjectRef defines the resource that this ServerlessService
	// is responsible for making "serverless".
	ObjectRef corev1.ObjectReference `json:"objectRef"`

	// The application-layer protocol. Matches `RevisionProtocolType` set on the owning pa/revision.
	// serving imports networking, so just use string.
	ProtocolType networking.ProtocolType
}

// ServerlessServiceStatus describes the current state of the ServerlessService.
type ServerlessServiceStatus struct {
	duckv1beta1.Status `json:",inline"`

	// ServiceName holds the name of a core K8s Service resource that
	// load balances over the pods backing this Revision (activator or revision).
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// PrivateServiceName holds the name of a core K8s Service resource that
	// load balances over the user service pods backing this Revision.
	// +optional
	PrivateServiceName string `json:"privateServiceName,omitempty"`
}

// ConditionType represents a ServerlessService condition value
const (
	// ServerlessServiceConditionReady is set when the clusterIngress networking setting is
	// configured and it has a load balancer address.
	ServerlessServiceConditionReady = apis.ConditionReady

	// ServerlessServiceConditionEndspointsPopulated is set when the ServerlessService's underlying
	// Revision K8s Service has been populated with endpoints.
	ServerlessServiceConditionEndspointsPopulated apis.ConditionType = "EndpointsPopulated"
)
