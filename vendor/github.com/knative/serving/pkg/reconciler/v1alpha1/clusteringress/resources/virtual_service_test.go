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
	istiov1alpha1 "github.com/knative/pkg/apis/istio/common/v1alpha1"
	"github.com/knative/pkg/apis/istio/v1alpha3"
	"github.com/knative/pkg/kmeta"
	"github.com/knative/pkg/system"
	_ "github.com/knative/pkg/system/testing"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestMakeVirtualServiceSpec_CorrectMetadata(t *testing.T) {
	ci := &v1alpha1.ClusterIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ingress",
			Labels: map[string]string{
				serving.RouteLabelKey:          "test-route",
				serving.RouteNamespaceLabelKey: "test-ns",
			},
		},
		Spec: v1alpha1.IngressSpec{},
	}
	expected := metav1.ObjectMeta{
		Name:      "test-ingress",
		Namespace: system.Namespace(),
		Labels: map[string]string{
			networking.IngressLabelKey:     "test-ingress",
			serving.RouteLabelKey:          "test-route",
			serving.RouteNamespaceLabelKey: "test-ns",
		},
		OwnerReferences: []metav1.OwnerReference{
			*kmeta.NewControllerRef(ci),
		},
	}
	meta := MakeVirtualService(ci, []string{}).ObjectMeta
	if diff := cmp.Diff(expected, meta); diff != "" {
		t.Errorf("Unexpected metadata (-want +got): %v", diff)
	}
}

func TestMakeVirtualServiceSpec_CorrectGateways(t *testing.T) {
	ci := &v1alpha1.ClusterIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ingress",
			Labels: map[string]string{
				serving.RouteLabelKey:          "test-route",
				serving.RouteNamespaceLabelKey: "test-ns",
			},
		},
		Spec: v1alpha1.IngressSpec{},
	}
	expected := []string{"gateway-one", "gateway-two", "mesh"}
	gateways := MakeVirtualService(ci, []string{"gateway-one", "gateway-two"}).Spec.Gateways
	if diff := cmp.Diff(expected, gateways); diff != "" {
		t.Errorf("Unexpected gateways (-want +got): %v", diff)
	}
}

