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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Route is responsible for configuring ingress over a collection of Revisions.
// Some of the Revisions a Route distributes traffic over may be specified by
// referencing the Configuration responsible for creating them; in these cases
// the Route is additionally responsible for monitoring the Configuration for
// "latest ready revision" changes, and smoothly rolling out latest revisions.
// See also: https://github.com/knative/serving/blob/master/docs/spec/overview.md#route
type Route struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Route (from the client).
	// +optional
	Spec RouteSpec `json:"spec,omitempty"`

	// Status communicates the observed state of the Route (from the controller).
	// +optional
	Status RouteStatus `json:"status,omitempty"`
}

// Verify that Route adheres to the appropriate interfaces.
var (
	// Check that Route may be validated and defaulted.
	_ apis.Validatable = (*Route)(nil)
	_ apis.Defaultable = (*Route)(nil)

	// Check that Route can be converted to higher versions.
	_ apis.Convertible = (*Route)(nil)

	// Check that we can create OwnerReferences to a Route.
	_ kmeta.OwnerRefable = (*Route)(nil)
)

// TrafficTarget holds a single entry of the routing table for a Route.
type TrafficTarget struct {
	// Tag is optionally used to expose a dedicated url for referencing
	// this target exclusively.
	// +optional
	// TODO(mattmoor): Discuss alternative naming options.
	Tag string `json:"tag,omitempty"`

	// RevisionName of a specific revision to which to send this portion of
	// traffic.  This is mutually exclusive with ConfigurationName.
	// +optional
	RevisionName string `json:"revisionName,omitempty"`

	// ConfigurationName of a configuration to whose latest revision we will send
	// this portion of traffic. When the "status.latestReadyRevisionName" of the
	// referenced configuration changes, we will automatically migrate traffic
	// from the prior "latest ready" revision to the new one.  This field is never
	// set in Route's status, only its spec.  This is mutually exclusive with
	// RevisionName.
	// +optional
	ConfigurationName string `json:"configurationName,omitempty"`

	// LatestRevision may be optionally provided to indicate that the latest
	// ready Revision of the Configuration should be used for this traffic
	// target.  When provided LatestRevision must be true if RevisionName is
	// empty; it must be false when RevisionName is non-empty.
	// +optional
	LatestRevision *bool `json:"latestRevision,omitempty"`

	// Percent specifies percent of the traffic to this Revision or Configuration.
	// This defaults to zero if unspecified.
	// +optional
	Percent int `json:"percent"`

	// URL displays the URL for accessing named traffic targets. URL is displayed in
	// status, and is disallowed on spec. URL must contain a scheme (e.g. http://) and
	// a hostname, but may not contain anything else (e.g. basic auth, url path, etc.)
	// +optional
	URL *apis.URL `json:"url,omitempty"`
}

// RouteSpec holds the desired state of the Route (from the client).
type RouteSpec struct {
	// Traffic specifies how to distribute traffic over a collection of
	// revisions and configurations.
	// +optional
	Traffic []TrafficTarget `json:"traffic,omitempty"`
}

const (
	// RouteConditionReady is set when the service is configured
	// and has available backends ready to receive traffic.
	RouteConditionReady = apis.ConditionReady
)

// RouteStatusFields holds the fields of Route's status that
// are not generally shared.  This is defined separately and inlined so that
// other types can readily consume these fields via duck typing.
type RouteStatusFields struct {
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}
	// +optional
	URL *apis.URL `json:"url,omitempty"`

	// Address holds the information needed for a Route to be the target of an event.
	// +optional
	Address *duckv1beta1.Addressable `json:"address,omitempty"`

	// Traffic holds the configured traffic distribution.
	// These entries will always contain RevisionName references.
	// When ConfigurationName appears in the spec, this will hold the
	// LatestReadyRevisionName that we last observed.
	// +optional
	Traffic []TrafficTarget `json:"traffic,omitempty"`
}

// RouteStatus communicates the observed state of the Route (from the controller).
type RouteStatus struct {
	duckv1beta1.Status `json:",inline"`

	RouteStatusFields `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteList is a list of Route resources
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Route `json:"items"`
}
