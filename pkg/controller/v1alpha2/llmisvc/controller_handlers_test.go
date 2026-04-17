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

package llmisvc

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestHasRoutingGatewayRef(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		Status: v1alpha2.LLMInferenceServiceStatus{
			Routing: &v1alpha2.RoutingStatus{
				Gateways: []v1alpha2.ObservedGateway{
					{
						ObjectReference: gwapiv1.ObjectReference{
							Name:      "gateway-a",
							Namespace: ptr.To(gwapiv1.Namespace("networking")),
						},
					},
					{
						ObjectReference: gwapiv1.ObjectReference{
							Name:      "gateway-b",
							Namespace: ptr.To(gwapiv1.Namespace("kserve")),
						},
					},
				},
			},
		},
	}

	g.Expect(hasRoutingGatewayRef(llmSvc, gwapiv1.ObjectName("gateway-b"), gwapiv1.Namespace("kserve"))).To(BeTrue())
	g.Expect(hasRoutingGatewayRef(llmSvc, gwapiv1.ObjectName("gateway-b"), gwapiv1.Namespace("networking"))).To(BeFalse())
	g.Expect(hasRoutingGatewayRef(llmSvc, gwapiv1.ObjectName("gateway-missing"), gwapiv1.Namespace("kserve"))).To(BeFalse())
}

func TestHasRoutingGatewayRefReturnsFalseWithoutObservedGateways(t *testing.T) {
	g := NewGomegaWithT(t)

	g.Expect(hasRoutingGatewayRef(&v1alpha2.LLMInferenceService{}, gwapiv1.ObjectName("gateway"), gwapiv1.Namespace("default"))).To(BeFalse())
	g.Expect(hasRoutingGatewayRef(&v1alpha2.LLMInferenceService{
		Status: v1alpha2.LLMInferenceServiceStatus{
			Routing: &v1alpha2.RoutingStatus{},
		},
	}, gwapiv1.ObjectName("gateway"), gwapiv1.Namespace("default"))).To(BeFalse())
}

func TestHasRoutingHTTPRouteRef(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "routing",
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Routing: &v1alpha2.RoutingStatus{
				Gateways: []v1alpha2.ObservedGateway{
					{
						HTTPRoutes: []gwapiv1.ObjectReference{
							{
								Name: "llm-route-missing",
								Kind: "HTTPRoute",
							},
							{
								Name: "llm-route-match",
								Kind: "HTTPRoute",
							},
						},
					},
				},
			},
		},
	}

	g.Expect(hasRoutingHTTPRouteRef(llmSvc, gwapiv1.ObjectName("llm-route-match"), "routing")).To(BeTrue())
	g.Expect(hasRoutingHTTPRouteRef(llmSvc, gwapiv1.ObjectName("llm-route-match"), "other")).To(BeFalse())
}

func TestHasRoutingHTTPRouteRefReturnsFalseWithoutObservedRoutes(t *testing.T) {
	g := NewGomegaWithT(t)

	g.Expect(hasRoutingHTTPRouteRef(&v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "routing",
		},
	}, gwapiv1.ObjectName("llm-route"), "routing")).To(BeFalse())
}

func TestSetRoutingPoolStatusOnlyWritesWhenRoutingExists(t *testing.T) {
	g := NewGomegaWithT(t)
	llmSvc := &v1alpha2.LLMInferenceService{}
	poolRef := gwapiv1.ObjectReference{
		Group: "inference.networking.k8s.io",
		Kind:  "InferencePool",
		Name:  "managed-pool",
	}
	svcRef := gwapiv1.ObjectReference{
		Kind: "Service",
		Name: "epp-service",
	}

	setRoutingPoolStatus(llmSvc, poolRef, svcRef)
	g.Expect(llmSvc.Status.Routing).To(BeNil())

	llmSvc.Status.Routing = &v1alpha2.RoutingStatus{
		Gateways: []v1alpha2.ObservedGateway{
			{
				ObjectReference: gwapiv1.ObjectReference{
					Group: "gateway.networking.k8s.io",
					Kind:  "Gateway",
					Name:  "kserve-gateway",
				},
			},
		},
	}
	setRoutingPoolStatus(llmSvc, poolRef, svcRef)
	g.Expect(llmSvc.Status.Routing).ToNot(BeNil())
	g.Expect(llmSvc.Status.Routing.InferencePool).ToNot(BeNil())
	g.Expect(string(llmSvc.Status.Routing.InferencePool.Name)).To(Equal("managed-pool"))
	g.Expect(llmSvc.Status.Routing.SchedulerService).ToNot(BeNil())
	g.Expect(string(llmSvc.Status.Routing.SchedulerService.Name)).To(Equal("epp-service"))
}
