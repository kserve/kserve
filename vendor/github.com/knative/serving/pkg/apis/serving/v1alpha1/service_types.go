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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Service acts as a top-level container that manages a set of Routes and
// Configurations which implement a network service. Service exists to provide a
// singular abstraction which can be access controlled, reasoned about, and
// which encapsulates software lifecycle decisions such as rollout policy and
// team resource ownership. Service acts only as an orchestrator of the
// underlying Routes and Configurations (much as a kubernetes Deployment
// orchestrates ReplicaSets), and its usage is optional but recommended.
//
// The Service's controller will track the statuses of its owned Configuration
// and Route, reflecting their statuses and conditions as its own.
//
// See also: https://github.com/knative/serving/blob/master/docs/spec/overview.md#service
type Service struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Spec ServiceSpec `json:"spec,omitempty"`
	// +optional
	Status ServiceStatus `json:"status,omitempty"`
}

// Verify that Service adheres to the appropriate interfaces.
var (
	// Check that Service may be validated and defaulted.
	_ apis.Validatable = (*Service)(nil)
	_ apis.Defaultable = (*Service)(nil)

	// Check that Route can be converted to higher versions.
	_ apis.Convertible = (*Service)(nil)

	// Check that we can create OwnerReferences to a Service.
	_ kmeta.OwnerRefable = (*Service)(nil)
)

// ServiceSpec represents the configuration for the Service object. Exactly one
// of its members (other than Generation) must be specified. Services can either
// track the latest ready revision of a configuration or be pinned to a specific
// revision.
type ServiceSpec struct {
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

	// DeprecatedRunLatest defines a simple Service. It will automatically
	// configure a route that keeps the latest ready revision
	// from the supplied configuration running.
	// +optional
	DeprecatedRunLatest *RunLatestType `json:"runLatest,omitempty"`

	// DeprecatedPinned is DEPRECATED in favor of ReleaseType
	// +optional
	DeprecatedPinned *PinnedType `json:"pinned,omitempty"`

	// DeprecatedManual mode enables users to start managing the underlying Route and Configuration
	// resources directly.  This advanced usage is intended as a path for users to graduate
	// from the limited capabilities of Service to the full power of Route.
	// +optional
	DeprecatedManual *ManualType `json:"manual,omitempty"`

	// Release enables gradual promotion of new revisions by allowing traffic
	// to be split between two revisions. This type replaces the deprecated Pinned type.
	// +optional
	DeprecatedRelease *ReleaseType `json:"release,omitempty"`

	// We are moving to a shape where the Configuration and Route specifications
	// are inlined into the Service, which gives them compatible shapes.  We are
	// staging this change here as a path to this in v1beta1, which drops the
	// "mode" based specifications above.  Ultimately all non-v1beta1 fields will
	// be deprecated, and then dropped in v1beta1.
	ConfigurationSpec `json:",inline"`
	RouteSpec         `json:",inline"`
}

// ManualType contains the options for configuring a manual service. See ServiceSpec for
// more details.
type ManualType struct {
	// DeprecatedManual type does not contain a configuration as this type provides the
	// user complete control over the configuration and route.
}

// ReleaseType contains the options for slowly releasing revisions. See ServiceSpec for
// more details.
type ReleaseType struct {
	// Revisions is an ordered list of 1 or 2 revisions. The first will
	// have a TrafficTarget with a name of "current" and the second will have
	// a name of "candidate".
	// +optional
	Revisions []string `json:"revisions,omitempty"`

	// RolloutPercent is the percent of traffic that should be sent to the "candidate"
	// revision. Valid values are between 0 and 99 inclusive.
	// +optional
	RolloutPercent int `json:"rolloutPercent,omitempty"`

	// The configuration for this service. All revisions from this service must
	// come from a single configuration.
	// +optional
	Configuration ConfigurationSpec `json:"configuration,omitempty"`
}

// ReleaseLatestRevisionKeyword is a shortcut for usage in the `release` mode
// to refer to the latest created revision.
// See #2819 for details.
const ReleaseLatestRevisionKeyword = "@latest"

// RunLatestType contains the options for always having a route to the latest configuration. See
// ServiceSpec for more details.
type RunLatestType struct {
	// The configuration for this service.
	// +optional
	Configuration ConfigurationSpec `json:"configuration,omitempty"`
}

// PinnedType is DEPRECATED. ReleaseType should be used instead. To get the behavior of PinnedType set
// ReleaseType.Revisions to []string{PinnedType.RevisionName} and ReleaseType.RolloutPercent to 0.
type PinnedType struct {
	// The revision name to pin this service to until changed
	// to a different service type.
	// +optional
	RevisionName string `json:"revisionName,omitempty"`

	// The configuration for this service.
	// +optional
	Configuration ConfigurationSpec `json:"configuration,omitempty"`
}

// ConditionType represents a Service condition value
const (
	// ServiceConditionReady is set when the service is configured
	// and has available backends ready to receive traffic.
	ServiceConditionReady = apis.ConditionReady
	// ServiceConditionRoutesReady is set when the service's underlying
	// routes have reported readiness.
	ServiceConditionRoutesReady apis.ConditionType = "RoutesReady"
	// ServiceConditionConfigurationsReady is set when the service's underlying
	// configurations have reported readiness.
	ServiceConditionConfigurationsReady apis.ConditionType = "ConfigurationsReady"
)

// ServiceStatus represents the Status stanza of the Service resource.
type ServiceStatus struct {
	duckv1beta1.Status `json:",inline"`

	RouteStatusFields `json:",inline"`

	ConfigurationStatusFields `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceList is a list of Service resources
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Service `json:"items"`
}
