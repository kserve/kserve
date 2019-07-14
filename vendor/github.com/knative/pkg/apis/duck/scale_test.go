/*
Copyright 2018 The Knative Authors

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

package duck_test

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis/duck"
)

type Scalable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScalableSpec   `json:"spec,omitempty"`
	Status ScalableStatus `json:"status,omitempty"`
}

type ScalableSpec struct {
	Replicas *int32                `json:"replicas,omitempty"`
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

type ScalableStatus struct {
	Replicas int32 `json:"replicas,omitempty"`
}

var _ duck.Populatable = (*Scalable)(nil)
var _ duck.Implementable = (*Scalable)(nil)

// GetFullType implements duck.Implementable
func (_ *Scalable) GetFullType() duck.Populatable {
	return &Scalable{}
}

// Populate implements duck.Populatable
func (t *Scalable) Populate() {
	one := int32(1)
	t.Spec = ScalableSpec{
		Replicas: &one,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"foo": "bar",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "baz",
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"blah", "blog"},
			}},
		},
	}
	t.Status = ScalableStatus{
		Replicas: one,
	}
}

func TestImplementsScalable(t *testing.T) {
	instances := []interface{}{
		&Scalable{},
		&appsv1.ReplicaSet{},
		&appsv1.Deployment{},
		&appsv1.StatefulSet{},
	}
	for _, instance := range instances {
		if err := duck.VerifyType(instance, &Scalable{}); err != nil {
			t.Error(err)
		}
	}
}
