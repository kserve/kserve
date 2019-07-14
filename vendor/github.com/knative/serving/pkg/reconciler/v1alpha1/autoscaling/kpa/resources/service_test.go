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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/knative/pkg/ptr"
	pav1a1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

func TestMakeService(t *testing.T) {
	pa := &pav1a1.PodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "here",
			Name:      "with-you",
			UID:       "2006",
			// Those labels are propagated from the Revision->KPA.
			Labels: map[string]string{
				serving.RevisionLabelKey: "with-you",
				serving.RevisionUID:      "2009",
			},
			Annotations: map[string]string{
				"a": "b",
			},
		},
		Spec: pav1a1.PodAutoscalerSpec{
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "with-you",
			},
			ServiceName: "with-you-service",
		},
	}
	selector := map[string]string{"cant": "stop"}
	want := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "here",
			Name:      "with-you-metrics",
			Labels: map[string]string{
				// Those should be propagated.
				serving.RevisionLabelKey:  "with-you",
				serving.RevisionUID:       "2009",
				kpaLabelKey:               "with-you",
				networking.ServiceTypeKey: string(networking.ServiceTypeMetrics),
			},
			Annotations: map[string]string{
				"a": "b",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         pav1a1.SchemeGroupVersion.String(),
				Kind:               "PodAutoscaler",
				Name:               "with-you",
				UID:                "2006",
				Controller:         ptr.Bool(true),
				BlockOwnerDeletion: ptr.Bool(true),
			}},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "metrics",
				Protocol:   corev1.ProtocolTCP,
				Port:       9090,
				TargetPort: intstr.FromString("queue-metrics"),
			}},
			Selector: selector,
		},
	}
	got := MakeMetricsService(pa, selector)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Metrics K8s Service mismatch (-want, +got) = %v", diff)
	}
}
