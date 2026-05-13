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
	"k8s.io/utils/ptr"
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
