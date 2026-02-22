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

func TestDiscoverURLs(t *testing.T) {
	tests := []struct {
		name               string
		route              *gwapiv1.HTTPRoute
		gateway            *gwapiv1.Gateway
		additionalGateways []*gwapiv1.Gateway // Additional gateways for multiple parent refs test
		expectedURLs       []string           // Always expect multiple URLs, single URL cases will have length 1
		expectedErrorCheck func(error) bool
	}{
		{
			name: "basic external address resolution",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
			),
			gateway:      HTTPGateway("test-gateway", "test-ns", "203.0.113.1"),
			expectedURLs: []string{"http://203.0.113.1/"},
		},
		{
			name: "address ordering consistency - same addresses different order",
			route: HTTPRoute("consistency-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("consistency-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("consistency-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses([]string{"203.0.113.200", "203.0.113.100"}...),
			),
			expectedURLs: []string{
				"http://203.0.113.100/",
				"http://203.0.113.200/",
			},
		},
		{
			name: "mixed internal and external addresses - deterministic selection",
			route: HTTPRoute("mixed-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("mixed-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("mixed-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("192.168.1.10", "203.0.113.50", "10.0.0.20", "203.0.113.25"),
			),
			expectedURLs: []string{
				"http://10.0.0.20/",
				"http://192.168.1.10/",
				"http://203.0.113.25/",
				"http://203.0.113.50/",
			},
		},
		{
			name: "route hostname override",
			route: HTTPRoute("hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("hostname-gateway", RefInNamespace("test-ns"))),
				WithHostnames("api.example.com"),
			),
			gateway: Gateway("hostname-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses([]string{"203.0.113.1"}...),
			),
			expectedURLs: []string{"http://api.example.com/"},
		},
		{
			name: "route wildcard hostname - use gateway address",
			route: HTTPRoute("wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-gateway", RefInNamespace("test-ns"))),
				WithHostnames("*"),
			),
			gateway: Gateway("wildcard-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.100"),
			),
			expectedURLs: []string{"http://203.0.113.100/"},
		},
		{
			name: "multiple hostnames - generates multiple URLs",
			route: HTTPRoute("multi-hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("multi-hostname-gateway", RefInNamespace("test-ns"))),
				WithHostnames("*", "", "api.example.com", "alt.example.com"),
			),
			gateway: Gateway("multi-hostname-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{
				"http://alt.example.com/",
				"http://api.example.com/",
			},
		},
		{
			name: "custom path extraction",
			route: HTTPRoute("path-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("path-gateway", RefInNamespace("test-ns"))),
				WithPath("/api/v1/models"),
			),
			gateway: Gateway("path-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/api/v1/models"},
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
			gateway: Gateway("multi-rule-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/ns/name"},
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
			gateway: Gateway("no-svc-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/ns/name/v1/completions"},
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
			gateway: Gateway("default-kind-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/ns/name"},
		},
		{
			name: "HTTPS scheme from gateway listener",
			route: HTTPRoute("https-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("https-gateway", RefInNamespace("test-ns"))),
			),
			gateway:      HTTPSGateway("https-gateway", "test-ns", "203.0.113.1"),
			expectedURLs: []string{"https://203.0.113.1/"},
		},
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
			gateway: Gateway("a-gateway",
				InNamespace[*gwapiv1.Gateway]("a-namespace"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses([]string{"203.0.113.1"}...),
			),
			additionalGateways: []*gwapiv1.Gateway{
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
			expectedURLs: []string{
				"http://203.0.113.2/",
				"http://203.0.113.1/",
				"http://203.0.113.3/",
			},
		},
		{
			name: "parent ref without namespace - use route namespace",
			route: HTTPRoute("no-ns-route",
				InNamespace[*gwapiv1.HTTPRoute]("route-ns"),
				WithParentRef(GatewayRefWithoutNamespace("no-ns-gateway")),
			),
			gateway: Gateway("no-ns-gateway",
				InNamespace[*gwapiv1.Gateway]("route-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/"},
		},
		{
			name: "no external addresses - custom ExternalAddressNotFoundError",
			route: HTTPRoute("no-external-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("no-external-addresses-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("no-external-addresses-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("192.168.1.10", "10.0.0.20"),
			),
			expectedURLs: []string{
				"http://10.0.0.20/",
				"http://192.168.1.10/",
			},
		},
		{
			name: "gateway not found should cause not found error",
			route: HTTPRoute("missing-gw-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("missing-gateway", RefInNamespace("test-ns"))),
			),
			expectedErrorCheck: apierrors.IsNotFound,
		},
		{
			name: "empty route rules - default path",
			route: HTTPRoute("empty-rules-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("empty-rules-gateway", RefInNamespace("test-ns"))),
				WithRules(), // Empty rules
			),
			gateway: Gateway("empty-rules-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/"},
		},
		// Hostname address type tests
		{
			name: "hostname addresses - basic resolution",
			route: HTTPRoute("hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("hostname-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("hostname-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithHostnameAddresses("api.example.com"),
			),
			expectedURLs: []string{"http://api.example.com/"},
		},
		{
			name: "mixed hostname and IP addresses - deterministic selection",
			route: HTTPRoute("mixed-types-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("mixed-types-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("mixed-types-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithMixedAddresses(
					HostnameAddress("z.example.com"),
					IPAddress("203.0.113.1"),
					HostnameAddress("api.example.com"),
					IPAddress("198.51.100.1"),
				),
			),
			expectedURLs: []string{
				"http://198.51.100.1/",
				"http://203.0.113.1/",
				"http://api.example.com/",
				"http://z.example.com/",
			},
		},
		{
			name: "hostname addresses with internal hostnames filtered",
			route: HTTPRoute("internal-hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("internal-hostname-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("internal-hostname-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithMixedAddresses(
					HostnameAddress("localhost"),
					HostnameAddress("service.local"),
					HostnameAddress("app.internal"),
					HostnameAddress("api.example.com"),
					HostnameAddress("backup.example.com"),
				),
			),
			expectedURLs: []string{
				"http://api.example.com/",
				"http://app.internal/",
				"http://backup.example.com/",
				"http://localhost/",
				"http://service.local/",
			},
		},
		{
			name: "only internal addresses (IP + hostnames) - ExternalAddressNotFoundError",
			route: HTTPRoute("only-internal-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("only-internal-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("only-internal-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithMixedAddresses(
					IPAddress("192.168.1.10"),
					IPAddress("10.0.0.20"),
					HostnameAddress("localhost"),
					HostnameAddress("app.local"),
				),
			),
			expectedURLs: []string{
				"http://10.0.0.20/",
				"http://192.168.1.10/",
				"http://app.local/",
				"http://localhost/",
			},
		},
		{
			name: "backwards compatibility - nil Type defaults to IP behavior",
			route: HTTPRoute("nil-type-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("nil-type-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("nil-type-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1", "192.168.1.10"),
			),
			expectedURLs: []string{
				"http://192.168.1.10/",
				"http://203.0.113.1/",
			},
		},
		{
			name: "no addresses at all - ExternalAddressNotFoundError",
			route: HTTPRoute("no-addresses-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("no-addresses-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("no-addresses-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
			),
			expectedErrorCheck: llmisvc.IsExternalAddressNotFound,
		},
		{
			name: "custom port handling - non-standard HTTP port",
			route: HTTPRoute("custom-port-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("custom-port-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("custom-port-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     8080,
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1:8080/"},
		},
		{
			name: "custom port handling - non-standard HTTPS port",
			route: HTTPRoute("custom-https-port-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("custom-https-port-gateway", RefInNamespace("test-ns"))),
				WithHostnames("secure.example.com"),
			),
			gateway: Gateway("custom-https-port-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPSProtocolType,
					Port:     8443,
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"https://secure.example.com:8443/"},
		},
		{
			name: "standard ports omitted - HTTP port 80",
			route: HTTPRoute("standard-http-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("standard-http-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("standard-http-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/"},
		},
		{
			name: "standard ports omitted - HTTPS port 443",
			route: HTTPRoute("standard-https-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("standard-https-gateway", RefInNamespace("test-ns"))),
				WithHostnames("secure.example.com"),
			),
			gateway: Gateway("standard-https-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPSProtocolType,
					Port:     443,
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"https://secure.example.com/"},
		},
		{
			name: "sectionName selects specific listener",
			route: HTTPRoute("section-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(gwapiv1.ParentReference{
					Name:        "multi-listener-gateway",
					Namespace:   ptr.To(gwapiv1.Namespace("test-ns")),
					SectionName: ptr.To(gwapiv1.SectionName("https-listener")),
					Group:       ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
					Kind:        ptr.To(gwapiv1.Kind("Gateway")),
				}),
				WithHostnames("secure.example.com"),
			),
			gateway: Gateway("multi-listener-gateway",
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
			expectedURLs: []string{"https://secure.example.com/"},
		},
		{
			name: "multiple hostnames and addresses - comprehensive URL generation",
			route: HTTPRoute("comprehensive-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("comprehensive-gateway", RefInNamespace("test-ns"))),
				WithHostnames("api.example.com", "backup.example.com", "primary.example.com"),
			),
			gateway: Gateway("comprehensive-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.1", "198.51.100.1"),
			),
			expectedURLs: []string{
				"http://api.example.com/",
				"http://backup.example.com/",
				"http://primary.example.com/",
			},
		},
		{
			name: "listener hostname fallback - no route hostnames",
			route: HTTPRoute("listener-hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("listener-hostname-gateway", RefInNamespace("test-ns"))),
				// No hostnames specified in route
			),
			gateway: Gateway("listener-hostname-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("listener.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://listener.example.com/"},
		},
		{
			name: "listener hostname fallback - route has wildcard hostname",
			route: HTTPRoute("listener-hostname-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("listener-hostname-wildcard-gateway", RefInNamespace("test-ns"))),
				WithHostnames("*"), // Wildcard should be filtered out
			),
			gateway: Gateway("listener-hostname-wildcard-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("fallback.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://fallback.example.com/"},
		},
		{
			name: "listener hostname fallback - route hostname takes precedence",
			route: HTTPRoute("route-hostname-precedence",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("route-hostname-precedence-gateway", RefInNamespace("test-ns"))),
				WithHostnames("route.example.com"),
			),
			gateway: Gateway("route-hostname-precedence-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("listener.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://route.example.com/"}, // Route hostname should be used, not listener
		},
		{
			name: "listener hostname fallback - empty listener hostname uses addresses",
			route: HTTPRoute("empty-listener-hostname-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("empty-listener-hostname-gateway", RefInNamespace("test-ns"))),
				// No hostnames specified in route
			),
			gateway: Gateway("empty-listener-hostname-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("")), // Empty hostname
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://203.0.113.1/"}, // Should fall back to addresses
		},
		{
			name: "listener wildcard hostname - basic wildcard expansion",
			route: HTTPRoute("wildcard-listener-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-listener-gateway", RefInNamespace("test-ns"))),
				// No hostnames specified in route
			),
			gateway: Gateway("wildcard-listener-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://inference.example.com/"}, // Should expand wildcard to inference.example.com
		},
		{
			name: "listener wildcard hostname - wildcard with subdomain",
			route: HTTPRoute("wildcard-subdomain-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("wildcard-subdomain-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("wildcard-subdomain-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("*.api.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://inference.api.example.com/"}, // Should expand to inference.api.example.com
		},
		{
			name: "listener wildcard hostname - route hostname takes precedence over wildcard",
			route: HTTPRoute("route-over-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("route-over-wildcard-gateway", RefInNamespace("test-ns"))),
				WithHostnames("custom.example.com"),
			),
			gateway: Gateway("route-over-wildcard-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gwapiv1.Hostname("*.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://custom.example.com/"}, // Route hostname should take precedence
		},
		{
			name: "listener wildcard hostname - HTTPS with wildcard",
			route: HTTPRoute("https-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("https-wildcard-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("https-wildcard-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPSProtocolType,
					Port:     443,
					Hostname: ptr.To(gwapiv1.Hostname("*.secure.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"https://inference.secure.example.com/"},
		},
		{
			name: "listener wildcard hostname - custom port with wildcard",
			route: HTTPRoute("custom-port-wildcard-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("custom-port-wildcard-gateway", RefInNamespace("test-ns"))),
			),
			gateway: Gateway("custom-port-wildcard-gateway",
				InNamespace[*gwapiv1.Gateway]("test-ns"),
				WithListeners(gwapiv1.Listener{
					Protocol: gwapiv1.HTTPProtocolType,
					Port:     8080,
					Hostname: ptr.To(gwapiv1.Hostname("*.apps.example.com")),
				}),
				WithAddresses("203.0.113.1"),
			),
			expectedURLs: []string{"http://inference.apps.example.com:8080/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			ctx := t.Context()

			scheme := runtime.NewScheme()
			err := gwapiv1.Install(scheme)
			g.Expect(err).ToNot(HaveOccurred())

			var objects []client.Object
			if tt.gateway != nil {
				objects = append(objects, tt.gateway)
			}
			if tt.route != nil {
				objects = append(objects, tt.route)
			}
			for _, gw := range tt.additionalGateways {
				objects = append(objects, gw)
			}
			objects = append(objects, DefaultGatewayClass())

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			urls, err := llmisvc.DiscoverURLs(ctx, fakeClient, tt.route)

			if tt.expectedErrorCheck != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(tt.expectedErrorCheck(err)).To(BeTrue(), "Error check function failed for error: %v", err)
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(urls).To(HaveLen(len(tt.expectedURLs)))

				// Convert to strings for easier comparison
				var actualURLs []string
				for _, url := range urls {
					actualURLs = append(actualURLs, url.String())
				}

				g.Expect(actualURLs).To(Equal(tt.expectedURLs))
			}
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
