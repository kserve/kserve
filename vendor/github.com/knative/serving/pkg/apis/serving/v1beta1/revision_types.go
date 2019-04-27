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

package v1beta1

import (
	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Revision is an immutable snapshot of code and configuration.  A revision
// references a container image. Revisions are created by updates to a
// Configuration.
//
// See also: https://github.com/knative/serving/blob/master/docs/spec/overview.md#revision
type Revision struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec RevisionSpec `json:"spec,omitempty"`

	// +optional
	Status RevisionStatus `json:"status,omitempty"`
}

// Verify that Revision adheres to the appropriate interfaces.
var (
	// Check that Revision can be validated, can be defaulted, and has immutable fields.
	_ apis.Validatable = (*Revision)(nil)
	_ apis.Defaultable = (*Revision)(nil)

	// Check that we can create OwnerReferences to a Revision.
	_ kmeta.OwnerRefable = (*Revision)(nil)
)

// RevisionTemplateSpec describes the data a revision should have when created from a template.
// Based on: https://github.com/kubernetes/api/blob/e771f807/core/v1/types.go#L3179-L3190
type RevisionTemplateSpec struct {
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec RevisionSpec `json:"spec,omitempty"`
}

// RevisionContainerConcurrencyType is an integer expressing the maximum number of
// in-flight (concurrent) requests.
type RevisionContainerConcurrencyType int64

const (
	// RevisionContainerConcurrencyMax is the maximum configurable
	// container concurrency.
	RevisionContainerConcurrencyMax RevisionContainerConcurrencyType = 1000
)

// PodSpec is our standing for corev1.PodSpec.
// TODO(mattmoor): We cannot inline PodSpec until 0.7 when the containers
// field it introduces has been available for a release because this
// definition doesn't "omitempty" the containers field, which means that
// 0.6 clients would send `containers: null`, which 0.5 webhooks will
// reject.  Once we ship a webhook that will parse the null properly, we
// can switch to the PodSpec definition.
type PodSpec struct {
	// ServiceAccountName holds the name of the Kubernetes service account
	// as which the underlying K8s resources should be run. If unspecified
	// this will default to the "default" service account for the namespace
	// in which the Revision exists.
	// This may be used to provide access to private container images by
	// following: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account
	// TODO(ZhiminXiang): verify the corresponding service account exists.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Containers holds the single container that defines the unit of
	// execution for this Revision.
	// In the context of a Revision, we disallow a number of the fields of
	// this Container, including: name and lifecycle.
	// See also the runtime contract for more information about the execution
	// environment:
	// https://github.com/knative/serving/blob/master/docs/runtime-contract.md
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`

	// Volumes defines a set of Kubernetes volumes to be mounted into the
	// specified Container.  Currently only ConfigMap and Secret volumes are
	// supported.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`
}

// RevisionSpec holds the desired state of the Revision (from the client).
type RevisionSpec struct {
	PodSpec `json:",inline"`

	// ContainerConcurrency specifies the maximum allowed in-flight (concurrent)
	// requests per container of the Revision.  Defaults to `0` which means
	// unlimited concurrency.
	// +optional
	ContainerConcurrency RevisionContainerConcurrencyType `json:"containerConcurrency,omitempty"`

	// TimeoutSeconds holds the max duration the instance is allowed for
	// responding to a request.  If unspecified, a system default will
	// be provided.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`
}

const (
	// RevisionConditionReady is set when the revision is starting to materialize
	// runtime resources, and becomes true when those resources are ready.
	RevisionConditionReady = apis.ConditionReady
)

// RevisionStatus communicates the observed state of the Revision (from the controller).
type RevisionStatus struct {
	duckv1beta1.Status `json:",inline"`

	// ServiceName holds the name of a core Kubernetes Service resource that
	// load balances over the pods backing this Revision.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// LogURL specifies the generated logging url for this particular revision
	// based on the revision url template specified in the controller's config.
	// +optional
	LogURL string `json:"logUrl,omitempty"`

	// TODO(mattmoor): Revisit the way we resolve tag-to-digest.
	// ImageDigest holds the resolved digest for the image specified
	// within .Spec.Container.Image. The digest is resolved during the creation
	// of Revision. This field holds the digest value regardless of whether
	// a tag or digest was originally specified in the Container object. It
	// may be empty if the image comes from a registry listed to skip resolution.
	// +optional
	// ImageDigest string `json:"imageDigest,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RevisionList is a list of Revision resources
type RevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Revision `json:"items"`
}
