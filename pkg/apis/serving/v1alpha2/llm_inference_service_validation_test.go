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

package v1alpha2

import (
	"testing"

	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParentRefsMatchGatewayRefs(t *testing.T) {
	tests := []struct {
		name       string
		parentRefs []gwapiv1.ParentReference
		gwRefs     []UntypedObjectReference
		want       bool
	}{
		{
			name:       "both empty",
			parentRefs: nil,
			gwRefs:     nil,
			want:       true,
		},
		{
			name: "single ref, matching",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
			},
			want: true,
		},
		{
			name: "single ref, different name",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-other", Namespace: "ns-a"},
			},
			want: false,
		},
		{
			name: "single ref, different namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-b"},
			},
			want: false,
		},
		{
			name: "parentRef with nil namespace matches empty namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1"},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: ""},
			},
			want: true,
		},
		{
			name: "parentRef with nil namespace does not match non-empty namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1"},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
			},
			want: false,
		},
		{
			name: "different lengths",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
			},
			want: false,
		},
		{
			name: "multiple refs, same order",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
				{Name: "gw-2", Namespace: "ns-b"},
			},
			want: true,
		},
		{
			name: "multiple refs, different order",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
				{Name: "gw-2", Namespace: "ns-b"},
			},
			want: true,
		},
		{
			name: "multiple refs, one mismatch",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-3", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
				{Name: "gw-2", Namespace: "ns-b"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parentRefsMatchGatewayRefs(tt.parentRefs, tt.gwRefs)
			if got != tt.want {
				t.Errorf("parentRefsMatchGatewayRefs() = %v, want %v", got, tt.want)
			}
		})
	}
}
