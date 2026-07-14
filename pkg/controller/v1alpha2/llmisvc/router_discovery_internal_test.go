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
