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
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/kmeta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Revision is an immutable snapshot of code and configuration.  A revision
// references a container image, and optionally a build that is responsible for
// materializing that container image from source. Revisions are created by
// updates to a Configuration.
//
// See also: https://github.com/knative/serving/blob/master/docs/spec/overview.md#revision
type Revision struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Revision (from the client).
	// +optional
	Spec RevisionSpec `json:"spec,omitempty"`

	// Status communicates the observed state of the Revision (from the controller).
	// +optional
	Status RevisionStatus `json:"status,omitempty"`
}

// Verify that Revision adheres to the appropriate interfaces.
var (
	// Check that Revision can be validated, can be defaulted, and has immutable fields.
	_ apis.Validatable = (*Revision)(nil)
	_ apis.Defaultable = (*Revision)(nil)
	_ apis.Immutable   = (*Revision)(nil)

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

// DeprecatedRevisionServingStateType is an enumeration of the levels of serving readiness of the Revision.
// See also: https://github.com/knative/serving/blob/master/docs/spec/errors.md#error-conditions-and-reporting
type DeprecatedRevisionServingStateType string

const (
	// The revision is ready to serve traffic. It should have Kubernetes
	// resources, and the Istio route should be pointed to the given resources.
	DeprecatedRevisionServingStateActive DeprecatedRevisionServingStateType = "Active"
	// The revision is not currently serving traffic, but could be made to serve
	// traffic quickly. It should have Kubernetes resources, but the Istio route
	// should be pointed to the activator.
	DeprecatedRevisionServingStateReserve DeprecatedRevisionServingStateType = "Reserve"
	// The revision has been decommissioned and is not needed to serve traffic
	// anymore. It should not have any Istio routes or Kubernetes resources.
	// A Revision may be brought out of retirement, but it may take longer than
	// it would from a "Reserve" state.
	// Note: currently not set anywhere. See https://github.com/knative/serving/issues/1203
	DeprecatedRevisionServingStateRetired DeprecatedRevisionServingStateType = "Retired"
)

// RevisionRequestConcurrencyModelType is an enumeration of the
// concurrency models supported by a Revision.
// Deprecated in favor of RevisionContainerConcurrencyType.
type RevisionRequestConcurrencyModelType string

const (
	// RevisionRequestConcurrencyModelSingle guarantees that only one
	// request will be handled at a time (concurrently) per instance
	// of Revision Container.
	RevisionRequestConcurrencyModelSingle RevisionRequestConcurrencyModelType = "Single"
	// RevisionRequestConcurencyModelMulti allows more than one request to
	// be handled at a time (concurrently) per instance of Revision
	// Container.
	RevisionRequestConcurrencyModelMulti RevisionRequestConcurrencyModelType = "Multi"
)

// RevisionContainerConcurrencyType is an integer expressing a number of
// in-flight (concurrent) requests.
type RevisionContainerConcurrencyType int64

const (
	// The maximum configurable container concurrency.
	RevisionContainerConcurrencyMax RevisionContainerConcurrencyType = 1000
)

// RevisionSpec holds the desired state of the Revision (from the client).
type RevisionSpec struct {
	// DeprecatedGeneration was used prior in Kubernetes versions <1.11
	// when metadata.generation was not being incremented by the api server
	//
	// This property will be dropped in future Knative releases and should
	// not be used - use metadata.generation
	//
	// Tracking issue: https://github.com/knative/serving/issues/643
	//
	// +optional
	DeprecatedGeneration int64 `json:"generation,omitempty"`

	// DeprecatedServingState holds a value describing the desired state the Kubernetes
	// resources should be in for this Revision.
	// Users must not specify this when creating a revision. These values are no longer
	// updated by the system.
	// +optional
	DeprecatedServingState DeprecatedRevisionServingStateType `json:"servingState,omitempty"`

	// DeprecatedConcurrencyModel specifies the desired concurrency model
	// (Single or Multi) for the
	// Revision. Defaults to Multi.
	// Deprecated in favor of ContainerConcurrency.
	// +optional
	DeprecatedConcurrencyModel RevisionRequestConcurrencyModelType `json:"concurrencyModel,omitempty"`

	// ContainerConcurrency specifies the maximum allowed
	// in-flight (concurrent) requests per container of the Revision.
	// Defaults to `0` which means unlimited concurrency.
	// This field replaces ConcurrencyModel. A value of `1`
	// is equivalent to `Single` and `0` is equivalent to `Multi`.
	// +optional
	ContainerConcurrency RevisionContainerConcurrencyType `json:"containerConcurrency,omitempty"`

	// ServiceAccountName holds the name of the Kubernetes service account
	// as which the underlying K8s resources should be run. If unspecified
	// this will default to the "default" service account for the namespace
	// in which the Revision exists.
	// This may be used to provide access to private container images by
	// following: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account
	// TODO(ZhiminXiang): verify the corresponding service account exists.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// DeprecatedBuildName optionally holds the name of the Build responsible for
	// producing the container image for its Revision.
	// DEPRECATED: Use BuildRef instead.
	// +optional
	DeprecatedBuildName string `json:"buildName,omitempty"`

	// BuildRef holds the reference to the build (if there is one) responsible
	// for producing the container image for this Revision. Otherwise, nil
	// +optional
	BuildRef *corev1.ObjectReference `json:"buildRef,omitempty"`

	// Container defines the unit of execution for this Revision.
	// In the context of a Revision, we disallow a number of the fields of
	// this Container, including: name and lifecycle.
	// See also the runtime contract for more information about the execution
	// environment:
	// https://github.com/knative/serving/blob/master/docs/runtime-contract.md
	// +optional
	Container corev1.Container `json:"container,omitempty"`

	// Volumes defines a set of Kubernetes volumes to be mounted into the
	// specified Container.  Currently only ConfigMap and Secret volumes are
	// supported.
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// TimeoutSeconds holds the max duration the instance is allowed for responding to a request.
	// +optional
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`
}

const (
	// RevisionConditionReady is set when the revision is starting to materialize
	// runtime resources, and becomes true when those resources are ready.
	RevisionConditionReady = duckv1alpha1.ConditionReady
	// RevisionConditionBuildSucceeded is set when the revision has an associated build
	// and is marked True if/once the Build has completed successfully.
	RevisionConditionBuildSucceeded duckv1alpha1.ConditionType = "BuildSucceeded"
	// RevisionConditionResourcesAvailable is set when underlying
	// Kubernetes resources have been provisioned.
	RevisionConditionResourcesAvailable duckv1alpha1.ConditionType = "ResourcesAvailable"
	// RevisionConditionContainerHealthy is set when the revision readiness check completes.
	RevisionConditionContainerHealthy duckv1alpha1.ConditionType = "ContainerHealthy"
	// RevisionConditionActive is set when the revision is receiving traffic.
	RevisionConditionActive duckv1alpha1.ConditionType = "Active"
)

// RevisionStatus communicates the observed state of the Revision (from the controller).
type RevisionStatus struct {
	duckv1alpha1.Status `json:",inline"`

	// ServiceName holds the name of a core Kubernetes Service resource that
	// load balances over the pods backing this Revision. When the Revision
	// is Active, this service would be an appropriate ingress target for
	// targeting the revision.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// LogURL specifies the generated logging url for this particular revision
	// based on the revision url template specified in the controller's config.
	// +optional
	LogURL string `json:"logUrl,omitempty"`

	// ImageDigest holds the resolved digest for the image specified
	// within .Spec.Container.Image. The digest is resolved during the creation
	// of Revision. This field holds the digest value regardless of whether
	// a tag or digest was originally specified in the Container object. It
	// may be empty if the image comes from a registry listed to skip resolution.
	// +optional
	ImageDigest string `json:"imageDigest,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RevisionList is a list of Revision resources
type RevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Revision `json:"items"`
}