func TestMakeVirtualServiceSpec_CorrectRoutes(t *testing.T) {
	ci := &v1alpha1.ClusterIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ingress",
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.ClusterIngressRule{{
				Hosts: []string{
					"domain.com",
					"test-route.test-ns.svc.cluster.local",
					"test-route.test-ns.svc",
					"test-route.test-ns",
				},
				HTTP: &v1alpha1.HTTPClusterIngressRuleValue{
					Paths: []v1alpha1.HTTPClusterIngressPath{{
						Path: "^/pets/(.*?)?",
						Splits: []v1alpha1.ClusterIngressBackendSplit{{
							ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
								ServiceNamespace: "test-ns",
								ServiceName:      "v2-service",
								ServicePort:      intstr.FromInt(80),
							},
							Percent: 100,
							AppendHeaders: map[string]string{
								"ugh": "blah",
							},
						}},
						AppendHeaders: map[string]string{
							"foo": "bar",
						},
						Timeout: &metav1.Duration{Duration: networking.DefaultTimeout},
						Retries: &v1alpha1.HTTPRetry{
							PerTryTimeout: &metav1.Duration{Duration: networking.DefaultTimeout},
							Attempts:      networking.DefaultRetryCount,
						},
					}},
				},
			}, {
				Hosts: []string{
					"v1.domain.com",
				},
				HTTP: &v1alpha1.HTTPClusterIngressRuleValue{
					Paths: []v1alpha1.HTTPClusterIngressPath{{
						Path: "^/pets/(.*?)?",
						Splits: []v1alpha1.ClusterIngressBackendSplit{{
							ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
								ServiceNamespace: "test-ns",
								ServiceName:      "v1-service",
								ServicePort:      intstr.FromInt(80),
							},
							Percent: 100,
						}},
						AppendHeaders: map[string]string{
							"foo": "baz",
						},
						Timeout: &metav1.Duration{Duration: networking.DefaultTimeout},
						Retries: &v1alpha1.HTTPRetry{
							PerTryTimeout: &metav1.Duration{Duration: networking.DefaultTimeout},
							Attempts:      networking.DefaultRetryCount,
						},
					}},
				},
			}},
		},
	}

	expected := []v1alpha3.HTTPRoute{{
		Match: []v1alpha3.HTTPMatchRequest{{
			URI:       &istiov1alpha1.StringMatch{Regex: "^/pets/(.*?)?"},
			Authority: &istiov1alpha1.StringMatch{Regex: `^domain\.com(?::\d{1,5})?$`},
		}, {
			URI:       &istiov1alpha1.StringMatch{Regex: "^/pets/(.*?)?"},
			Authority: &istiov1alpha1.StringMatch{Regex: `^test-route\.test-ns(?::\d{1,5})?$`},
		}, {
			URI:       &istiov1alpha1.StringMatch{Regex: "^/pets/(.*?)?"},
			Authority: &istiov1alpha1.StringMatch{Regex: `^test-route\.test-ns\.svc(?::\d{1,5})?$`},
		}, {
			URI:       &istiov1alpha1.StringMatch{Regex: "^/pets/(.*?)?"},
			Authority: &istiov1alpha1.StringMatch{Regex: `^test-route\.test-ns\.svc\.cluster\.local(?::\d{1,5})?$`},
		}},
		Route: []v1alpha3.HTTPRouteDestination{{
			Destination: v1alpha3.Destination{
				Host: "v2-service.test-ns.svc.cluster.local",
				Port: v1alpha3.PortSelector{Number: 80},
			},
			Weight: 100,
			// Headers: &v1alpha3.Headers{
			// 	Request: &v1alpha3.HeaderOperations{
			// 		Add: map[string]string{
			// 			"ugh": "blah",
			// 		},
			// 	},
			// },
		}},
		// Headers: &v1alpha3.Headers{
		// 	Request: &v1alpha3.HeaderOperations{
		// 		Add: map[string]string{
		// 			"foo": "bar",
		// 		},
		// 	},
		// },
		DeprecatedAppendHeaders: map[string]string{
			"foo": "bar",
		},
		Timeout: networking.DefaultTimeout.String(),
		Retries: &v1alpha3.HTTPRetry{
			Attempts:      networking.DefaultRetryCount,
			PerTryTimeout: networking.DefaultTimeout.String(),
		},
		WebsocketUpgrade: true,
	}, {
		Match: []v1alpha3.HTTPMatchRequest{{
			URI:       &istiov1alpha1.StringMatch{Regex: "^/pets/(.*?)?"},
			Authority: &istiov1alpha1.StringMatch{Regex: `^v1\.domain\.com(?::\d{1,5})?$`},
		}},
		Route: []v1alpha3.HTTPRouteDestination{{
			Destination: v1alpha3.Destination{
				Host: "v1-service.test-ns.svc.cluster.local",
				Port: v1alpha3.PortSelector{Number: 80},
			},
			Weight: 100,
		}},
		// Headers: &v1alpha3.Headers{
		// 	Request: &v1alpha3.HeaderOperations{
		// 		Add: map[string]string{
		// 			"foo": "baz",
		// 		},
		// 	},
		// },
		DeprecatedAppendHeaders: map[string]string{
			"foo": "baz",
		},
		Timeout: networking.DefaultTimeout.String(),
		Retries: &v1alpha3.HTTPRetry{
			Attempts:      networking.DefaultRetryCount,
			PerTryTimeout: networking.DefaultTimeout.String(),
		},
		WebsocketUpgrade: true,
	}}

	routes := MakeVirtualService(ci, []string{}).Spec.HTTP
	if diff := cmp.Diff(expected, routes); diff != "" {
		t.Errorf("Unexpected routes (-want +got): %v", diff)
	}
}

// One active target.
func TestMakeVirtualServiceRoute_Vanilla(t *testing.T) {
	ingressPath := &v1alpha1.HTTPClusterIngressPath{
		Splits: []v1alpha1.ClusterIngressBackendSplit{{
			ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
				ServiceNamespace: "test-ns",
				ServiceName:      "revision-service",
				ServicePort:      intstr.FromInt(80),
			},
			Percent: 100,
		}},
		Timeout: &metav1.Duration{Duration: networking.DefaultTimeout},
		Retries: &v1alpha1.HTTPRetry{
			PerTryTimeout: &metav1.Duration{Duration: networking.DefaultTimeout},
			Attempts:      networking.DefaultRetryCount,
		},
	}
	hosts := []string{"a.com", "b.org"}
	route := makeVirtualServiceRoute(hosts, ingressPath)
	expected := v1alpha3.HTTPRoute{
		Match: []v1alpha3.HTTPMatchRequest{{
			Authority: &istiov1alpha1.StringMatch{Regex: `^a\.com(?::\d{1,5})?$`},
		}, {
			Authority: &istiov1alpha1.StringMatch{Regex: `^b\.org(?::\d{1,5})?$`},
		}},
		Route: []v1alpha3.HTTPRouteDestination{{
			Destination: v1alpha3.Destination{
				Host: "revision-service.test-ns.svc.cluster.local",
				Port: v1alpha3.PortSelector{Number: 80},
			},
			Weight: 100,
		}},
		Timeout: networking.DefaultTimeout.String(),
		Retries: &v1alpha3.HTTPRetry{
			Attempts:      networking.DefaultRetryCount,
			PerTryTimeout: networking.DefaultTimeout.String(),
		},
		WebsocketUpgrade: true,
	}
	if diff := cmp.Diff(&expected, route); diff != "" {
		t.Errorf("Unexpected route  (-want +got): %v", diff)
	}
}

