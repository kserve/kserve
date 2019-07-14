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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/kmeta"
	netv1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
)

var (
	r = &v1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-route",
			Namespace: "test-ns",
		},
	}
	expectedMeta = metav1.ObjectMeta{
		Name:      "test-route",
		Namespace: "test-ns",
		OwnerReferences: []metav1.OwnerReference{
			*kmeta.NewControllerRef(r),
		},
	}
)

func TestNewMakeK8SService(t *testing.T) {
	scenarios := map[string]struct {
		// Inputs
		route        *v1alpha1.Route
		ingress      *netv1alpha1.ClusterIngress
		expectedSpec corev1.ServiceSpec
		shouldFail   bool
	}{
		"no-loadbalancer": {
			route: r,
			ingress: &netv1alpha1.ClusterIngress{
				Status: netv1alpha1.IngressStatus{},
			},
			shouldFail: true,
		},
		"empty-loadbalancer": {
			route: r,
			ingress: &netv1alpha1.ClusterIngress{
				Status: netv1alpha1.IngressStatus{
					LoadBalancer: &netv1alpha1.LoadBalancerStatus{
						Ingress: []netv1alpha1.LoadBalancerIngressStatus{{}},
					},
				},
			},
			shouldFail: true,
		},
		"multi-loadbalancer": {
			route: r,
			ingress: &netv1alpha1.ClusterIngress{
				Status: netv1alpha1.IngressStatus{
					LoadBalancer: &netv1alpha1.LoadBalancerStatus{
						Ingress: []netv1alpha1.LoadBalancerIngressStatus{{
							Domain: "domain.com",
						}, {
							DomainInternal: "domain.com",
						}},
					},
				},
			},
			shouldFail: true,
		},
		"ingress-with-domain": {
			route: r,
			ingress: &netv1alpha1.ClusterIngress{
				Status: netv1alpha1.IngressStatus{
					LoadBalancer: &netv1alpha1.LoadBalancerStatus{
						Ingress: []netv1alpha1.LoadBalancerIngressStatus{{Domain: "domain.com"}},
					},
				},
			},
			expectedSpec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "domain.com",
			},
		},
		"ingress-with-domaininternal": {
			route: r,
			ingress: &netv1alpha1.ClusterIngress{
				Status: netv1alpha1.IngressStatus{
					LoadBalancer: &netv1alpha1.LoadBalancerStatus{
						Ingress: []netv1alpha1.LoadBalancerIngressStatus{{DomainInternal: "istio-ingressgateway.istio-system.svc.cluster.local"}},
					},
				},
			},
			expectedSpec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "istio-ingressgateway.istio-system.svc.cluster.local",
			},
		},
		"ingress-with-only-mesh": {
			route: r,
			ingress: &netv1alpha1.ClusterIngress{
				Status: netv1alpha1.IngressStatus{
					LoadBalancer: &netv1alpha1.LoadBalancerStatus{
						Ingress: []netv1alpha1.LoadBalancerIngressStatus{{MeshOnly: true}},
					},
				},
			},
			expectedSpec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{{
					Name: "http",
					Port: 80,
				}},
			},
		},
	}

	for name, scenario := range scenarios {
		service, err := MakeK8sService(scenario.route, scenario.ingress)
		// Validate
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(expectedMeta, service.ObjectMeta); diff != "" {
				t.Errorf("Unexpected Metadata  (-want +got): %v", diff)
			}
			if diff := cmp.Diff(scenario.expectedSpec, service.Spec); diff != "" {
				t.Errorf("Unexpected ServiceSpec (-want +got): %v", diff)
			}
		}
	}
}
