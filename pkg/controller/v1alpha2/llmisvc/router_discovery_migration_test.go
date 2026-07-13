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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func TestIsInferencePoolV1Supported(t *testing.T) {
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
			name: "route without InferencePool backendRef returns Unknown",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefService("my-service")),
				),
				WithHTTPRouteReadyStatus("gateway-controller/v1"),
			),
			expected: metav1.ConditionUnknown,
		},
		{
			name: "route with v1alpha2 InferencePool backendRef returns Unknown (not v1)",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePoolV1Alpha2("my-pool")),
				),
				WithHTTPRouteReadyStatus("gateway-controller/v1"),
			),
			expected: metav1.ConditionUnknown,
		},
		{
			name: "route with v1 InferencePool and ResolvedRefs=True returns True",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				WithHTTPRouteReadyStatus("gateway-controller/v1"),
			),
			expected: metav1.ConditionTrue,
		},
		{
			name: "route with v1 InferencePool and ResolvedRefs=False InvalidKind returns False",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				WithHTTPRouteMultipleControllerStatus(
					GatewayRef("test-gateway", RefInNamespace("test-ns")),
					GatewayAPIControllerStatusInvalidKind,
				),
			),
			expected: metav1.ConditionFalse,
		},
		{
			name: "route with v1 InferencePool and ResolvedRefs=False BackendNotFound returns True (kind is supported)",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				WithHTTPRouteMultipleControllerStatus(
					GatewayRef("test-gateway", RefInNamespace("test-ns")),
					GatewayAPIControllerStatusBackendNotFound,
				),
			),
			expected: metav1.ConditionTrue,
		},
		{
			name: "route with v1 InferencePool and no status returns Unknown",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				// No status set
			),
			expected: metav1.ConditionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := llmisvc.IsInferencePoolV1Supported(tt.route)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestIsInferencePoolSupportedWithMultipleParents(t *testing.T) {
	tests := []struct {
		name     string
		route    *gwapiv1.HTTPRoute
		expected metav1.ConditionStatus
	}{
		{
			name: "route with both Gateway and Kuadrant controllers - Gateway has ResolvedRefs=True",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				WithHTTPRouteMultipleControllerStatus(
					GatewayRef("test-gateway", RefInNamespace("test-ns")),
					GatewayAPIControllerStatus,
					KuadrantControllerStatus,
				),
			),
			expected: metav1.ConditionTrue,
		},
		{
			name: "route with both Gateway and Kuadrant controllers - Gateway has ResolvedRefs=False InvalidKind",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				WithHTTPRouteMultipleControllerStatus(
					GatewayRef("test-gateway", RefInNamespace("test-ns")),
					GatewayAPIControllerStatusInvalidKind,
					KuadrantControllerStatus,
				),
			),
			expected: metav1.ConditionFalse,
		},
		{
			name: "route with only Kuadrant controller (no ResolvedRefs condition) returns Unknown",
			route: HTTPRoute("test-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRef(GatewayRef("test-gateway", RefInNamespace("test-ns"))),
				WithHTTPRule(
					WithBackendRefs(BackendRefInferencePool("my-pool")),
				),
				WithHTTPRouteMultipleControllerStatus(
					GatewayRef("test-gateway", RefInNamespace("test-ns")),
					KuadrantControllerStatus, // Only Kuadrant, no Gateway controller
				),
			),
			expected: metav1.ConditionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := llmisvc.IsInferencePoolV1Supported(tt.route)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}
