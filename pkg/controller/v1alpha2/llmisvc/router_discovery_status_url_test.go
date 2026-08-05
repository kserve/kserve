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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// TestStatusURLStability is a characterization test that verifies status.url
// remains stable across the multi-listener internal URL change. The expected
// values (expectedStatusURL) were captured by running this test against master
// before the change, establishing the baseline behavior that must be preserved.
//
// Unlike earlier iterations that simulated the pipeline with inline helpers,
// this test calls the actual updateRoutingStatus production code path through
// an exported test shim, ensuring full fidelity with the real controller.
func TestStatusURLStability(t *testing.T) {
	tests := []struct {
		name              string
		route             *gwapiv1.HTTPRoute
		gateways          []*gwapiv1.Gateway
		services          []*corev1.Service
		urlScheme         string
		expectedStatusURL string
		expectedAddrCount int
	}{
		{
			// Baseline captured from master: old code used listeners[0] (HTTPS,
			// sorted first by selectListeners when preferredScheme=https) to
			// produce a single internal URL. New code generates URLs for all
			// listeners; status.url must still select the HTTPS one.
			name: "multi-listener HTTPS preferred - no external addresses",
			route: HTTPRoute("test-svc",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gw-ns"))),
				WithPath("/test-ns/test-svc"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(
						gwapiv1.Listener{Name: "http", Protocol: gwapiv1.HTTPProtocolType, Port: 80},
						gwapiv1.Listener{Name: "https", Protocol: gwapiv1.HTTPSProtocolType, Port: 443},
					),
				),
			},
			services: []*corev1.Service{
				backingService("gw-ns", "my-gateway",
					corev1.ServicePort{Name: "http", Port: 80},
					corev1.ServicePort{Name: "https", Port: 443},
				),
			},
			urlScheme:         "https",
			expectedStatusURL: "https://my-gateway-svc.gw-ns.svc.cluster.local/test-ns/test-svc",
			expectedAddrCount: 2,
		},
		{
			// Default config (urlScheme="http"). Alphabetical sort naturally
			// picks http:// first, matching the old single-listener behavior.
			name: "multi-listener HTTP preferred (default) - no external addresses",
			route: HTTPRoute("test-svc",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gw-ns"))),
				WithPath("/test-ns/test-svc"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(
						gwapiv1.Listener{Name: "http", Protocol: gwapiv1.HTTPProtocolType, Port: 80},
						gwapiv1.Listener{Name: "https", Protocol: gwapiv1.HTTPSProtocolType, Port: 443},
					),
				),
			},
			services: []*corev1.Service{
				backingService("gw-ns", "my-gateway",
					corev1.ServicePort{Name: "http", Port: 80},
					corev1.ServicePort{Name: "https", Port: 443},
				),
			},
			urlScheme:         "http",
			expectedStatusURL: "http://my-gateway-svc.gw-ns.svc.cluster.local/test-ns/test-svc",
			expectedAddrCount: 2,
		},
		{
			// Baseline captured from master: external URL selection uses
			// alphabetical ordering without scheme preference to avoid
			// changing status.url on upgrade. This means http:// wins
			// over https:// even when urlScheme is configured.
			name: "multi-listener HTTPS preferred - with external addresses",
			route: HTTPRoute("test-svc",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gw-ns"))),
				WithPath("/test-ns/test-svc"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(
						gwapiv1.Listener{Name: "http", Protocol: gwapiv1.HTTPProtocolType, Port: 80},
						gwapiv1.Listener{Name: "https", Protocol: gwapiv1.HTTPSProtocolType, Port: 443},
					),
					WithAddresses("203.0.113.1"),
				),
			},
			services: []*corev1.Service{
				backingService("gw-ns", "my-gateway",
					corev1.ServicePort{Name: "http", Port: 80},
					corev1.ServicePort{Name: "https", Port: 443},
				),
			},
			urlScheme:         "https",
			expectedStatusURL: "http://203.0.113.1/test-ns/test-svc",
			expectedAddrCount: 4,
		},
		{
			// Single listener is trivially stable: only one URL to choose from.
			name: "single listener - no external addresses",
			route: HTTPRoute("test-svc",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("my-gateway", RefInNamespace("gw-ns"))),
				WithPath("/test-ns/test-svc"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway]("gw-ns"),
					WithListeners(
						gwapiv1.Listener{Name: "https", Protocol: gwapiv1.HTTPSProtocolType, Port: 443},
					),
				),
			},
			services: []*corev1.Service{
				backingService("gw-ns", "my-gateway",
					corev1.ServicePort{Name: "https", Port: 443},
				),
			},
			urlScheme:         "https",
			expectedStatusURL: "https://my-gateway-svc.gw-ns.svc.cluster.local/test-ns/test-svc",
			expectedAddrCount: 1,
		},
		{
			// Scheme mismatch: urlScheme says "https" but gateway only has
			// an HTTP listener. No URL matches the preferred scheme, so
			// preferPathBasedURL falls back to the first path match.
			name: "scheme mismatch - HTTPS preferred but only HTTP listener",
			route: HTTPRoute("test-svc",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("http-only-gw", RefInNamespace("infra-ns"))),
				WithPath("/test-ns/test-svc"),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("http-only-gw",
					InNamespace[*gwapiv1.Gateway]("infra-ns"),
					WithListeners(
						gwapiv1.Listener{Name: "http", Protocol: gwapiv1.HTTPProtocolType, Port: 80},
					),
				),
			},
			services: []*corev1.Service{
				backingService("infra-ns", "http-only-gw",
					corev1.ServicePort{Name: "http", Port: 80},
				),
			},
			urlScheme:         "https",
			expectedStatusURL: "http://my-gateway-svc.infra-ns.svc.cluster.local/test-ns/test-svc",
			expectedAddrCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			s := runtime.NewScheme()
			g.Expect(gwapiv1.Install(s)).To(Succeed())
			g.Expect(corev1.AddToScheme(s)).To(Succeed())
			g.Expect(v1alpha2.AddToScheme(s)).To(Succeed())

			var objects []client.Object
			objects = append(objects, tt.route)
			for _, gw := range tt.gateways {
				objects = append(objects, gw)
			}
			for _, svc := range tt.services {
				objects = append(objects, svc)
			}
			objects = append(objects, DefaultGatewayClass())
			objects = append(objects, inferenceServiceConfigMap(tt.urlScheme))

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(objects...).
				Build()

			reconciler := &llmisvc.LLMISVCReconciler{
				Client: fakeClient,
			}

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "test-ns",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{},
					},
				},
			}

			_, err := llmisvc.UpdateRoutingStatusForTest(ctx, reconciler, llmSvc, tt.route)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(llmSvc.Status.URL).ToNot(BeNil(), "status.url should be set")
			g.Expect(llmSvc.Status.URL.String()).To(Equal(tt.expectedStatusURL),
				"status.url must match the baseline captured from master")

			g.Expect(llmSvc.Status.Addresses).To(HaveLen(tt.expectedAddrCount),
				"status.addresses count must be stable")
		})
	}
}

func inferenceServiceConfigMap(urlScheme string) *corev1.ConfigMap {
	cm := InferenceServiceCfgMapWithUrlScheme(constants.KServeNamespace, urlScheme)
	cm.Namespace = constants.KServeNamespace
	return cm
}

// backingService creates a corev1.Service with the gateway label that
// DiscoverGatewayService uses for discovery.
func backingService(namespace, gatewayName string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gateway-svc",
			Namespace: namespace,
			Labels: map[string]string{
				"gateway.networking.k8s.io/gateway-name": gatewayName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: ports,
		},
	}
}
