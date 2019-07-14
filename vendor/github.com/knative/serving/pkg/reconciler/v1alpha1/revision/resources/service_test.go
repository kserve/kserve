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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/knative/pkg/ptr"
	"github.com/knative/serving/pkg/apis/autoscaling"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
)

func TestMakeK8sService(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		want *corev1.Service
	}{{
		name: "name is bar and use KPA by default",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
		},
		want: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar-service",
				Labels: map[string]string{
					autoscaling.KPALabelKey:   "bar",
					serving.RevisionLabelKey:  "bar",
					serving.RevisionUID:       "1234",
					AppLabelKey:               "bar",
					networking.ServiceTypeKey: string(networking.ServiceTypePublic),
				},
				Annotations: map[string]string{},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Revision",
					Name:               "bar",
					UID:                "1234",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       ServicePortNameHTTP1,
					Protocol:   corev1.ProtocolTCP,
					Port:       ServicePort,
					TargetPort: intstr.FromString(v1alpha1.RequestQueuePortName),
				}},
				Selector: map[string]string{
					serving.RevisionLabelKey: "bar",
				},
			},
		},
	}, {
		name: "name is baz and use KPA explicitly",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "blah",
				Name:      "baz",
				UID:       "1234",
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey: autoscaling.KPA,
				},
			},
			Spec: v1alpha1.RevisionSpec{
				Container: &corev1.Container{
					Ports: []corev1.ContainerPort{{
						Name: "h2c",
					}},
				},
			},
		},
		want: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "blah",
				Name:      "baz-service",
				Labels: map[string]string{
					autoscaling.KPALabelKey:   "baz",
					serving.RevisionLabelKey:  "baz",
					serving.RevisionUID:       "1234",
					AppLabelKey:               "baz",
					networking.ServiceTypeKey: string(networking.ServiceTypePublic),
				},
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey: autoscaling.KPA,
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Revision",
					Name:               "baz",
					UID:                "1234",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       ServicePortNameH2C,
					Protocol:   corev1.ProtocolTCP,
					Port:       ServicePort,
					TargetPort: intstr.FromString(v1alpha1.RequestQueuePortName),
				}},
				Selector: map[string]string{
					serving.RevisionLabelKey: "baz",
				},
			},
		},
	}, {
		name: "use HPA explicitly",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey: autoscaling.HPA,
				},
			},
		},
		want: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar-service",
				Labels: map[string]string{
					serving.RevisionLabelKey:  "bar",
					serving.RevisionUID:       "1234",
					AppLabelKey:               "bar",
					networking.ServiceTypeKey: string(networking.ServiceTypePublic),
				},
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey: autoscaling.HPA,
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Revision",
					Name:               "bar",
					UID:                "1234",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       ServicePortNameHTTP1,
					Protocol:   corev1.ProtocolTCP,
					Port:       ServicePort,
					TargetPort: intstr.FromString(v1alpha1.RequestQueuePortName),
				}},
				Selector: map[string]string{
					serving.RevisionLabelKey: "bar",
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MakeK8sService(test.rev)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("MakeK8sService (-want, +got) = %v", diff)
			}
		})
	}
}
