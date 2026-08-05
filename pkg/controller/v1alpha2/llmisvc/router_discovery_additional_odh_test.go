//go:build distro

/*
Copyright 2026 The KServe Authors.

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

package llmisvc_test

import (
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func gatewayOrigin(name, ns string) *gwapiv1.ObjectReference {
	return &gwapiv1.ObjectReference{
		Group:     gwapiv1.GroupName,
		Kind:      "Gateway",
		Name:      gwapiv1.ObjectName(name),
		Namespace: ptr.To(gwapiv1.Namespace(ns)),
	}
}

func clusterLocalURL(host, path string) *apis.URL {
	u := apis.HTTPS(host)
	u.Path = path

	return u
}

func notAdmittedRoute(name, ns, host, targetServiceName string) *routev1.Route {
	r := admittedRoute(name, ns, host, targetServiceName)
	r.Status.Ingress[0].Conditions[0].Status = corev1.ConditionFalse

	return r
}

func admittedRoute(name, ns, host, targetServiceName string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: routev1.RouteSpec{
			Host: host,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: targetServiceName,
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationReencrypt,
			},
		},
		Status: routev1.RouteStatus{
			Ingress: []routev1.RouteIngress{
				{
					Host: host,
					Conditions: []routev1.RouteIngressCondition{
						{
							Type:   routev1.RouteAdmitted,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
	}
}

func gwService(name, ns, gwName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				llmisvc.GatewayNameLabelForTest: gwName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				llmisvc.GatewayNameLabelForTest: gwName,
			},
		},
	}
}

func newFakeClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = routev1.Install(scheme)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}

func newReconciler(fc client.Client) (*llmisvc.LLMISVCReconciler, *record.FakeRecorder) {
	rec := record.NewFakeRecorder(10)

	return &llmisvc.LLMISVCReconciler{
		Client:        fc,
		EventRecorder: rec,
	}, rec
}

func newTestLLMInferenceService() *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
	}
}

func TestDiscoverRouteURLs(t *testing.T) {
	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
		path   = "/demo-ns/my-model"
	)

	tests := []struct {
		name     string
		objects  []client.Object
		wantURLs []string
		wantNil  bool
	}{
		{
			name: "Route fronting gateway backing service",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				admittedRoute("my-gateway", gwNS,
					"my-gateway.apps.example.com",
					"my-gateway-openshift-default"),
			},
			wantURLs: []string{"https://my-gateway.apps.example.com/demo-ns/my-model"},
		},
		{
			name: "Route targeting operator-created service with same pod selector",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				gwService("my-gateway-data-science-gateway-class", gwNS, gwName),
				admittedRoute("my-gateway", gwNS,
					"my-gateway.apps.example.com",
					"my-gateway-data-science-gateway-class"),
			},
			wantURLs: []string{"https://my-gateway.apps.example.com/demo-ns/my-model"},
		},
		{
			name: "no Route exists",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
			},
			wantNil: true,
		},
		{
			name: "Route not admitted",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				notAdmittedRoute("my-gateway", gwNS, "host.example.com", "my-gateway-openshift-default"),
			},
			wantNil: true,
		},
		{
			name: "Route targets unrelated service",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				admittedRoute("unrelated", gwNS,
					"unrelated.apps.example.com",
					"some-other-service"),
			},
			wantNil: true,
		},
		{
			name: "Route with Edge TLS termination",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				func() *routev1.Route {
					r := admittedRoute("my-gateway", gwNS,
						"my-gateway.apps.example.com",
						"my-gateway-openshift-default")
					r.Spec.TLS = &routev1.TLSConfig{
						Termination:                   routev1.TLSTerminationEdge,
						InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
					}

					return r
				}(),
			},
			wantURLs: []string{"https://my-gateway.apps.example.com/demo-ns/my-model"},
		},
		{
			name: "disconnected environment topology - Route targets <gw>-data-science-gateway-class with Edge TLS",
			objects: []client.Object{
				func() *corev1.Service {
					svc := gwService("my-gateway-data-science-gateway-class", gwNS, gwName)
					svc.Spec.Ports = []corev1.ServicePort{
						{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443)},
					}

					return svc
				}(),
				func() *routev1.Route {
					r := admittedRoute("my-gateway-route", gwNS,
						"my-gateway.apps.example.com",
						"my-gateway-data-science-gateway-class")
					r.Spec.TLS = &routev1.TLSConfig{
						Termination:                   routev1.TLSTerminationEdge,
						InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
					}
					r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("https")}

					return r
				}(),
			},
			wantURLs: []string{"https://my-gateway.apps.example.com/demo-ns/my-model"},
		},
		{
			name: "Route with path-based routing is skipped",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				func() *routev1.Route {
					r := admittedRoute("my-gateway", gwNS,
						"my-gateway.apps.example.com",
						"my-gateway-openshift-default")
					r.Spec.Path = "/prefix"

					return r
				}(),
			},
			wantNil: true,
		},
		{
			name: "mixed: host-only Route discovered, path-based Route skipped",
			objects: []client.Object{
				gwService("my-gateway-openshift-default", gwNS, gwName),
				admittedRoute("host-only", gwNS,
					"my-gateway.apps.example.com",
					"my-gateway-openshift-default"),
				func() *routev1.Route {
					r := admittedRoute("path-based", gwNS,
						"my-gateway-alt.apps.example.com",
						"my-gateway-openshift-default")
					r.Spec.Path = "/prefix"

					return r
				}(),
			},
			wantURLs: []string{"https://my-gateway.apps.example.com/demo-ns/my-model"},
		},
		{
			name: "no services with gateway selector",
			objects: []client.Object{
				admittedRoute("my-gateway", gwNS,
					"my-gateway.apps.example.com",
					"my-gateway-openshift-default"),
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, path)

			r, _ := newReconciler(newFakeClient(tt.objects...))
			urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
			g.Expect(err).ToNot(HaveOccurred())

			if tt.wantNil {
				g.Expect(urls).To(BeEmpty())

				return
			}

			var actual []string
			for _, u := range urls {
				actual = append(actual, u.URL.String())
			}
			g.Expect(actual).To(Equal(tt.wantURLs))

			for _, u := range urls {
				g.Expect(u.Origin).ToNot(BeNil())
				g.Expect(string(u.Origin.Kind)).To(Equal("Route"))
				g.Expect(string(u.Origin.Group)).To(Equal(routev1.GroupName))
			}
		})
	}
}

func TestDiscoverAdditionalURLs_SkipsGatewaysWithExternalURLs(t *testing.T) {
	g := NewWithT(t)

	origin := gatewayOrigin("my-gateway", "gw-ns")

	discovered := []llmisvc.DiscoveredURL{
		{URL: apis.HTTPS("my-model.example.com"), Origin: origin},
		{URL: clusterLocalURL("gw.gw-ns.svc.cluster.local", "/ns/model"), Origin: origin},
	}

	r, _ := newReconciler(newFakeClient())
	additional, err := llmisvc.DiscoverAdditionalURLsForTest(r, t.Context(), newTestLLMInferenceService(), discovered)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(additional).To(BeEmpty())
}

func TestDiscoverAdditionalURLs_DiscoversRouteForGatewayWithoutExternalURLs(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "ai-gateway"
		gwNS   = "openshift-ingress"
	)

	origin := gatewayOrigin(gwName, gwNS)

	discovered := []llmisvc.DiscoveredURL{
		{URL: clusterLocalURL("ai-gateway-openshift-default.openshift-ingress.svc.cluster.local", "/demo/model"), Origin: origin},
	}

	fc := newFakeClient(
		gwService("ai-gateway-openshift-default", gwNS, gwName),
		gwService("ai-gateway-data-science-gateway-class", gwNS, gwName),
		admittedRoute("ai-gateway", gwNS,
			"ai-gateway.apps.cluster.example.com",
			"ai-gateway-data-science-gateway-class"),
	)

	r, _ := newReconciler(fc)
	additional, err := llmisvc.DiscoverAdditionalURLsForTest(r, t.Context(), newTestLLMInferenceService(), discovered)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(additional).To(HaveLen(1))
	g.Expect(additional[0].URL.String()).To(Equal("https://ai-gateway.apps.cluster.example.com/demo/model"))
	g.Expect(string(additional[0].Origin.Kind)).To(Equal("Route"))
	g.Expect(string(additional[0].Origin.Name)).To(Equal("ai-gateway"))
}

func TestDiscoverAdditionalURLs_MultiplePathsPerGateway(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "ai-gateway"
		gwNS   = "openshift-ingress"
	)

	origin := gatewayOrigin(gwName, gwNS)

	discovered := []llmisvc.DiscoveredURL{
		{URL: clusterLocalURL("gw.openshift-ingress.svc.cluster.local", "/ns/model-a"), Origin: origin},
		{URL: clusterLocalURL("gw.openshift-ingress.svc.cluster.local", "/ns/model-b"), Origin: origin},
	}

	fc := newFakeClient(
		gwService("ai-gateway-svc", gwNS, gwName),
		admittedRoute("ai-gateway", gwNS,
			"ai-gateway.apps.example.com",
			"ai-gateway-svc"),
	)

	r, _ := newReconciler(fc)
	additional, err := llmisvc.DiscoverAdditionalURLsForTest(r, t.Context(), newTestLLMInferenceService(), discovered)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(additional).To(HaveLen(2))

	urls := make(map[string]bool)
	for _, a := range additional {
		urls[a.URL.String()] = true
	}
	g.Expect(urls).To(HaveKey("https://ai-gateway.apps.example.com/ns/model-a"))
	g.Expect(urls).To(HaveKey("https://ai-gateway.apps.example.com/ns/model-b"))
}

func TestDiscoverAdditionalURLs_DeduplicatesAgainstExistingURLs(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "ai-gateway"
		gwNS   = "openshift-ingress"
	)

	origin := gatewayOrigin(gwName, gwNS)

	// Simulate a case where the Route URL is already in the discovered set
	// (e.g. if the gateway status address happens to match the Route host)
	discovered := []llmisvc.DiscoveredURL{
		{URL: clusterLocalURL("gw.openshift-ingress.svc.cluster.local", "/demo/model"), Origin: origin},
		{URL: clusterLocalURL("ai-gateway.apps.example.com", "/demo/model"), Origin: origin},
	}

	fc := newFakeClient(
		gwService("ai-gateway-svc", gwNS, gwName),
		admittedRoute("ai-gateway", gwNS,
			"ai-gateway.apps.example.com",
			"ai-gateway-svc"),
	)

	r, _ := newReconciler(fc)
	additional, err := llmisvc.DiscoverAdditionalURLsForTest(r, t.Context(), newTestLLMInferenceService(), discovered)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(additional).To(BeEmpty(), "Route URL should be deduplicated against existing discovered URLs")
}

func TestDiscoverRouteURLs_PathBasedRouteEmitsWarningEvent(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	fc := newFakeClient(
		gwService("my-gateway-svc", gwNS, gwName),
		func() *routev1.Route {
			r := admittedRoute("path-route", gwNS,
				"my-gateway.apps.example.com",
				"my-gateway-svc")
			r.Spec.Path = "/prefix"

			return r
		}(),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(urls).To(BeEmpty(), "path-based Route should be skipped")

	g.Expect(rec.Events).To(HaveLen(1))
	event := <-rec.Events
	g.Expect(event).To(ContainSubstring("RoutePathIncompatible"))
	g.Expect(event).To(ContainSubstring("path-based routing"))
	g.Expect(event).To(ContainSubstring("path-route"))
	g.Expect(event).To(ContainSubstring("/prefix"))
}

func TestDiscoverRouteURLs_PortMismatchEmitsWarningEvent(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	svc := gwService("my-gateway-svc", gwNS, gwName)
	svc.Spec.Ports = []corev1.ServicePort{
		{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443)},
	}

	fc := newFakeClient(
		svc,
		func() *routev1.Route {
			r := admittedRoute("mismatched-port", gwNS,
				"my-gateway.apps.example.com",
				"my-gateway-svc")
			r.Spec.Port = &routev1.RoutePort{
				TargetPort: intstr.FromString("wrong-port"),
			}

			return r
		}(),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	// URL is still generated despite the warning (non-blocking mismatch)
	g.Expect(urls).To(HaveLen(1))

	g.Expect(rec.Events).To(HaveLen(1))
	event := <-rec.Events
	g.Expect(event).To(ContainSubstring("RoutePortMismatch"))
	g.Expect(event).To(ContainSubstring("port mismatch"))
	g.Expect(event).To(ContainSubstring("wrong-port"))
}

func TestDiscoverRouteURLs_MatchingPortNoWarning(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	svc := gwService("my-gateway-svc", gwNS, gwName)
	svc.Spec.Ports = []corev1.ServicePort{
		{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443)},
	}

	fc := newFakeClient(
		svc,
		func() *routev1.Route {
			r := admittedRoute("good-port", gwNS,
				"my-gateway.apps.example.com",
				"my-gateway-svc")
			r.Spec.Port = &routev1.RoutePort{
				TargetPort: intstr.FromString("https"),
			}

			return r
		}(),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(urls).To(HaveLen(1))
	g.Expect(rec.Events).To(BeEmpty(), "matching port should not produce a warning")
}

func TestDiscoverRouteURLs_NoPortSpecifiedNoWarning(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	fc := newFakeClient(
		gwService("my-gateway-svc", gwNS, gwName),
		admittedRoute("no-port", gwNS,
			"my-gateway.apps.example.com",
			"my-gateway-svc"),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(urls).To(HaveLen(1))
	g.Expect(rec.Events).To(BeEmpty(), "Route without port spec should not produce a warning")
}

func TestCheckRoutePortMismatch(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "gw-svc", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443)},
				{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080)},
			},
		},
	}
	gwServices := map[string]*corev1.Service{"gw-svc": svc}

	tests := []struct {
		name      string
		route     *routev1.Route
		wantEmpty bool
	}{
		{
			name: "no port specified - no mismatch",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To: routev1.RouteTargetReference{Name: "gw-svc"},
				},
			},
			wantEmpty: true,
		},
		{
			name: "matching port name",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromString("https")},
				},
			},
			wantEmpty: true,
		},
		{
			name: "matching port number",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromInt32(443)},
				},
			},
			wantEmpty: true,
		},
		{
			name: "matching target port number",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromInt32(8443)},
				},
			},
			wantEmpty: true,
		},
		{
			name: "matching numeric string port",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromString("443")},
				},
			},
			wantEmpty: true,
		},
		{
			name: "matching numeric string target port",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromString("8443")},
				},
			},
			wantEmpty: true,
		},
		{
			name: "mismatched port name",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromString("grpc")},
				},
			},
			wantEmpty: false,
		},
		{
			name: "mismatched port number",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "gw-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromInt32(9999)},
				},
			},
			wantEmpty: false,
		},
		{
			name: "service not in map - no mismatch",
			route: &routev1.Route{
				Spec: routev1.RouteSpec{
					To:   routev1.RouteTargetReference{Name: "other-svc"},
					Port: &routev1.RoutePort{TargetPort: intstr.FromString("grpc")},
				},
			},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := llmisvc.CheckRoutePortMismatchForTest(tt.route, gwServices)
			if tt.wantEmpty {
				g.Expect(result).To(BeEmpty())
			} else {
				g.Expect(result).ToNot(BeEmpty())
				g.Expect(result).To(ContainSubstring("does not match"))
			}
		})
	}
}

func TestDiscoverRouteURLs_TLSMismatchEmitsWarningEvent(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	httpsProto := "https"
	svc := gwService("my-gateway-svc", gwNS, gwName)
	svc.Spec.Ports = []corev1.ServicePort{
		{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443), AppProtocol: &httpsProto},
	}

	fc := newFakeClient(
		svc,
		func() *routev1.Route {
			r := admittedRoute("edge-route", gwNS,
				"my-gateway.apps.example.com",
				"my-gateway-svc")
			r.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}

			return r
		}(),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(urls).To(HaveLen(1), "URL is still generated despite TLS mismatch")

	g.Expect(rec.Events).To(HaveLen(1))
	event := <-rec.Events
	g.Expect(event).To(ContainSubstring("RouteTLSMismatch"))
	g.Expect(event).To(ContainSubstring("edge"))
	g.Expect(event).To(ContainSubstring("appProtocol"))
}

func TestDiscoverRouteURLs_CompatibleTLSNoWarning(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	httpsProto := "https"
	svc := gwService("my-gateway-svc", gwNS, gwName)
	svc.Spec.Ports = []corev1.ServicePort{
		{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443), AppProtocol: &httpsProto},
	}

	fc := newFakeClient(
		svc,
		func() *routev1.Route {
			r := admittedRoute("passthrough-route", gwNS,
				"my-gateway.apps.example.com",
				"my-gateway-svc")
			r.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			}

			return r
		}(),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(urls).To(HaveLen(1))
	g.Expect(rec.Events).To(BeEmpty(), "passthrough Route with HTTPS service should not produce a TLS warning")
}

func TestDiscoverRouteURLs_NoAppProtocolNoTLSWarning(t *testing.T) {
	g := NewWithT(t)

	const (
		gwName = "my-gateway"
		gwNS   = "openshift-ingress"
	)

	svc := gwService("my-gateway-svc", gwNS, gwName)
	svc.Spec.Ports = []corev1.ServicePort{
		{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443)},
	}

	fc := newFakeClient(
		svc,
		func() *routev1.Route {
			r := admittedRoute("edge-route", gwNS,
				"my-gateway.apps.example.com",
				"my-gateway-svc")
			r.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}

			return r
		}(),
	)

	r, rec := newReconciler(fc)
	gw := llmisvc.NewGatewayWithPaths(gwName, gwNS, "/ns/model")

	urls, err := llmisvc.DiscoverRouteURLsForTest(r, t.Context(), newTestLLMInferenceService(), gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(urls).To(HaveLen(1))
	g.Expect(rec.Events).To(BeEmpty(), "no appProtocol means best-effort skips TLS check")
}

func TestCheckRouteTLSMismatch(t *testing.T) {
	httpProto := "http"
	httpsProto := "https"
	h2cProto := "kubernetes.io/h2c"

	svcWith := func(appProto *string) map[string]*corev1.Service {
		return map[string]*corev1.Service{
			"gw-svc": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw-svc", Namespace: "ns"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "main", Port: 443, AppProtocol: appProto},
					},
				},
			},
		}
	}

	route := func(termination routev1.TLSTerminationType) *routev1.Route {
		r := &routev1.Route{Spec: routev1.RouteSpec{To: routev1.RouteTargetReference{Name: "gw-svc"}}}
		if termination != "" {
			r.Spec.TLS = &routev1.TLSConfig{Termination: termination}
		}

		return r
	}

	tests := []struct {
		name       string
		route      *routev1.Route
		gwServices map[string]*corev1.Service
		wantEmpty  bool
	}{
		{
			name:       "edge Route + https appProtocol - mismatch",
			route:      route(routev1.TLSTerminationEdge),
			gwServices: svcWith(&httpsProto),
			wantEmpty:  false,
		},
		{
			name:       "no-TLS Route + https appProtocol - mismatch",
			route:      route(""),
			gwServices: svcWith(&httpsProto),
			wantEmpty:  false,
		},
		{
			name:       "passthrough Route + http appProtocol - mismatch",
			route:      route(routev1.TLSTerminationPassthrough),
			gwServices: svcWith(&httpProto),
			wantEmpty:  false,
		},
		{
			name:       "reencrypt Route + h2c appProtocol - mismatch",
			route:      route(routev1.TLSTerminationReencrypt),
			gwServices: svcWith(&h2cProto),
			wantEmpty:  false,
		},
		{
			name:       "passthrough Route + https appProtocol - compatible",
			route:      route(routev1.TLSTerminationPassthrough),
			gwServices: svcWith(&httpsProto),
			wantEmpty:  true,
		},
		{
			name:       "reencrypt Route + https appProtocol - compatible",
			route:      route(routev1.TLSTerminationReencrypt),
			gwServices: svcWith(&httpsProto),
			wantEmpty:  true,
		},
		{
			name:       "edge Route + http appProtocol - compatible",
			route:      route(routev1.TLSTerminationEdge),
			gwServices: svcWith(&httpProto),
			wantEmpty:  true,
		},
		{
			name:       "edge Route + h2c appProtocol - compatible",
			route:      route(routev1.TLSTerminationEdge),
			gwServices: svcWith(&h2cProto),
			wantEmpty:  true,
		},
		{
			name:       "no appProtocol - skip check",
			route:      route(routev1.TLSTerminationEdge),
			gwServices: svcWith(nil),
			wantEmpty:  true,
		},
		{
			name:       "service not in map - skip check",
			route:      route(routev1.TLSTerminationEdge),
			gwServices: map[string]*corev1.Service{},
			wantEmpty:  true,
		},
		{
			name:       "unknown appProtocol - skip check",
			route:      route(routev1.TLSTerminationEdge),
			gwServices: svcWith(ptr.To("grpc")),
			wantEmpty:  true,
		},
		{
			name:  "no service ports - skip check",
			route: route(routev1.TLSTerminationEdge),
			gwServices: map[string]*corev1.Service{
				"gw-svc": {
					ObjectMeta: metav1.ObjectMeta{Name: "gw-svc", Namespace: "ns"},
					Spec:       corev1.ServiceSpec{},
				},
			},
			wantEmpty: true,
		},
		{
			name: "multi-port service - uses targeted port appProtocol",
			route: func() *routev1.Route {
				r := route(routev1.TLSTerminationEdge)
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("http")}

				return r
			}(),
			gwServices: map[string]*corev1.Service{
				"gw-svc": {
					ObjectMeta: metav1.ObjectMeta{Name: "gw-svc", Namespace: "ns"},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "https", Port: 443, AppProtocol: &httpsProto},
							{Name: "http", Port: 80, AppProtocol: &httpProto},
						},
					},
				},
			},
			wantEmpty: true,
		},
		{
			name: "multi-port service - mismatched targeted port",
			route: func() *routev1.Route {
				r := route(routev1.TLSTerminationEdge)
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("https")}

				return r
			}(),
			gwServices: map[string]*corev1.Service{
				"gw-svc": {
					ObjectMeta: metav1.ObjectMeta{Name: "gw-svc", Namespace: "ns"},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "https", Port: 443, AppProtocol: &httpsProto},
							{Name: "http", Port: 80, AppProtocol: &httpProto},
						},
					},
				},
			},
			wantEmpty: false,
		},
		{
			name: "numeric string target port resolves appProtocol",
			route: func() *routev1.Route {
				r := route(routev1.TLSTerminationEdge)
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("443")}

				return r
			}(),
			gwServices: map[string]*corev1.Service{
				"gw-svc": {
					ObjectMeta: metav1.ObjectMeta{Name: "gw-svc", Namespace: "ns"},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "https", Port: 443, AppProtocol: &httpsProto},
						},
					},
				},
			},
			wantEmpty: false,
		},
		{
			name: "target port not found on service - skip check",
			route: func() *routev1.Route {
				r := route(routev1.TLSTerminationEdge)
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("missing")}

				return r
			}(),
			gwServices: svcWith(&httpsProto),
			wantEmpty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := llmisvc.CheckRouteTLSMismatchForTest(tt.route, tt.gwServices)
			if tt.wantEmpty {
				g.Expect(result).To(BeEmpty())
			} else {
				g.Expect(result).ToNot(BeEmpty())
			}
		})
	}
}

func TestRouteChangePredicate(t *testing.T) {
	pred := llmisvc.RouteChangePredicateForTest()

	tests := []struct {
		name   string
		old    *routev1.Route
		new    *routev1.Route
		expect bool
	}{
		{
			name:   "host change triggers update",
			old:    admittedRoute("r", "ns", "old.example.com", "svc"),
			new:    admittedRoute("r", "ns", "new.example.com", "svc"),
			expect: true,
		},
		{
			name:   "backend retarget triggers update",
			old:    admittedRoute("r", "ns", "host.example.com", "svc-a"),
			new:    admittedRoute("r", "ns", "host.example.com", "svc-b"),
			expect: true,
		},
		{
			name: "TLS termination mode change triggers update",
			old:  admittedRoute("r", "ns", "host.example.com", "svc"),
			new: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.TLS.Termination = routev1.TLSTerminationPassthrough

				return r
			}(),
			expect: true,
		},
		{
			name: "TLS added triggers update",
			old: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.TLS = nil

				return r
			}(),
			new:    admittedRoute("r", "ns", "host.example.com", "svc"),
			expect: true,
		},
		{
			name: "path added triggers update",
			old:  admittedRoute("r", "ns", "host.example.com", "svc"),
			new: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Path = "/prefix"

				return r
			}(),
			expect: true,
		},
		{
			name: "path changed triggers update",
			old: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Path = "/old"

				return r
			}(),
			new: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Path = "/new"

				return r
			}(),
			expect: true,
		},
		{
			name: "path removed triggers update",
			old: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Path = "/prefix"

				return r
			}(),
			new:    admittedRoute("r", "ns", "host.example.com", "svc"),
			expect: true,
		},
		{
			name: "port added triggers update",
			old:  admittedRoute("r", "ns", "host.example.com", "svc"),
			new: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("https")}

				return r
			}(),
			expect: true,
		},
		{
			name: "port changed triggers update",
			old: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("http")}

				return r
			}(),
			new: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("https")}

				return r
			}(),
			expect: true,
		},
		{
			name: "port removed triggers update",
			old: func() *routev1.Route {
				r := admittedRoute("r", "ns", "host.example.com", "svc")
				r.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString("https")}

				return r
			}(),
			new:    admittedRoute("r", "ns", "host.example.com", "svc"),
			expect: true,
		},
		{
			name:   "no change does not trigger",
			old:    admittedRoute("r", "ns", "host.example.com", "svc"),
			new:    admittedRoute("r", "ns", "host.example.com", "svc"),
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := pred.Update(event.UpdateEvent{
				ObjectOld: tt.old,
				ObjectNew: tt.new,
			})
			g.Expect(result).To(Equal(tt.expect))
		})
	}
}

func TestHasRouteOrigin(t *testing.T) {
	origin := func(kind, name, ns string) *gwapiv1.ObjectReference {
		return &gwapiv1.ObjectReference{
			Kind:      gwapiv1.Kind(kind),
			Name:      gwapiv1.ObjectName(name),
			Namespace: ptr.To(gwapiv1.Namespace(ns)),
		}
	}

	tests := []struct {
		name      string
		addresses []v1alpha2.SourcedAddress
		expect    bool
	}{
		{
			name: "matches Route origin",
			addresses: []v1alpha2.SourcedAddress{
				{Origin: origin("Route", "my-route", "gw-ns")},
			},
			expect: true,
		},
		{
			name: "different Route name",
			addresses: []v1alpha2.SourcedAddress{
				{Origin: origin("Route", "other-route", "gw-ns")},
			},
			expect: false,
		},
		{
			name: "Gateway origin not matched",
			addresses: []v1alpha2.SourcedAddress{
				{Origin: origin("Gateway", "my-route", "gw-ns")},
			},
			expect: false,
		},
		{
			name:      "empty addresses",
			addresses: nil,
			expect:    false,
		},
		{
			name: "nil origin skipped",
			addresses: []v1alpha2.SourcedAddress{
				{Origin: nil},
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			llmSvc := &v1alpha2.LLMInferenceService{}
			llmSvc.Status.Addresses = tt.addresses

			g.Expect(llmisvc.HasRouteOriginForTest(llmSvc, "my-route", "gw-ns")).To(Equal(tt.expect))
		})
	}
}

func TestHTTPRouteReferencesGateway(t *testing.T) {
	tests := []struct {
		name   string
		route  *gwapiv1.HTTPRoute
		gwName string
		gwNS   string
		expect bool
	}{
		{
			name: "parentRef matches gateway",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{Name: "my-gw", Namespace: ptr.To(gwapiv1.Namespace("gw-ns"))},
						},
					},
				},
			},
			gwName: "my-gw",
			gwNS:   "gw-ns",
			expect: true,
		},
		{
			name: "parentRef does not match",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{Name: "other-gw", Namespace: ptr.To(gwapiv1.Namespace("other-ns"))},
						},
					},
				},
			},
			gwName: "my-gw",
			gwNS:   "gw-ns",
			expect: false,
		},
		{
			name: "parentRef namespace defaults to HTTPRoute namespace",
			route: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Namespace: "route-ns"},
				Spec: gwapiv1.HTTPRouteSpec{
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{Name: "my-gw"},
						},
					},
				},
			},
			gwName: "my-gw",
			gwNS:   "route-ns",
			expect: true,
		},
		{
			name: "matches when one of multiple parentRefs targets the gateway",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{Name: "other-gw", Namespace: ptr.To(gwapiv1.Namespace("other-ns"))},
							{Name: "my-gw", Namespace: ptr.To(gwapiv1.Namespace("gw-ns"))},
						},
					},
				},
			},
			gwName: "my-gw",
			gwNS:   "gw-ns",
			expect: true,
		},
		{
			name: "parentRef targeting non-Gateway kind is ignored",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Group:     ptr.To(gwapiv1.Group("")),
								Kind:      ptr.To(gwapiv1.Kind("Service")),
								Name:      "my-gw",
								Namespace: ptr.To(gwapiv1.Namespace("gw-ns")),
							},
						},
					},
				},
			},
			gwName: "my-gw",
			gwNS:   "gw-ns",
			expect: false,
		},
		{
			name: "no parentRefs",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{},
			},
			gwName: "my-gw",
			gwNS:   "gw-ns",
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(llmisvc.HTTPRouteReferencesGatewayForTest(tt.route, tt.gwName, tt.gwNS)).To(Equal(tt.expect))
		})
	}
}

func newMapperFakeClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = routev1.Install(scheme)
	_ = gwapiv1.Install(scheme)
	_ = v1alpha2.AddToScheme(scheme)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(&v1alpha2.LLMInferenceService{}).
		Build()
}

func TestMapRouteToRequests(t *testing.T) {
	const (
		gwName = "my-gateway"
		gwNS   = "gw-ns"
	)

	route := admittedRoute("my-route", gwNS, "my-route.apps.example.com", "my-gateway-svc")

	tests := []struct {
		name    string
		objects []client.Object
		want    []types.NamespacedName
	}{
		{
			name: "reverse: enqueues service with Route origin in status",
			objects: []client.Object{
				gwService("my-gateway-svc", gwNS, gwName),
				&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-with-origin", Namespace: "app-ns"},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Addresses: []v1alpha2.SourcedAddress{
							{Origin: &gwapiv1.ObjectReference{
								Kind:      "Route",
								Name:      "my-route",
								Namespace: ptr.To(gwapiv1.Namespace(gwNS)),
							}},
						},
					},
				},
			},
			want: []types.NamespacedName{{Namespace: "app-ns", Name: "svc-with-origin"}},
		},
		{
			name: "forward (a): enqueues owner of managed HTTPRoute targeting gateway",
			objects: []client.Object{
				gwService("my-gateway-svc", gwNS, gwName),
				&gwapiv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "managed-route",
						Namespace: "app-ns",
						Labels:    map[string]string{constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue},
						OwnerReferences: []metav1.OwnerReference{
							{Kind: "LLMInferenceService", Name: "svc-managed", APIVersion: "serving.kserve.io/v1alpha2"},
						},
					},
					Spec: gwapiv1.HTTPRouteSpec{
						CommonRouteSpec: gwapiv1.CommonRouteSpec{
							ParentRefs: []gwapiv1.ParentReference{
								{Name: gwapiv1.ObjectName(gwName), Namespace: ptr.To(gwapiv1.Namespace(gwNS))},
							},
						},
					},
				},
				&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-managed", Namespace: "app-ns"},
				},
			},
			want: []types.NamespacedName{{Namespace: "app-ns", Name: "svc-managed"}},
		},
		{
			name: "forward (b): enqueues service with explicit refs to HTTPRoute targeting gateway",
			objects: []client.Object{
				gwService("my-gateway-svc", gwNS, gwName),
				&gwapiv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{Name: "external-route", Namespace: "app-ns"},
					Spec: gwapiv1.HTTPRouteSpec{
						CommonRouteSpec: gwapiv1.CommonRouteSpec{
							ParentRefs: []gwapiv1.ParentReference{
								{Name: gwapiv1.ObjectName(gwName), Namespace: ptr.To(gwapiv1.Namespace(gwNS))},
							},
						},
					},
				},
				&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-refs", Namespace: "app-ns"},
					Spec: v1alpha2.LLMInferenceServiceSpec{
						Router: &v1alpha2.RouterSpec{
							Route: &v1alpha2.GatewayRoutesSpec{
								HTTP: &v1alpha2.HTTPRouteSpec{
									Refs: []corev1.LocalObjectReference{{Name: "external-route"}},
								},
							},
						},
					},
				},
			},
			want: []types.NamespacedName{{Namespace: "app-ns", Name: "svc-refs"}},
		},
		{
			name: "forward (c): enqueues service with gateway in status.router.gateways",
			objects: []client.Object{
				gwService("my-gateway-svc", gwNS, gwName),
				&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-status", Namespace: "app-ns"},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Router: &v1alpha2.RouterStatus{
							Gateways: []v1alpha2.ObservedGateway{
								{ObjectReference: gwapiv1.ObjectReference{
									Kind:      "Gateway",
									Name:      gwapiv1.ObjectName(gwName),
									Namespace: ptr.To(gwapiv1.Namespace(gwNS)),
								}},
							},
						},
					},
				},
			},
			want: []types.NamespacedName{{Namespace: "app-ns", Name: "svc-status"}},
		},
		{
			name: "deduplicates across strategies",
			objects: []client.Object{
				gwService("my-gateway-svc", gwNS, gwName),
				&gwapiv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "managed-route",
						Namespace: "app-ns",
						Labels:    map[string]string{constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue},
						OwnerReferences: []metav1.OwnerReference{
							{Kind: "LLMInferenceService", Name: "svc-both", APIVersion: "serving.kserve.io/v1alpha2"},
						},
					},
					Spec: gwapiv1.HTTPRouteSpec{
						CommonRouteSpec: gwapiv1.CommonRouteSpec{
							ParentRefs: []gwapiv1.ParentReference{
								{Name: gwapiv1.ObjectName(gwName), Namespace: ptr.To(gwapiv1.Namespace(gwNS))},
							},
						},
					},
				},
				&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-both", Namespace: "app-ns"},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Router: &v1alpha2.RouterStatus{
							Gateways: []v1alpha2.ObservedGateway{
								{ObjectReference: gwapiv1.ObjectReference{
									Kind:      "Gateway",
									Name:      gwapiv1.ObjectName(gwName),
									Namespace: ptr.To(gwapiv1.Namespace(gwNS)),
								}},
							},
						},
					},
				},
			},
			want: []types.NamespacedName{{Namespace: "app-ns", Name: "svc-both"}},
		},
		{
			name: "no match returns empty",
			objects: []client.Object{
				gwService("my-gateway-svc", gwNS, gwName),
				&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-unrelated", Namespace: "app-ns"},
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fc := newMapperFakeClient(tt.objects...)
			r := &llmisvc.LLMISVCReconciler{Client: fc}

			reqs := llmisvc.MapRouteToRequestsForTest(r, t.Context(), route)

			var got []types.NamespacedName
			for _, req := range reqs {
				got = append(got, req.NamespacedName)
			}
			if tt.want == nil {
				g.Expect(got).To(BeEmpty())
			} else {
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}
