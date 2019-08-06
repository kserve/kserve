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
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/kmeta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterIngress is a collection of rules that allow inbound connections to reach the
// endpoints defined by a backend. A ClusterIngress can be configured to give services
// externally-reachable URLs, load balance traffic, offer name based virtual hosting, etc.
//
// This is heavily based on K8s Ingress https://godoc.org/k8s.io/api/extensions/v1beta1#Ingress
// which some highlighted modifications.
type ClusterIngress struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the ClusterIngress.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec IngressSpec `json:"spec,omitempty"`

	// Status is the current state of the ClusterIngress.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Status IngressStatus `json:"status,omitempty"`
}

// Verify that ClusterIngress adheres to the appropriate interfaces.
var (
	// Check that ClusterIngress may be validated and defaulted.
	_ apis.Validatable = (*ClusterIngress)(nil)
	_ apis.Defaultable = (*ClusterIngress)(nil)

	// Check that we can create OwnerReferences to a ClusterIngress.
	_ kmeta.OwnerRefable = (*ClusterIngress)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterIngressList is a collection of ClusterIngress objects.
type ClusterIngressList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of ClusterIngress objects.
	Items []ClusterIngress `json:"items"`
}
