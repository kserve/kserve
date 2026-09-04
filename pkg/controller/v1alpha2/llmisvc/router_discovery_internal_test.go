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
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
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

func TestResolveSpecRefGateways(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gwapiv1.Install(scheme); err != nil {
		t.Fatalf("install gwapi scheme: %v", err)
	}

	gateway := func(ns, name, class string) *gwapiv1.Gateway {
		return &gwapiv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       gwapiv1.GatewaySpec{GatewayClassName: gwapiv1.ObjectName(class)},
		}
	}
	svc := func(refs ...v1alpha2.GatewayObjectReference) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "svc-ns"},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Gateway: &v1alpha2.GatewaySpec{Refs: refs}},
			},
		}
	}
	ref := func(name, ns string) v1alpha2.GatewayObjectReference {
		return v1alpha2.GatewayObjectReference{
			UntypedObjectReference: v1alpha2.UntypedObjectReference{Name: gwapiv1.ObjectName(name), Namespace: gwapiv1.Namespace(ns)},
		}
	}

	t.Run("no refs resolves nothing without touching the API", func(t *testing.T) {
		r := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
		got, err := r.resolveSpecRefGateways(context.Background(), svc())
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v; want empty, nil", got, err)
		}
	})

	t.Run("synthesizes a parentRef carrying the ref's identity and sectionName", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(gateway("gw-ns", "byo-gw", "envoy"), &gwapiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "envoy"}}).
			Build()
		r := &LLMISVCReconciler{Client: c}

		pinned := ref("byo-gw", "gw-ns")
		pinned.SectionName = ptr.To(gwapiv1.SectionName("https"))

		got, err := r.resolveSpecRefGateways(context.Background(), svc(pinned))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d gateways, want 1", len(got))
		}
		pr := got[0].ParentRef
		if string(pr.Name) != "byo-gw" || ptr.Deref(pr.Namespace, "") != "gw-ns" {
			t.Errorf("parentRef identity = %s/%s, want gw-ns/byo-gw", ptr.Deref(pr.Namespace, ""), pr.Name)
		}
		if ptr.Deref(pr.SectionName, "") != "https" {
			t.Errorf("sectionName not carried through: %#v", pr.SectionName)
		}
		if got[0].GatewayClass == nil || got[0].GatewayClass.Name != "envoy" {
			t.Errorf("GatewayClass not resolved: %#v", got[0].GatewayClass)
		}
		// The key must line up with how InferencePool parents are matched.
		if keys := resolvedGatewayKeys(got); keys[0] != (types.NamespacedName{Name: "byo-gw", Namespace: "gw-ns"}) {
			t.Errorf("resolved key = %v", keys[0])
		}
	})

	t.Run("ref namespace defaults to the service namespace", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway("svc-ns", "local-gw", "envoy")).Build()
		r := &LLMISVCReconciler{Client: c}

		got, err := r.resolveSpecRefGateways(context.Background(), svc(ref("local-gw", "")))
		if err != nil || len(got) != 1 {
			t.Fatalf("got %v, %v; want one gateway", got, err)
		}
		if ptr.Deref(got[0].ParentRef.Namespace, "") != "svc-ns" {
			t.Errorf("namespace = %q, want svc-ns", ptr.Deref(got[0].ParentRef.Namespace, ""))
		}
	})

	t.Run("missing gateway is skipped, not an error", func(t *testing.T) {
		// validateRouterReferences already reports missing refs as RefsInvalid;
		// failing here would mask that clearer message.
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway("gw-ns", "present", "envoy")).Build()
		r := &LLMISVCReconciler{Client: c}

		got, err := r.resolveSpecRefGateways(context.Background(), svc(ref("present", "gw-ns"), ref("absent", "gw-ns")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Gateway.Name != "present" {
			t.Fatalf("got %v, want only the present gateway", got)
		}
	})

	t.Run("missing GatewayClass is tolerated", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway("gw-ns", "byo-gw", "no-such-class")).Build()
		r := &LLMISVCReconciler{Client: c}

		got, err := r.resolveSpecRefGateways(context.Background(), svc(ref("byo-gw", "gw-ns")))
		if err != nil || len(got) != 1 {
			t.Fatalf("got %v, %v; want one gateway", got, err)
		}
		if got[0].GatewayClass != nil {
			t.Errorf("expected nil GatewayClass for a missing class, got %#v", got[0].GatewayClass)
		}
	})
}
