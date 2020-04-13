/*
Copyright 2019 The Knative Authors

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
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck"
)

// +genduck
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodScalable is a duck type that the resources referenced by the
// PodAutoscaler's ScaleTargetRef must implement.  They must also
// implement the `/scale` sub-resource for use with `/scale` based
// implementations (e.g. HPA), but this further constrains the shape
// the referenced resources may take.
type PodScalable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodScalableSpec   `json:"spec"`
	Status PodScalableStatus `json:"status"`
}

// PodScalableSpec is the specification for the desired state of a
// PodScalable (or at least our shared portion).
type PodScalableSpec struct {
	Replicas *int32                 `json:"replicas,omitempty"`
	Selector *metav1.LabelSelector  `json:"selector"`
	Template corev1.PodTemplateSpec `json:"template"`
}

// PodScalableStatus is the observed state of a PodScalable (or at
// least our shared portion).
type PodScalableStatus struct {
	Replicas int32 `json:"replicas,omitempty"`
}

var _ duck.Populatable = (*PodScalable)(nil)
var _ duck.Implementable = (*PodScalable)(nil)
var _ apis.Listable = (*PodScalable)(nil)

// GetFullType implements duck.Implementable
func (*PodScalable) GetFullType() duck.Populatable {
	return &PodScalable{}
}

// Populate implements duck.Populatable
func (t *PodScalable) Populate() {
	twelve := int32(12)
	t.Spec = PodScalableSpec{
		Replicas: &twelve,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"foo": "bar",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "foo",
				Operator: "In",
				Values:   []string{"baz", "blah"},
			}},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "container-name",
					Image: "container-image:latest",
				}},
			},
		},
	}
	t.Status = PodScalableStatus{
		Replicas: 42,
	}
}

// GetListType implements apis.Listable
func (*PodScalable) GetListType() runtime.Object {
	return &PodScalableList{}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodScalableList is a list of PodScalable resources
type PodScalableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []PodScalable `json:"items"`
}
