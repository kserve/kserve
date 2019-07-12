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
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/serverlessservice/resources/names"
)

var (
	goodPod = "good-pod"
	badPod  = "bad-pod"
)

func TestMakeService(t *testing.T) {
	tests := []struct {
		name     string
		sks      *v1alpha1.ServerlessService
		selector map[string]string
		want     *corev1.Service
	}{{
		name: "HTTP",
		sks: &v1alpha1.ServerlessService{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "melon",
				Name:      "collie",
				UID:       "1982",
				// Those labels are propagated from the Revision->KPA.
				Labels: map[string]string{
					serving.RevisionLabelKey: "collie",
					serving.RevisionUID:      "1982",
				},
			},
			Spec: v1alpha1.ServerlessServiceSpec{
				ProtocolType: networking.ProtocolHTTP1,
			},
		},
		selector: map[string]string{
			"app": "sadness",
		},
		want: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "melon",
				Name:      "collie-pub",
				Labels: map[string]string{
					// Those should be propagated.
					serving.RevisionLabelKey:  "collie",
					serving.RevisionUID:       "1982",
					networking.SKSLabelKey:    "collie",
					networking.ServiceTypeKey: "Public",
				},
				Annotations: map[string]string{},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "ServerlessService",
					Name:               "collie",
					UID:                "1982",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       servicePortNameHTTP1,
					Protocol:   corev1.ProtocolTCP,
					Port:       servicePort,
					TargetPort: intstr.FromString(requestQueuePortName),
				}},
			},
		},
	}, {
		name: "HTTP2",
		sks: &v1alpha1.ServerlessService{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "siamese",
				Name:      "dream",
				UID:       "1988",
				// Those labels are propagated from the Revision->KPA.
				Labels: map[string]string{
					serving.RevisionLabelKey: "dream",
					serving.RevisionUID:      "1988",
				},
				Annotations: map[string]string{
					"cherub": "rock",
				},
			},
			Spec: v1alpha1.ServerlessServiceSpec{
				ProtocolType: networking.ProtocolH2C,
			},
		},
		selector: map[string]string{
			"app": "today",
		},
		want: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "siamese",
				Name:      "dream-pub",
				Labels: map[string]string{
					// Those should be propagated.
					serving.RevisionLabelKey:  "dream",
					serving.RevisionUID:       "1988",
					networking.SKSLabelKey:    "dream",
					networking.ServiceTypeKey: "Public",
				},
				Annotations: map[string]string{
					"cherub": "rock",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "ServerlessService",
					Name:               "dream",
					UID:                "1988",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       servicePortNameH2C,
					Protocol:   corev1.ProtocolTCP,
					Port:       servicePort,
					TargetPort: intstr.FromString(requestQueuePortName),
				}},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MakePublicService(test.sks)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Public K8s Service mismatch (-want, +got) = %v", diff)
			}
			// Now let's patch selector.
			test.want.Spec.Selector = test.selector
			test.want.Name = names.PrivateService(test.sks.Name)
			test.want.Labels[networking.ServiceTypeKey] = "Private"

			got = MakePrivateService(test.sks, test.selector)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Private K8s Service mismatch (-want, +got) = %v", diff)
			}
		})
	}
}

func TestMakeEndpoints(t *testing.T) {
	tests := []struct {
		name string
		sks  *v1alpha1.ServerlessService
		eps  *corev1.Endpoints
		want *corev1.Endpoints
	}{{
		name: "empty source",
		sks: &v1alpha1.ServerlessService{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "melon",
				Name:      "collie",
				UID:       "1982",
				// Those labels are propagated from the Revision->KPA.
				Labels: map[string]string{
					serving.RevisionLabelKey: "collie",
					serving.RevisionUID:      "1982",
				},
				Annotations: map[string]string{
					"cherub": "rock",
				},
			},
			Spec: v1alpha1.ServerlessServiceSpec{
				ProtocolType: networking.ProtocolHTTP1,
			},
		},
		eps: &corev1.Endpoints{},
		want: &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "melon",
				Name:      "collie-pub",
				Labels: map[string]string{
					serving.RevisionLabelKey:  "collie",
					serving.RevisionUID:       "1982",
					networking.SKSLabelKey:    "collie",
					networking.ServiceTypeKey: "Public",
				},
				Annotations: map[string]string{
					"cherub": "rock",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "ServerlessService",
					Name:               "collie",
					UID:                "1982",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
		},
	}, {
		name: "some endpoints",
		sks: &v1alpha1.ServerlessService{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "melon",
				Name:      "collie",
				UID:       "1982",
				// Those labels are propagated from the Revision->KPA.
				Labels: map[string]string{
					serving.RevisionLabelKey:  "collie",
					serving.RevisionUID:       "1982",
					networking.ServiceTypeKey: "Public",
				},
				Annotations: map[string]string{
					"cherub": "rock",
				},
			},
			Spec: v1alpha1.ServerlessServiceSpec{
				ProtocolType: networking.ProtocolHTTP1,
			},
		},
		eps: &corev1.Endpoints{
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{
					IP:       "192.168.1.1",
					NodeName: &goodPod,
				}, {
					IP:       "10.5.6.21",
					NodeName: &badPod,
				}},
				Ports: []corev1.EndpointPort{{
					Name:     "http",
					Port:     8012,
					Protocol: "TCP",
				}},
			}},
		},
		want: &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "melon",
				Name:      "collie-pub",
				Labels: map[string]string{
					serving.RevisionLabelKey:  "collie",
					serving.RevisionUID:       "1982",
					networking.SKSLabelKey:    "collie",
					networking.ServiceTypeKey: "Public",
				},
				Annotations: map[string]string{
					"cherub": "rock",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "ServerlessService",
					Name:               "collie",
					UID:                "1982",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{
					IP:       "192.168.1.1",
					NodeName: &goodPod,
				}, {
					IP:       "10.5.6.21",
					NodeName: &badPod,
				}},
				Ports: []corev1.EndpointPort{{
					Name:     "http",
					Port:     8012,
					Protocol: "TCP",
				}},
			}},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MakePublicEndpoints(test.sks, test.eps)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Public K8s Endpoints mismatch (-want, +got) = %v", diff)
			}
		})
	}
}
