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
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Configuration represents the "floating HEAD" of a linear history of Revisions,
// and optionally how the containers those revisions reference are built.
// Users create new Revisions by updating the Configuration's spec.
// The "latest created" revision's name is available under status, as is the
// "latest ready" revision's name.
// See also: https://github.com/knative/serving/blob/master/docs/spec/overview.md#configuration
type Configuration struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Configuration (from the client).
	// +optional
	Spec ConfigurationSpec `json:"spec,omitempty"`

	// Status communicates the observed state of the Configuration (from the controller).
	// +optional
	Status ConfigurationStatus `json:"status,omitempty"`
}

// Verify that Configuration adheres to the appropriate interfaces.
var (
	// Check that Configuration may be validated and defaulted.
	_ apis.Validatable = (*Configuration)(nil)
	_ apis.Defaultable = (*Configuration)(nil)

	// Check that Configuration can be converted to higher versions.
	_ apis.Convertible = (*Configuration)(nil)

	// Check that we can create OwnerReferences to a Configuration.
	_ kmeta.OwnerRefable = (*Configuration)(nil)
)

// ConfigurationSpec holds the desired state of the Configuration (from the client).
type ConfigurationSpec struct {
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

	// Build optionally holds the specification for the build to
	// perform to produce the Revision's container image.
	// +optional
	DeprecatedBuild *runtime.RawExtension `json:"build,omitempty"`

	// DeprecatedRevisionTemplate holds the latest specification for the Revision to
	// be stamped out. If a Build specification is provided, then the
	// DeprecatedRevisionTemplate's BuildName field will be populated with the name of
	// the Build object created to produce the container for the Revision.
	// DEPRECATED Use Template instead.
	// +optional
	DeprecatedRevisionTemplate *RevisionTemplateSpec `json:"revisionTemplate,omitempty"`

	// Template holds the latest specification for the Revision to
	// be stamped out.
	// +optional
	Template *RevisionTemplateSpec `json:"template,omitempty"`
}

const (
	// ConfigurationConditionReady is set when the configuration's latest
	// underlying revision has reported readiness.
	ConfigurationConditionReady = apis.ConditionReady
)

// ConfigurationStatusFields holds all of the non-duckv1beta1.Status status fields of a Route.
// These are defined outline so that we can also inline them into Service, and more easily
// copy them.
type ConfigurationStatusFields struct {
	// LatestReadyRevisionName holds the name of the latest Revision stamped out
	// from this Configuration that has had its "Ready" condition become "True".
	// +optional
	LatestReadyRevisionName string `json:"latestReadyRevisionName,omitempty"`

	// LatestCreatedRevisionName is the last revision that was created from this
	// Configuration. It might not be ready yet, for that use LatestReadyRevisionName.
	// +optional
	LatestCreatedRevisionName string `json:"latestCreatedRevisionName,omitempty"`
}

// ConfigurationStatus communicates the observed state of the Configuration (from the controller).
type ConfigurationStatus struct {
	duckv1beta1.Status `json:",inline"`

	ConfigurationStatusFields `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigurationList is a list of Configuration resources
type ConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Configuration `json:"items"`
}
