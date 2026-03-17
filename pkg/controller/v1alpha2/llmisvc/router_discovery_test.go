/*
Copyright 2025 The KServe Authors.

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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// expectURLs returns an assert function that expects no error and exact URL match
func expectURLs(expected ...string) func(g Gomega, urls []string, err error) {
	return func(g Gomega, urls []string, err error) {
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(urls).To(Equal(expected))
	}
}

// expectURLsContain returns an assert function that expects no error and URLs containing the expected elements
func expectURLsContain(expected ...string) func(g Gomega, urls []string, err error) {
	return func(g Gomega, urls []string, err error) {
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(urls).To(ContainElements(expected))
	}
}

// expectError returns an assert function that expects an error matching the predicate
func expectError(check func(error) bool) func(g Gomega, urls []string, err error) {
	return func(g Gomega, urls []string, err error) {
		g.Expect(err).To(HaveOccurred())
		g.Expect(check(err)).To(BeTrue(), "Error check failed for: %v", err)
	}
}

func TestDiscoverURLs(t *testing.T) {
	tests := []struct {
		name               string
		route              *gwapiv1.HTTPRoute
		gateways           []*gwapiv1.Gateway
		services           []*corev1.Service
		preferredUrlScheme string
		assert             func(g Gomega, urls []string, err error)
	}{
		// ===== Basic address resolution =====
		{
			name: "basic external address resolution",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{HTTPGateway("test-gateway", "test-ns", "203.0.113.1")},
			assert:   expectURLs("http://203.0.113.1/"),
		},
		{
			name: "address ordering consistency - same addresses different order",
			route: HTTPRoute("consistency-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("consistency-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("consistency-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.200", "203.0.113.100"),
				),
			},
			assert: expectURLs("http://203.0.113.100/", "http://203.0.113.200/"),
		},
		// ===== Hostname handling =====
		{
			name: "route hostname within listener wildcard",
			route: HTTPRoute("hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("hostname-gateway", RefInNamespace("test-ns"))),
				WithHostnames("api.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("hostname-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://api.example.com/"),
		},
		{
			name: "route wildcard hostname - use gateway address",
			route: HTTPRoute("wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-gateway", RefInNamespace("test-ns"))),
				WithHostnames("*"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("wildcard-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.100"),
				),
			},
			assert: expectURLs("http://203.0.113.100/"),
		},
		{
			name: "multiple hostnames - generates multiple URLs",
			route: HTTPRoute("multi-hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("multi-hostname-gateway", RefInNamespace("test-ns"))),
				WithHostnames("api.example.com", "alt.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("multi-hostname-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://alt.example.com/", "http://api.example.com/"),
		},

		// ===== Path handling =====
		{
			name: "custom path extraction",
			route: HTTPRoute("path-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("path-gateway", RefInNamespace("test-ns"))),
				WithPath("/api/v1"),
			),
			gateways: []*gwapiv1.Gateway{HTTPGateway("path-gateway", "test-ns", "203.0.113.1")},
			assert:   expectURLs("http://203.0.113.1/api/v1"),
		},
		{
			name: "multi-rule path extraction - prefers Service-backed rule path",
			route: HTTPRoute("multi-rule-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("multi-rule-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name/v1/completions")),
					WithBackendRefs(BackendRefInferencePool("pool")),
				),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name/v1/chat/completions")),
					WithBackendRefs(BackendRefInferencePool("pool")),
				),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name")),
					WithBackendRefs(BackendRefService("svc")),
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("multi-rule-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1/ns/name"),
		},
		{
			name: "multi-rule path extraction - falls back to shortest when no Service backend",
			route: HTTPRoute("no-svc-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("no-svc-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name/v1/completions")),
					WithBackendRefs(BackendRefInferencePool("pool")),
				),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name/v1/chat/completions")),
					WithBackendRefs(BackendRefInferencePool("pool")),
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("no-svc-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1/ns/name/v1/completions"),
		},
		{
			name: "multi-rule path extraction - Service with default Kind (nil)",
			route: HTTPRoute("default-kind-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("default-kind-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name/v1/completions")),
					WithBackendRefs(BackendRefInferencePool("pool")),
				),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name")),
					WithBackendRefs(gwapiv1.HTTPBackendRef{
						BackendRef: gwapiv1.BackendRef{
							BackendObjectReference: gwapiv1.BackendObjectReference{
								// Kind nil defaults to "Service" per Gateway API spec
								Name: "svc",
								Port: ptr.To(gwapiv1.PortNumber(8000)),
							},
						},
					}),
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("default-kind-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1/ns/name"),
		},
		{
			name: "multi-rule path extraction - nil match.Path is skipped",
			route: HTTPRoute("nil-path-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("nil-path-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					Matches(gwapiv1.HTTPRouteMatch{
						// No Path set - header-only match
						Headers: []gwapiv1.HTTPHeaderMatch{{
							Name:  "x-custom",
							Value: "val",
						}},
					}),
					WithBackendRefs(BackendRefService("svc")),
				),
				WithHTTPRule(
					Matches(PathPrefixMatch("/ns/name")),
					WithBackendRefs(BackendRefService("svc")),
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("nil-path-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1/ns/name"),
		},
		{
			name: "empty route rules - default path",
			route: HTTPRoute("empty-rules-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("empty-rules-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{HTTPGateway("empty-rules-gateway", "test-ns", "203.0.113.1")},
			assert:   expectURLs("http://203.0.113.1/"),
		},

		// ===== Scheme from listener =====
		{
			name: "HTTPS scheme from gateway listener",
			route: HTTPRoute("https-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("https-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{HTTPSGateway("https-gateway", "test-ns", "203.0.113.1")},
			assert:   expectURLs("https://203.0.113.1/"),
		},
		{
			name: "TLS protocol listener uses HTTPS scheme",
			route: HTTPRoute("tls-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("tls-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("tls-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "tls",
						Protocol: gwapiv1.TLSProtocolType,
						Port:     443,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("https://203.0.113.1/"),
		},
		{
			name: "IPv6 gateway address - brackets in URL",
			route: HTTPRoute("ipv6-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("ipv6-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{HTTPGateway("ipv6-gateway", "test-ns", "2001:db8::1")},
			assert:   expectURLs("http://[2001:db8::1]/"),
		},

		// ===== Multiple parent refs =====
		{
			name: "multiple parent refs - sorted selection",
			route: HTTPRoute("multi-parent-route",
				InNamespace[*gwapiv1.HTTPRoute]("default-ns"),
				WithParentRefs(
					GatewayRef("z-gateway", RefInNamespace("z-namespace")),
					GatewayRef("a-gateway", RefInNamespace("a-namespace")),
					GatewayRef("b-gateway", RefInNamespace("a-namespace")),
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("a-gateway",
					InNamespace[*gwapiv1.Gateway]("a-namespace"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				),
				Gateway("z-gateway",
					InNamespace[*gwapiv1.Gateway]("z-namespace"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.2"),
				),
				Gateway("b-gateway",
					InNamespace[*gwapiv1.Gateway]("a-namespace"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.3"),
				),
			},
			assert: expectURLs(
				"http://203.0.113.2/",
				"http://203.0.113.1/",
				"http://203.0.113.3/",
			),
		},
		{
			name: "route with multiple parent refs to different gateways",
			route: HTTPRoute("multi-gateway-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(
					gwapiv1.ParentReference{
						Name:      "gateway-a",
						Namespace: ptr.To(gwapiv1.Namespace("gw-ns")),
						Group:     ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
						Kind:      ptr.To(gwapiv1.Kind("Gateway")),
					},
					gwapiv1.ParentReference{
						Name:      "gateway-b",
						Namespace: ptr.To(gwapiv1.Namespace("gw-ns")),
						Group:     ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
						Kind:      ptr.To(gwapiv1.Kind("Gateway")),
					},
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("gateway-a",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "https",
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     443,
					}),
					WithAddresses("gateway-a.example.com"),
				),
				Gateway("gateway-b",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     8080,
					}),
					WithAddresses("gateway-b.example.com"),
				),
			},
			assert: expectURLsContain(
				"https://gateway-a.example.com/",
				"http://gateway-b.example.com:8080/",
			),
		},
		{
			name: "parent ref without namespace - use route namespace",
			route: HTTPRoute("no-ns-route",
				InNamespace[*gwapiv1.HTTPRoute]("route-ns"),
				WithParentRef(GatewayRef("same-ns-gateway")),
			),
			gateways: []*gwapiv1.Gateway{HTTPGateway("same-ns-gateway", "route-ns", "203.0.113.1")},
			assert:   expectURLs("http://203.0.113.1/"),
		},

		// ===== Address types =====
		{
			name: "private addresses only - still returns URLs",
			route: HTTPRoute("private-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("private-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("private-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("192.168.1.100", "10.0.0.50"),
				),
			},
			assert: expectURLs("http://10.0.0.50/", "http://192.168.1.100/"),
		},
		{
			name: "hostname addresses - basic resolution",
			route: HTTPRoute("hostname-addr-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("hostname-addr-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("hostname-addr-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithHostnameAddresses("api.example.com", "lb.example.com"),
				),
			},
			assert: expectURLs("http://api.example.com/", "http://lb.example.com/"),
		},
		{
			name: "mixed hostname and IP addresses - deterministic selection",
			route: HTTPRoute("mixed-addr-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("mixed-addr-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("mixed-addr-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithMixedAddresses(
						IPAddress("203.0.113.1"),
						HostnameAddress("api.example.com"),
						HostnameAddress("lb.example.com"),
					),
				),
			},
			assert: expectURLs(
				"http://203.0.113.1/",
				"http://api.example.com/",
				"http://lb.example.com/",
			),
		},
		// ===== Error cases =====
		{
			name: "gateway not found should cause not found error",
			route: HTTPRoute("nonexistent-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("nonexistent-gateway", RefInNamespace("test-ns"))),
			),
			gateways: nil,
			assert:   expectError(apierrors.IsNotFound),
		},
		{
			name: "no addresses at all - NoURLsDiscoveredError",
			route: HTTPRoute("no-addr-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("no-addr-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("no-addr-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
				),
			},
			assert: expectError(llmisvc.IsNoURLsDiscovered),
		},
		{
			name: "gateway with only TCP listeners returns error",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "tcp-listener",
						Protocol: gwapiv1.TCPProtocolType,
						Port:     9000,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectError(func(err error) bool { return err != nil }),
		},

		// ===== Port handling =====
		{
			name: "custom port handling - non-standard HTTP port",
			route: HTTPRoute("custom-port-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("custom-port-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("custom-port-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     8080,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1:8080/"),
		},
		{
			name: "custom port handling - non-standard HTTPS port",
			route: HTTPRoute("custom-https-port-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("custom-https-port-gateway", RefInNamespace("test-ns"))),
				WithHostnames("secure.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("custom-https-port-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     8443,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("https://secure.example.com:8443/"),
		},
		{
			name: "standard ports omitted - HTTP port 80",
			route: HTTPRoute("standard-http-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("standard-http-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("standard-http-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1/"),
		},
		{
			name: "standard ports omitted - HTTPS port 443",
			route: HTTPRoute("standard-https-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("standard-https-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("standard-https-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     443,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("https://203.0.113.1/"),
		},

		// ===== sectionName listener selection =====
		{
			name: "sectionName isolates to specific listener - no leakage from other listeners",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(gwapiv1.ParentReference{
					Name:        "multi-listener-gateway",
					Namespace:   ptr.To(gwapiv1.Namespace("gw-ns")),
					SectionName: ptr.To(gwapiv1.SectionName("http")),
					Group:       ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
					Kind:        ptr.To(gwapiv1.Kind("Gateway")),
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("multi-listener-gateway",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
							Hostname: ptr.To(gwapiv1.Hostname("http.example.com")),
						},
						gwapiv1.Listener{
							Name:     "https",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
							Hostname: ptr.To(gwapiv1.Hostname("https.example.com")),
						},
						gwapiv1.Listener{
							Name:     "internal",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     8080,
							Hostname: ptr.To(gwapiv1.Hostname("internal.example.com")),
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: func(g Gomega, urls []string, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(urls).To(Equal([]string{"http://http.example.com/"}))
				g.Expect(urls).ToNot(ContainElement(ContainSubstring("https.example.com")))
				g.Expect(urls).ToNot(ContainElement(ContainSubstring("internal.example.com")))
			},
		},

		// ===== Comprehensive URL generation =====
		{
			name: "sectionName does not match any listener - should error, not silently use wrong listener",
			route: HTTPRoute("mismatched-section-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(gwapiv1.ParentReference{
					Name:        "mismatched-section-gateway",
					Namespace:   ptr.To(gwapiv1.Namespace("test-ns")),
					SectionName: ptr.To(gwapiv1.SectionName("nonexistent-listener")),
					Group:       ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
					Kind:        ptr.To(gwapiv1.Kind("Gateway")),
				}),
				WithHostnames("api.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("mismatched-section-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http-listener",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
						},
						gwapiv1.Listener{
							Name:     "https-listener",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: func(g Gomega, urls []string, err error) {
				g.Expect(err).To(HaveOccurred())
			},
		},
		{
			name: "gateway with no listeners - should error, not panic",
			route: HTTPRoute("no-listener-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("no-listener-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("no-listener-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					// No WithListener — Gateway has empty Listeners slice
					WithAddresses("203.0.113.1"),
				),
			},
			assert: func(g Gomega, urls []string, err error) {
				g.Expect(err).To(HaveOccurred())
			},
		},
		{
			name: "multiple hostnames and addresses - comprehensive URL generation",
			route: HTTPRoute("comprehensive-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("comprehensive-gateway", RefInNamespace("test-ns"))),
				WithHostnames("api.example.com", "v2.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("comprehensive-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1", "203.0.113.2"),
				),
			},
			assert: expectURLs("http://api.example.com/", "http://v2.example.com/"),
		},

		// ===== Listener hostname fallback =====
		{
			name: "listener hostname fallback - no route hostnames",
			route: HTTPRoute("listener-hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("listener-hostname-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("listener-hostname-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("listener.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://listener.example.com/"),
		},
		{
			name: "listener hostname fallback - route has wildcard hostname",
			route: HTTPRoute("wildcard-listener-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-listener-gateway", RefInNamespace("test-ns"))),
				WithHostnames("*"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("wildcard-listener-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("listener.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://listener.example.com/"),
		},
		{
			name: "listener hostname fallback - route hostname takes precedence",
			route: HTTPRoute("precedence-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("precedence-gateway", RefInNamespace("test-ns"))),
				WithHostnames("route.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("precedence-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("listener.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://route.example.com/"),
		},
		{
			name: "listener hostname fallback - empty listener hostname uses addresses",
			route: HTTPRoute("empty-listener-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("empty-listener-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("empty-listener-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://203.0.113.1/"),
		},

		// ===== Listener wildcard hostname =====
		{
			name: "listener wildcard hostname - basic wildcard expansion",
			route: HTTPRoute("wildcard-expansion-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-expansion-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("wildcard-expansion-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			// Wildcards are expanded to inference.example.com
			assert: expectURLs("http://inference.example.com/"),
		},
		{
			name: "listener wildcard hostname - wildcard with subdomain",
			route: HTTPRoute("subdomain-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("subdomain-wildcard-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("subdomain-wildcard-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("*.api.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			// Wildcards are expanded to inference.api.example.com
			assert: expectURLs("http://inference.api.example.com/"),
		},
		{
			name: "listener wildcard hostname - route hostname takes precedence over wildcard",
			route: HTTPRoute("wildcard-precedence-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-precedence-gateway", RefInNamespace("test-ns"))),
				WithHostnames("specific.example.com"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("wildcard-precedence-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("http://specific.example.com/"),
		},
		{
			name: "listener wildcard hostname - HTTPS with wildcard",
			route: HTTPRoute("https-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("https-wildcard-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("https-wildcard-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     443,
						Hostname: ptr.To(gwapiv1.Hostname("*.secure.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			// HTTPS with wildcard expanded to inference.secure.example.com
			assert: expectURLs("https://inference.secure.example.com/"),
		},
		{
			name: "listener wildcard hostname - custom port with wildcard",
			route: HTTPRoute("custom-port-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("custom-port-wildcard-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("custom-port-wildcard-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     8080,
						Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			// Custom port with wildcard expanded
			assert: expectURLs("http://inference.example.com:8080/"),
		},

		// ===== preferredUrlScheme =====
		{
			name: "preferredUrlScheme=https prioritizes HTTPS listener",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http-listener",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
						},
						gwapiv1.Listener{
							Name:     "https-listener",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "https",
			assert:             expectURLs("https://203.0.113.1/", "http://203.0.113.1/"),
		},
		{
			name: "preferredUrlScheme=http prioritizes HTTP listener",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http-listener",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
						},
						gwapiv1.Listener{
							Name:     "https-listener",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "http",
			assert:             expectURLs("http://203.0.113.1/", "https://203.0.113.1/"),
		},
		{
			name: "preferredUrlScheme mismatch falls back to available listener",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http-listener",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "https",
			assert:             expectURLs("http://203.0.113.1/"),
		},
		{
			name: "preferredUrlScheme matching listener preserves non-standard port",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "https-listener",
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     8443,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "https",
			assert:             expectURLs("https://203.0.113.1:8443/"),
		},
		{
			name: "empty preferredUrlScheme returns ALL listeners (HTTPS first)",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http-listener",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
						},
						gwapiv1.Listener{
							Name:     "https-listener",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "",
			assert:             expectURLs("https://203.0.113.1/", "http://203.0.113.1/"),
		},
		{
			name: "empty preferredUrlScheme falls back to HTTP if no HTTPS",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http-listener",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     8080,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "",
			assert:             expectURLs("http://203.0.113.1:8080/"),
		},
		{
			name: "sectionName takes precedence over preferredUrlScheme",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(gwapiv1.ParentReference{
					Name:        "test-gateway",
					Namespace:   ptr.To(gwapiv1.Namespace("test-ns")),
					SectionName: ptr.To(gwapiv1.SectionName("http-listener")),
					Group:       ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
					Kind:        ptr.To(gwapiv1.Kind("Gateway")),
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http-listener",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
						},
						gwapiv1.Listener{
							Name:     "https-listener",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			preferredUrlScheme: "https", // ignored because sectionName is set
			assert:             expectURLs("http://203.0.113.1/"),
		},
		{
			name: "gateway with multiple HTTP-capable listeners - all listeners advertised",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("multi-listener-gateway", RefInNamespace("gw-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("multi-listener-gateway",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(
						gwapiv1.Listener{
							Name:     "http",
							Protocol: gwapiv1.HTTPProtocolType,
							Port:     80,
							Hostname: ptr.To(gwapiv1.Hostname("api.example.com")),
						},
						gwapiv1.Listener{
							Name:     "https",
							Protocol: gwapiv1.HTTPSProtocolType,
							Port:     443,
							Hostname: ptr.To(gwapiv1.Hostname("api.example.com")),
						},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			assert: expectURLs("https://api.example.com/", "http://api.example.com/"),
		},

		// ===== Internal service discovery =====
		{
			name: "discovers internal URL via gateway label",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gateway-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gateway-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-service",
						Namespace: "gateway-ns",
						Labels: map[string]string{
							"gateway.networking.k8s.io/gateway-name": "my-gateway",
						},
					},
				},
			},
			assert: expectURLs(
				"http://203.0.113.1/",
				"http://gateway-service.gateway-ns.svc.cluster.local/",
			),
		},
		{
			name: "discovers internal URL via same-name service fallback",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gateway-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gateway-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gateway", // Same name as gateway
						Namespace: "gateway-ns",
					},
				},
			},
			assert: expectURLs(
				"http://203.0.113.1/",
				"http://my-gateway.gateway-ns.svc.cluster.local/",
			),
		},
		{
			name: "internal URL includes non-standard port",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gateway-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gateway-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http-alt",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     8080,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-service",
						Namespace: "gateway-ns",
						Labels: map[string]string{
							"gateway.networking.k8s.io/gateway-name": "my-gateway",
						},
					},
				},
			},
			assert: expectURLs(
				"http://203.0.113.1:8080/",
				"http://gateway-service.gateway-ns.svc.cluster.local:8080/",
			),
		},
		{
			name: "internal URL uses same scheme as gateway listener",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gateway-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gateway-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "https",
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     443,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-service",
						Namespace: "gateway-ns",
						Labels: map[string]string{
							"gateway.networking.k8s.io/gateway-name": "my-gateway",
						},
					},
				},
			},
			// Internal URL matches the listener's scheme and port
			assert: expectURLs(
				"https://203.0.113.1/",
				"https://gateway-service.gateway-ns.svc.cluster.local/",
			),
		},
		{
			name: "no internal URL without backing service",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gateway-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gateway-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("203.0.113.1"),
				),
			},
			services: nil,
			assert:   expectURLs("http://203.0.113.1/"),
		},
		{
			name: "no external addresses but backing service exists - returns internal URL only",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gateway-ns"))),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gateway-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     8080,
					}),
					// No addresses - LoadBalancer pending
				),
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-service",
						Namespace: "gateway-ns",
						Labels: map[string]string{
							"gateway.networking.k8s.io/gateway-name": "my-gateway",
						},
					},
				},
			},
			assert: expectURLs("http://gateway-service.gateway-ns.svc.cluster.local:8080/"),
		},
		{
			name: "multiple gateways with backing services - each gets internal URL",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(
					gwapiv1.ParentReference{
						Name:      "gateway-a",
						Namespace: ptr.To(gwapiv1.Namespace("gw-ns")),
						Group:     ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
						Kind:      ptr.To(gwapiv1.Kind("Gateway")),
					},
					gwapiv1.ParentReference{
						Name:      "gateway-b",
						Namespace: ptr.To(gwapiv1.Namespace("gw-ns")),
						Group:     ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
						Kind:      ptr.To(gwapiv1.Kind("Gateway")),
					},
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("gateway-a",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "https",
						Protocol: gwapiv1.HTTPSProtocolType,
						Port:     443,
					}),
					WithAddresses("gw-a.example.com"),
				),
				Gateway("gateway-b",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(gwapiv1.Listener{
						Name:     "http",
						Protocol: gwapiv1.HTTPProtocolType,
						Port:     80,
					}),
					WithAddresses("gw-b.example.com"),
				),
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-a",
						Namespace: "gw-ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-b-svc",
						Namespace: "gw-ns",
						Labels: map[string]string{
							"gateway.networking.k8s.io/gateway-name": "gateway-b",
						},
					},
				},
			},
			assert: expectURLsContain(
				"https://gw-a.example.com/",
				"https://gateway-a.gw-ns.svc.cluster.local/",
				"http://gw-b.example.com/",
				"http://gateway-b-svc.gw-ns.svc.cluster.local/",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			scheme := runtime.NewScheme()
			g.Expect(gwapiv1.Install(scheme)).To(Succeed())
			g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

			var objects []client.Object
			if tt.route != nil {
				objects = append(objects, tt.route)
			}
			for _, gw := range tt.gateways {
				objects = append(objects, gw)
			}
			for _, svc := range tt.services {
				objects = append(objects, svc)
			}
			objects = append(objects, DefaultGatewayClass())

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			urls, err := llmisvc.DiscoverURLs(ctx, fakeClient, tt.route, tt.preferredUrlScheme)

			var actualURLs []string
			for _, url := range urls {
				actualURLs = append(actualURLs, url.String())
			}

			tt.assert(g, actualURLs, err)
		})
	}
}

func TestFilterURLs(t *testing.T) {
	convertToURLs := func(urls []string) ([]*apis.URL, error) {
		var parsedURLs []*apis.URL
		for _, urlStr := range urls {
			url, err := apis.ParseURL(urlStr)
			if err != nil {
				return nil, err
			}
			parsedURLs = append(parsedURLs, url)
		}

		return parsedURLs, nil
	}
	t.Run("mixed internal and external URLs", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{
			"http://192.168.1.10/",
			"http://api.example.com/",
			"http://10.0.0.20/",
			"https://secure.example.com/",
			"http://localhost/",
			"http://203.0.113.1/",
		}
		expectedInternal := []string{
			"http://192.168.1.10/",
			"http://10.0.0.20/",
			"http://localhost/",
		}
		expectedExternal := []string{
			"http://api.example.com/",
			"https://secure.example.com/",
			"http://203.0.113.1/",
		}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("URLs with custom ports", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{
			"http://192.168.1.10:8080/",
			"http://api.example.com:8080/",
			"https://secure.example.com:8443/",
			"http://localhost:3000/",
		}
		expectedInternal := []string{
			"http://192.168.1.10:8080/",
			"http://localhost:3000/",
		}
		expectedExternal := []string{
			"http://api.example.com:8080/",
			"https://secure.example.com:8443/",
		}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("internal hostname types", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{
			"http://localhost/",
			"http://service.local/",
			"http://app.localhost/",
			"http://backend.internal/",
			"http://api.example.com/",
		}
		expectedInternal := []string{
			"http://localhost/",
			"http://service.local/",
			"http://app.localhost/",
			"http://backend.internal/",
		}
		expectedExternal := []string{
			"http://api.example.com/",
		}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("all internal URLs", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{
			"http://192.168.1.10/",
			"http://10.0.0.20/",
			"http://localhost/",
		}
		expectedInternal := []string{
			"http://192.168.1.10/",
			"http://10.0.0.20/",
			"http://localhost/",
		}
		expectedExternal := []string{}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("all external URLs", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{
			"http://api.example.com/",
			"https://secure.example.com/",
			"http://203.0.113.1/",
		}
		expectedInternal := []string{}
		expectedExternal := []string{
			"http://api.example.com/",
			"https://secure.example.com/",
			"http://203.0.113.1/",
		}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("empty URL slice", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{}
		expectedInternal := []string{}
		expectedExternal := []string{}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("URLs with paths", func(t *testing.T) {
		g := NewGomegaWithT(t)
		inputURLs := []string{
			"http://192.168.1.10/api/v1/models",
			"http://api.example.com/api/v1/models",
			"http://localhost:8080/health",
		}
		expectedInternal := []string{
			"http://192.168.1.10/api/v1/models",
			"http://localhost:8080/health",
		}
		expectedExternal := []string{
			"http://api.example.com/api/v1/models",
		}

		parsedURLs, err := convertToURLs(inputURLs)
		g.Expect(err).ToNot(HaveOccurred())

		internalURLs := llmisvc.FilterInternalURLs(parsedURLs)
		actualInternal := make([]string, 0, len(internalURLs))
		for _, url := range internalURLs {
			actualInternal = append(actualInternal, url.String())
		}
		g.Expect(actualInternal).To(Equal(expectedInternal))

		externalURLs := llmisvc.FilterExternalURLs(parsedURLs)
		actualExternal := make([]string, 0, len(externalURLs))
		for _, url := range externalURLs {
			actualExternal = append(actualExternal, url.String())
		}
		g.Expect(actualExternal).To(Equal(expectedExternal))
	})

	t.Run("IsInternalURL and IsExternalURL are opposites", func(t *testing.T) {
		g := NewGomegaWithT(t)
		testURLs := []string{
			"http://192.168.1.10/",
			"http://api.example.com/",
			"http://localhost/",
			"https://secure.example.com:8443/",
		}

		for _, urlStr := range testURLs {
			url, err := apis.ParseURL(urlStr)
			g.Expect(err).ToNot(HaveOccurred())

			isInternal := llmisvc.IsInternalURL(url)
			isExternal := llmisvc.IsExternalURL(url)

			g.Expect(isInternal).To(Equal(!isExternal), "URL %s should be either internal or external, not both", urlStr)
		}
	})

	t.Run("AddressTypeName", func(t *testing.T) {
		tests := []struct {
			name     string
			url      string
			expected string
		}{
			{
				name:     "external hostname",
				url:      "https://api.example.com/",
				expected: "gateway-external",
			},
			{
				name:     "external IP",
				url:      "http://203.0.113.1/",
				expected: "gateway-external",
			},
			{
				name:     "private IP",
				url:      "http://192.168.1.100/",
				expected: "internal",
			},
			{
				name:     "localhost",
				url:      "http://localhost/",
				expected: "internal",
			},
			{
				name:     "cluster-local service",
				url:      "http://my-service.default.svc.cluster.local/",
				expected: "gateway-internal",
			},
			{
				name:     "cluster-local with port",
				url:      "http://gateway.ns.svc.cluster.local:8080/",
				expected: "gateway-internal",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewGomegaWithT(t)
				parsedURL, err := apis.ParseURL(tt.url)
				g.Expect(err).ToNot(HaveOccurred())

				result := llmisvc.AddressTypeName(parsedURL)
				g.Expect(result).To(Equal(tt.expected))
			})
		}
	})
}

func TestIsInferencePoolV1Alpha2Supported(t *testing.T) {
	v1alpha2Group := gwapiv1.Group("inference.networking.x-k8s.io")
	v1Group := gwapiv1.Group("inference.networking.k8s.io")
	poolKind := gwapiv1.Kind("InferencePool")

	tests := []struct {
		name     string
		route    *gwapiv1.HTTPRoute
		expected metav1.ConditionStatus
	}{
		{
			name:     "nil route returns Unknown",
			route:    nil,
			expected: metav1.ConditionUnknown,
		},
		{
			name: "route not using v1alpha2 InferencePool returns Unknown",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						{
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{
									Group: &v1Group,
									Kind:  &poolKind,
									Name:  "test-pool",
								}}},
							},
						},
					},
				},
			},
			expected: metav1.ConditionUnknown,
		},
		{
			name: "route using v1alpha2 with no status returns Unknown",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						{
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{
									Group: &v1alpha2Group,
									Kind:  &poolKind,
									Name:  "test-pool",
								}}},
							},
						},
					},
				},
			},
			expected: metav1.ConditionUnknown,
		},
		{
			name: "route using v1alpha2 with ResolvedRefs=True returns True (supported)",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						{
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{
									Group: &v1alpha2Group,
									Kind:  &poolKind,
									Name:  "test-pool",
								}}},
							},
						},
					},
				},
				Status: gwapiv1.HTTPRouteStatus{
					RouteStatus: gwapiv1.RouteStatus{
						Parents: []gwapiv1.RouteParentStatus{
							{
								Conditions: []metav1.Condition{
									{
										Type:   string(gwapiv1.RouteConditionResolvedRefs),
										Status: metav1.ConditionTrue,
										Reason: "ResolvedRefs",
									},
								},
							},
						},
					},
				},
			},
			expected: metav1.ConditionTrue,
		},
		{
			name: "route using v1alpha2 with ResolvedRefs=False/InvalidKind returns False (rejected)",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						{
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{
									Group: &v1alpha2Group,
									Kind:  &poolKind,
									Name:  "test-pool",
								}}},
							},
						},
					},
				},
				Status: gwapiv1.HTTPRouteStatus{
					RouteStatus: gwapiv1.RouteStatus{
						Parents: []gwapiv1.RouteParentStatus{
							{
								Conditions: []metav1.Condition{
									{
										Type:    string(gwapiv1.RouteConditionResolvedRefs),
										Status:  metav1.ConditionFalse,
										Reason:  "InvalidKind",
										Message: "Group is invalid: inference.networking.x-k8s.io",
									},
								},
							},
						},
					},
				},
			},
			expected: metav1.ConditionFalse,
		},
		{
			name: "route using v1alpha2 with ResolvedRefs=False but different reason returns True (not InvalidKind rejection)",
			route: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						{
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{
									Group: &v1alpha2Group,
									Kind:  &poolKind,
									Name:  "test-pool",
								}}},
							},
						},
					},
				},
				Status: gwapiv1.HTTPRouteStatus{
					RouteStatus: gwapiv1.RouteStatus{
						Parents: []gwapiv1.RouteParentStatus{
							{
								Conditions: []metav1.Condition{
									{
										Type:   string(gwapiv1.RouteConditionResolvedRefs),
										Status: metav1.ConditionFalse,
										Reason: "BackendNotFound",
									},
								},
							},
						},
					},
				},
			},
			expected: metav1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := llmisvc.IsInferencePoolV1Alpha2Supported(tt.route)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}
