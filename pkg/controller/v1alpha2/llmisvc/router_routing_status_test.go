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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// --- helpers ----------------------------------------------------------------

func parentRef(name, namespace string, sectionName *gwapiv1.SectionName) gwapiv1.ParentReference {
	ref := GatewayParentRef(name, namespace)
	ref.SectionName = sectionName
	return ref
}

func observedGateway(name, namespace string, listeners []gwapiv1.SectionName, httpRoutes []gwapiv1.ObjectReference) v1alpha2.ObservedGateway {
	return v1alpha2.ObservedGateway{
		ObjectReference: gwapiv1.ObjectReference{
			Group:     gwapiv1.Group(gwapiv1.GroupName),
			Kind:      "Gateway",
			Name:      gwapiv1.ObjectName(name),
			Namespace: ptr.To(gwapiv1.Namespace(namespace)),
		},
		Listeners:  listeners,
		HTTPRoutes: httpRoutes,
	}
}

func routeRef(name string) gwapiv1.ObjectReference {
	return gwapiv1.ObjectReference{
		Group:     gwapiv1.Group(gwapiv1.GroupName),
		Kind:      "HTTPRoute",
		Name:      gwapiv1.ObjectName(name),
		Namespace: ptr.To(gwapiv1.Namespace("")),
	}
}

// specRefRoutingFixture builds a route-less service referencing the given gateways,
// with a fake client holding one unprogrammed Gateway per distinct ref.
func specRefRoutingFixture(g *WithT, refs ...v1alpha2.GatewayObjectReference) (*llmisvc.LLMISVCReconciler, *v1alpha2.LLMInferenceService) {
	s := runtime.NewScheme()
	g.Expect(gwapiv1.Install(s)).To(Succeed())
	g.Expect(corev1.AddToScheme(s)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(s)).To(Succeed())

	objects := []client.Object{DefaultGatewayClass(), inferenceServiceConfigMap("")}
	seen := map[string]bool{}
	for _, ref := range refs {
		key := string(ref.Namespace) + "/" + string(ref.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		objects = append(objects, Gateway(string(ref.Name), InNamespace[*gwapiv1.Gateway](string(ref.Namespace))))
	}

	reconciler := &llmisvc.LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build(),
	}
	llmSvc := LLMInferenceService("svc",
		InNamespace[*v1alpha2.LLMInferenceService]("ns-a"),
		WithGatewayRefs(refs...),
	)
	return reconciler, llmSvc
}

// --- tests ------------------------------------------------------------------