// Two active targets.
func TestMakeVirtualServiceRoute_TwoTargets(t *testing.T) {
	ingressPath := &v1alpha1.HTTPClusterIngressPath{
		Splits: []v1alpha1.ClusterIngressBackendSplit{{
			ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
				ServiceNamespace: "test-ns",
				ServiceName:      "revision-service",
				ServicePort:      intstr.FromInt(80),
			},
			Percent: 90,
		}, {
			ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
				ServiceNamespace: "test-ns",
				ServiceName:      "new-revision-service",
				ServicePort:      intstr.FromString("test-port"),
			},
			Percent: 10,
		}},
		Timeout: &metav1.Duration{Duration: networking.DefaultTimeout},
		Retries: &v1alpha1.HTTPRetry{
			PerTryTimeout: &metav1.Duration{Duration: networking.DefaultTimeout},
			Attempts:      networking.DefaultRetryCount,
		},
	}
	hosts := []string{"test.org"}
	route := makeVirtualServiceRoute(hosts, ingressPath)
	expected := v1alpha3.HTTPRoute{
		Match: []v1alpha3.HTTPMatchRequest{{
			Authority: &istiov1alpha1.StringMatch{Regex: `^test\.org(?::\d{1,5})?$`},
		}},
		Route: []v1alpha3.HTTPRouteDestination{{
			Destination: v1alpha3.Destination{
				Host: "revision-service.test-ns.svc.cluster.local",
				Port: v1alpha3.PortSelector{Number: 80},
			},
			Weight: 90,
		}, {
			Destination: v1alpha3.Destination{
				Host: "new-revision-service.test-ns.svc.cluster.local",
				Port: v1alpha3.PortSelector{Name: "test-port"},
			},
			Weight: 10,
		}},
		Timeout: networking.DefaultTimeout.String(),
		Retries: &v1alpha3.HTTPRetry{
			Attempts:      networking.DefaultRetryCount,
			PerTryTimeout: networking.DefaultTimeout.String(),
		},
		WebsocketUpgrade: true,
	}
	if diff := cmp.Diff(&expected, route); diff != "" {
		t.Errorf("Unexpected route  (-want +got): %v", diff)
	}
}

func TestGetHosts_Duplicate(t *testing.T) {
	ci := &v1alpha1.ClusterIngress{
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.ClusterIngressRule{{
				Hosts: []string{
					"test-route1",
					"test-route2",
				},
			}, {
				Hosts: []string{
					"test-route1",
					"test-route3",
				},
			}},
		},
	}
	hosts := getHosts(ci)
	expected := []string{
		"test-route1",
		"test-route2",
		"test-route3",
	}
	if diff := cmp.Diff(expected, hosts); diff != "" {
		t.Errorf("Unexpected hosts  (-want +got): %v", diff)
	}
}

func TestGetExpandedHosts(t *testing.T) {
	for _, test := range []struct {
		name  string
		hosts []string
		want  []string
	}{{
		name: "cluster local service in non-default namespace",
		hosts: []string{
			"service.namespace.svc.cluster.local",
		},
		want: []string{
			"service.namespace",
			"service.namespace.svc",
			"service.namespace.svc.cluster.local",
		},
	}, {
		name: "example.com service",
		hosts: []string{
			"foo.bar.example.com",
		},
		want: []string{
			"foo.bar.example.com",
		},
	}, {
		name: "default.example.com service",
		hosts: []string{
			"foo.default.example.com",
		},
		want: []string{
			"foo.default.example.com",
		},
	}, {
		name: "mix",
		hosts: []string{
			"foo.default.example.com",
			"foo.default.svc.cluster.local",
		},
		want: []string{
			"foo.default",
			"foo.default.example.com",
			"foo.default.svc",
			"foo.default.svc.cluster.local",
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			got := expandedHosts(test.hosts)
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("Unexpected (-want +got): %v", diff)
			}
		})
	}
}
