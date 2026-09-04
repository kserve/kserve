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

package llmisvc

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestResolvedGatewayKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    []ResolvedGateway
		expected []types.NamespacedName
	}{
		{
			name:     "empty input",
			input:    nil,
			expected: nil,
		},
		{
			name: "nil parentRef namespace inferred from gateway object",
			input: []ResolvedGateway{{
				Gateway:   &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "gw-ns"}},
				ParentRef: gwapiv1.ParentReference{Name: "my-gw"},
			}},
			expected: []types.NamespacedName{{Name: "my-gw", Namespace: "gw-ns"}},
		},
		{
			name: "explicit namespace preserved",
			input: []ResolvedGateway{{
				Gateway:   &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "gw-ns"}},
				ParentRef: gwapiv1.ParentReference{Name: "my-gw", Namespace: ptr.To(gwapiv1.Namespace("explicit-ns"))},
			}},
			expected: []types.NamespacedName{{Name: "my-gw", Namespace: "explicit-ns"}},
		},
		{
			name: "mixed nil and explicit namespaces",
			input: []ResolvedGateway{
				{
					Gateway:   &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a"}},
					ParentRef: gwapiv1.ParentReference{Name: "gw-a"},
				},
				{
					Gateway:   &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b"}},
					ParentRef: gwapiv1.ParentReference{Name: "gw-b", Namespace: ptr.To(gwapiv1.Namespace("custom-ns"))},
				},
			},
			expected: []types.NamespacedName{
				{Name: "gw-a", Namespace: "ns-a"},
				{Name: "gw-b", Namespace: "custom-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvedGatewayKeys(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("got %d keys, want %d", len(got), len(tt.expected))
			}
			for i, key := range got {
				if key != tt.expected[i] {
					t.Errorf("key[%d] = %v, want %v", i, key, tt.expected[i])
				}
			}
		})
	}
}

func TestMergeResolvedGateways(t *testing.T) {
	gw := func(ns, name string) ResolvedGateway {
		return ResolvedGateway{
			Gateway:   &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
			ParentRef: gwapiv1.ParentReference{Name: gwapiv1.ObjectName(name), Namespace: ptr.To(gwapiv1.Namespace(ns))},
		}
	}

	tests := []struct {
		name       string
		primary    []ResolvedGateway
		additional []ResolvedGateway
		expected   []types.NamespacedName
	}{
		{
			name:       "spec refs only, as in a route-less BYO deployment",
			primary:    nil,
			additional: []ResolvedGateway{gw("gw-ns", "byo-gw")},
			expected:   []types.NamespacedName{{Name: "byo-gw", Namespace: "gw-ns"}},
		},
		{
			name:       "route-derived and spec-derived are unioned",
			primary:    []ResolvedGateway{gw("route-ns", "route-gw")},
			additional: []ResolvedGateway{gw("gw-ns", "byo-gw")},
			expected: []types.NamespacedName{
				{Name: "route-gw", Namespace: "route-ns"},
				{Name: "byo-gw", Namespace: "gw-ns"},
			},
		},
		{
			name:       "overlapping gateway is not duplicated",
			primary:    []ResolvedGateway{gw("gw-ns", "shared-gw")},
			additional: []ResolvedGateway{gw("gw-ns", "shared-gw")},
			expected:   []types.NamespacedName{{Name: "shared-gw", Namespace: "gw-ns"}},
		},
		{
			name:       "same name in a different namespace is a distinct gateway",
			primary:    []ResolvedGateway{gw("ns-a", "gw")},
			additional: []ResolvedGateway{gw("ns-b", "gw")},
			expected: []types.NamespacedName{
				{Name: "gw", Namespace: "ns-a"},
				{Name: "gw", Namespace: "ns-b"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvedGatewayKeys(mergeResolvedGateways(tt.primary, tt.additional))
			if len(got) != len(tt.expected) {
				t.Fatalf("got %d keys (%v), want %d (%v)", len(got), got, len(tt.expected), tt.expected)
			}
			for i, key := range got {
				if key != tt.expected[i] {
					t.Errorf("key[%d] = %v, want %v", i, key, tt.expected[i])
				}
			}
		})
	}
}

func TestMergeResolvedGatewaysKeepsRouteParentRef(t *testing.T) {
	// The route-derived entry carries the parentRef that actually bound the gateway,
	// including its sectionName, so it must win over a synthetic spec-ref entry.
	routeDerived := ResolvedGateway{
		Gateway: &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"}},
		ParentRef: gwapiv1.ParentReference{
			Name:        "gw",
			Namespace:   ptr.To(gwapiv1.Namespace("ns")),
			SectionName: ptr.To(gwapiv1.SectionName("https")),
		},
	}
	specDerived := ResolvedGateway{
		Gateway:   &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"}},
		ParentRef: gwapiv1.ParentReference{Name: "gw", Namespace: ptr.To(gwapiv1.Namespace("ns"))},
	}

	merged := mergeResolvedGateways([]ResolvedGateway{routeDerived}, []ResolvedGateway{specDerived})
	if len(merged) != 1 {
		t.Fatalf("got %d gateways, want 1", len(merged))
	}
	if merged[0].ParentRef.SectionName == nil || *merged[0].ParentRef.SectionName != "https" {
		t.Errorf("route-derived parentRef was not preserved: %#v", merged[0].ParentRef)
	}
}