func TestBuildObservedGateways(t *testing.T) {
	tests := []struct {
		name     string
		input    []*gwapiv1.HTTPRoute
		expected []v1alpha2.ObservedGateway
	}{
		{
			name:     "empty input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice input returns nil",
			input:    []*gwapiv1.HTTPRoute{},
			expected: nil,
		},
		{
			name: "single gateway single listener single route",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("my-route",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
			},
		},
		{
			name: "nil SectionName produces nil Listeners",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("my-route",
					WithParentRef(parentRef("gw-1", "ns-a", nil)),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					nil,
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
			},
		},
		{
			name: "same gateway from two routes deduplicates gateway aggregates routes",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("route-a",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
				HTTPRoute("route-b",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("route-a"), routeRef("route-b")},
				),
			},
		},
		{
			name: "same gateway two different listeners aggregates and sorts",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("my-route",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("https")))),
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					[]gwapiv1.SectionName{"http", "https"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
			},
		},
		{
			name: "duplicate listener from two routes on same gateway is deduplicated",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("route-a",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
				HTTPRoute("route-b",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("route-a"), routeRef("route-b")},
				),
			},
		},
		{
			name: "same route attached to two gateways appears under each",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("my-route",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
					WithParentRef(parentRef("gw-2", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
				observedGateway("gw-2", "ns-a",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
			},
		},
		{
			name: "gateways sorted by namespace/name",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("my-route",
					WithParentRef(parentRef("gw-beta", "ns-b", ptr.To(gwapiv1.SectionName("http")))),
					WithParentRef(parentRef("gw-alpha", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
					WithParentRef(parentRef("gw-alpha", "ns-b", ptr.To(gwapiv1.SectionName("http")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-alpha", "ns-a",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
				observedGateway("gw-alpha", "ns-b",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
				observedGateway("gw-beta", "ns-b",
					[]gwapiv1.SectionName{"http"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
			},
		},
		{
			name: "duplicate route ref from multiple parentRefs not duplicated",
			input: []*gwapiv1.HTTPRoute{
				HTTPRoute("my-route",
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("http")))),
					WithParentRef(parentRef("gw-1", "ns-a", ptr.To(gwapiv1.SectionName("https")))),
				),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a",
					[]gwapiv1.SectionName{"http", "https"},
					[]gwapiv1.ObjectReference{routeRef("my-route")},
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			got := llmisvc.BuildObservedGateways(tt.input)
			g.Expect(got).To(Equal(tt.expected))
		})
	}
}

// TestObservedGatewaysFromSpecRefs covers status.router.gateways when no route exists
// and gateways come from spec.router.gateway.refs. Both paths write the same field, so
// the shape must match BuildObservedGateways: one entry per Gateway. Unlike parentRefs,
// which Gateway API CEL keeps distinct per parent, gateway.refs can repeat a ref or mix
// a whole-Gateway ref with a listener pin on the same Gateway.
func TestObservedGatewaysFromSpecRefs(t *testing.T) {
	tests := []struct {
		name     string
		refs     []v1alpha2.GatewayObjectReference
		expected []v1alpha2.ObservedGateway
	}{
		{
			name:     "unpinned ref targets all listeners",
			refs:     []v1alpha2.GatewayObjectReference{LLMGatewayRef("gw-1", "ns-a")},
			expected: []v1alpha2.ObservedGateway{observedGateway("gw-1", "ns-a", nil, nil)},
		},
		{
			name: "two listeners on one gateway merge into one entry",
			refs: []v1alpha2.GatewayObjectReference{
				LLMGatewayRefWithSection("gw-1", "ns-a", "https"),
				LLMGatewayRefWithSection("gw-1", "ns-a", "http"),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a", []gwapiv1.SectionName{"http", "https"}, nil),
			},
		},
		{
			name: "repeated ref is reported once",
			refs: []v1alpha2.GatewayObjectReference{
				LLMGatewayRefWithSection("gw-1", "ns-a", "http"),
				LLMGatewayRefWithSection("gw-1", "ns-a", "http"),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a", []gwapiv1.SectionName{"http"}, nil),
			},
		},
		{
			name: "unpinned ref subsumes a listener pin on the same gateway",
			refs: []v1alpha2.GatewayObjectReference{
				LLMGatewayRefWithSection("gw-1", "ns-a", "https"),
				LLMGatewayRef("gw-1", "ns-a"),
			},
			expected: []v1alpha2.ObservedGateway{observedGateway("gw-1", "ns-a", nil, nil)},
		},
		{
			name: "distinct gateways stay separate, sorted by namespace/name",
			refs: []v1alpha2.GatewayObjectReference{
				LLMGatewayRef("gw-2", "ns-a"),
				LLMGatewayRef("gw-1", "ns-b"),
				LLMGatewayRef("gw-1", "ns-a"),
			},
			expected: []v1alpha2.ObservedGateway{
				observedGateway("gw-1", "ns-a", nil, nil),
				observedGateway("gw-2", "ns-a", nil, nil),
				observedGateway("gw-1", "ns-b", nil, nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			reconciler, llmSvc := specRefRoutingFixture(g, tt.refs...)

			resolved, err := llmisvc.UpdateRoutingStatusForTest(t.Context(), reconciler, llmSvc)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(llmSvc.Status.Router).ToNot(BeNil())
			g.Expect(llmSvc.Status.Router.Gateways).To(Equal(tt.expected))

			// The returned gateways feed GatewaysReady and InferencePool readiness;
			// each Gateway must appear once, as the route path guarantees.
			g.Expect(resolved).To(HaveLen(len(tt.expected)))
		})
	}
}

// Regression: two refs to one unprogrammed Gateway named it twice in GatewaysReady,
// a message RouterReady and Ready then repeat.
func TestSpecRefGatewaysNotReadyNamesEachGatewayOnce(t *testing.T) {
	g := NewGomegaWithT(t)
	reconciler, llmSvc := specRefRoutingFixture(g,
		LLMGatewayRefWithSection("gw-1", "ns-a", "http"),
		LLMGatewayRefWithSection("gw-1", "ns-a", "https"),
	)

	resolved, err := llmisvc.UpdateRoutingStatusForTest(t.Context(), reconciler, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	reconciler.EvaluateGatewayConditions(t.Context(), llmSvc, resolved)
	cond := llmSvc.Status.GetCondition(v1alpha2.GatewaysReady)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.IsFalse()).To(BeTrue())
	g.Expect(cond.Message).To(Equal("The following Gateways are not ready: [ns-a/gw-1]"))
}
