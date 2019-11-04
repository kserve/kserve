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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"
)

// Metric represents a resource to configure the metric collector with.
//
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Metric struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Metric (from the client).
	// +optional
	Spec MetricSpec `json:"spec,omitempty"`

	// Status communicates the observed state of the Metric (from the controller).
	// +optional
	Status MetricStatus `json:"status,omitempty"`
}

// Verify that Metric adheres to the appropriate interfaces.
var (
	// Check that Metric can be validated and can be defaulted.
	_ apis.Validatable = (*Metric)(nil)
	_ apis.Defaultable = (*Metric)(nil)

	// Check that we can create OwnerReferences to a Metric.
	_ kmeta.OwnerRefable = (*Metric)(nil)
)

// MetricSpec contains all values a metric collector needs to operate.
type MetricSpec struct {
	// StableWindow is the aggregation window for metrics in a stable state.
	StableWindow time.Duration `json:"stableWindow"`
	// PanicWindow is the aggregation window for metrics where quick reactions are needed.
	PanicWindow time.Duration `json:"panicWindow"`
	// ScrapeTarget is the K8s service that publishes the metric endpoint.
	ScrapeTarget string `json:"scrapeTarget"`
}

// MetricStatus reflects the status of metric collection for this specific entity.
type MetricStatus struct {
	duckv1.Status `json:",inline"`
}

// MetricList is a list of Metric resources
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Metric `json:"items"`
}
