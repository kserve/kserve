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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis/duck"
)

// PodSpecable is implemented by types containing a PodTemplateSpec
// in the manner of ReplicaSet, Deployment, DaemonSet, StatefulSet.
type PodSpecable corev1.PodTemplateSpec

type WithPod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WithPodSpec `json:"spec,omitempty"`
}

type WithPodSpec struct {
	Template PodSpecable `json:"template,omitempty"`
}

var _ duck.Populatable = (*WithPod)(nil)
var _ duck.Implementable = (*PodSpecable)(nil)

// GetFullType implements duck.Implementable
func (_ *PodSpecable) GetFullType() duck.Populatable {
	return &WithPod{}
}

// Populate implements duck.Populatable
func (t *WithPod) Populate() {
	t.Spec.Template = PodSpecable{
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
	}
}

func TestImplementsPodSpecable(t *testing.T) {
	instances := []interface{}{
		&WithPod{},
		&appsv1.ReplicaSet{},
		&appsv1.Deployment{},
		&appsv1.StatefulSet{},
		&appsv1.DaemonSet{},
		&batchv1.Job{},
	}
	for _, instance := range instances {
		if err := duck.VerifyType(instance, &PodSpecable{}); err != nil {
			t.Error(err)
		}
	}
}
