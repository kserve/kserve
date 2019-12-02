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
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Configuration represents the "floating HEAD" of a linear history of Revisions.
// Users create new Revisions by updating the Configuration's spec.
// The "latest created" revision's name is available under status, as is the
// "latest ready" revision's name.
// See also: https://knative.dev/serving/blob/master/docs/spec/overview.md#configuration
type Configuration struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec v1.ConfigurationSpec `json:"spec,omitempty"`

	// +optional
	Status v1.ConfigurationStatus `json:"status,omitempty"`
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

const (
	// ConfigurationConditionReady is set when the configuration's latest
	// underlying revision has reported readiness.
	ConfigurationConditionReady = apis.ConditionReady
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigurationList is a list of Configuration resources
type ConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Configuration `json:"items"`
}
