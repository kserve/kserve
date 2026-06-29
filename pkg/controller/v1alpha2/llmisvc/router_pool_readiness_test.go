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

	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func gw(name, namespace string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: namespace}
}

func TestIsInferencePoolReady(t *testing.T) {
	tests := []struct {
		name     string
		pool     *igwapi.InferencePool
		expected bool
	}{
		{
			name:     "nil pool is not ready",
			pool:     nil,
			expected: false,
		},
		{
			name: "pool with no parents is not ready",
			pool: &igwapi.InferencePool{
				Spec: igwapi.InferencePoolSpec{
					Selector: igwapi.LabelSelector{
						MatchLabels: map[igwapi.LabelKey]igwapi.LabelValue{"app": "vllm"},
					},
					TargetPorts: []igwapi.Port{{Number: 8080}},
				},
			},
			expected: false,
		},
		{
			name: "pool with Accepted=True parent is ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(igwapi.InferencePoolConditionAccepted),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pool with Accepted=False parent is not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(igwapi.InferencePoolConditionAccepted),
									Status: metav1.ConditionFalse,
									Reason: "NotSupportedByGateway",
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pool with missing Accepted condition is not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   "SomeOtherCondition",
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pool with stale Accepted condition is not ready",
			pool: &igwapi.InferencePool{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 5,
				},
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:               string(igwapi.InferencePoolConditionAccepted),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 3,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pool with multiple parents all accepted is ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pool with one parent not accepted among multiple is not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionFalse},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmisvc.IsInferencePoolReady(tt.pool)
			if got != tt.expected {
				t.Errorf("IsInferencePoolReady() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsInferencePoolV1Alpha2Ready(t *testing.T) {
	tests := []struct {
		name     string
		pool     *igwapiv1alpha2.InferencePool
		expected bool
	}{
		{
			name:     "nil pool is not ready",
			pool:     nil,
			expected: false,
		},
		{
			name: "pool with no parents is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
			},
			expected: false,
		},
		{
			name: "pool with Accepted=True parent is ready",
			pool: &igwapiv1alpha2.InferencePool{
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(igwapiv1alpha2.InferencePoolConditionAccepted),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pool with Accepted=False parent is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(igwapiv1alpha2.InferencePoolConditionAccepted),
									Status: metav1.ConditionFalse,
									Reason: "NotSupportedByGateway",
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pool with missing Accepted condition is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   "SomeOtherCondition",
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pool with stale Accepted condition is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 5,
				},
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:               string(igwapiv1alpha2.InferencePoolConditionAccepted),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 3,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pool with multiple parents all accepted is ready",
			pool: &igwapiv1alpha2.InferencePool{
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pool with one parent not accepted among multiple is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Spec: igwapiv1alpha2.InferencePoolSpec{
					Selector:         map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"},
					TargetPortNumber: 8080,
				},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionFalse},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmisvc.IsInferencePoolV1Alpha2Ready(tt.pool)
			if got != tt.expected {
				t.Errorf("IsInferencePoolV1Alpha2Ready() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsInferencePoolReadyForGateways(t *testing.T) {
	tests := []struct {
		name        string
		pool        *igwapi.InferencePool
		resolvedGWs []types.NamespacedName
		expected    bool
	}{
		{
			name: "stale rejected parent from old gateway does not block readiness",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "old-gateway", Namespace: "gw-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionFalse, Reason: "HTTPRouteNotAccepted"},
							},
						},
						{
							ParentRef: igwapi.ParentReference{Name: "current-gateway", Namespace: "gw-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("current-gateway", "gw-ns")},
			expected:    true,
		},
		{
			name: "all relevant parents rejected means not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "current-gateway", Namespace: "gw-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionFalse, Reason: "NotAllowedByListeners"},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("current-gateway", "gw-ns")},
			expected:    false,
		},
		{
			name: "no matching parents for current gateway means not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "old-gateway", Namespace: "old-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("new-gateway", "ingress-ns")},
			expected:    false,
		},
		{
			name: "nil resolvedGWs means no gateways so not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: nil,
			expected:    false,
		},
		{
			name: "pool parent with omitted namespace matches gateway in pool namespace",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "same-ns-gateway", Namespace: "my-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("same-ns-gateway", "my-ns")},
			expected:    true,
		},
		{
			name: "not ready when only some gateways have matching pool parents",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "gateway-a", Namespace: "gw-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{
				gw("gateway-a", "gw-ns"),
				gw("gateway-b", "gw-ns"),
			},
			expected: false,
		},
		{
			name:        "nil pool is not ready",
			pool:        nil,
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "pool with empty status parents is not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{Parents: []igwapi.ParentStatus{}},
			},
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "relevant parent with missing Accepted condition is not ready",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "gw", Namespace: "ns"},
							Conditions: []metav1.Condition{
								{Type: "ResolvedRefs", Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "relevant parent with stale Accepted condition is not ready",
			pool: &igwapi.InferencePool{
				ObjectMeta: metav1.ObjectMeta{Generation: 5},
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "gw", Namespace: "ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue, ObservedGeneration: 3},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "stale accepted parent from old gateway does not make pool ready when current is rejected",
			pool: &igwapi.InferencePool{
				Status: igwapi.InferencePoolStatus{
					Parents: []igwapi.ParentStatus{
						{
							ParentRef: igwapi.ParentReference{Name: "old-gateway", Namespace: "old-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
						{
							ParentRef: igwapi.ParentReference{Name: "current-gateway", Namespace: "current-ns"},
							Conditions: []metav1.Condition{
								{Type: string(igwapi.InferencePoolConditionAccepted), Status: metav1.ConditionFalse, Reason: "NotAllowedByListeners"},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("current-gateway", "current-ns")},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmisvc.IsInferencePoolReadyForGateways(tt.pool, tt.resolvedGWs)
			if got != tt.expected {
				t.Errorf("IsInferencePoolReadyForGateways() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsInferencePoolV1Alpha2ReadyForGateways(t *testing.T) {
	tests := []struct {
		name        string
		pool        *igwapiv1alpha2.InferencePool
		resolvedGWs []types.NamespacedName
		expected    bool
	}{
		{
			name: "stale rejected parent from old gateway does not block readiness",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "old-gateway", Namespace: ptr.To(igwapiv1alpha2.Namespace("gw-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionFalse, Reason: "HTTPRouteNotAccepted"},
							},
						},
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "current-gateway", Namespace: ptr.To(igwapiv1alpha2.Namespace("gw-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("current-gateway", "gw-ns")},
			expected:    true,
		},
		{
			name: "all relevant parents rejected means not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "current-gateway", Namespace: ptr.To(igwapiv1alpha2.Namespace("gw-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionFalse, Reason: "NotAllowedByListeners"},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("current-gateway", "gw-ns")},
			expected:    false,
		},
		{
			name: "no matching parents for current gateway means not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "old-gateway", Namespace: ptr.To(igwapiv1alpha2.Namespace("old-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("new-gateway", "ingress-ns")},
			expected:    false,
		},
		{
			name: "nil resolvedGWs means no gateways so not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "gw", Namespace: ptr.To(igwapiv1alpha2.Namespace("ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: nil,
			expected:    false,
		},
		{
			name: "pool parent with omitted namespace matches gateway in pool namespace",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "same-ns-gw", Namespace: ptr.To(igwapiv1alpha2.Namespace("my-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("same-ns-gw", "my-ns")},
			expected:    true,
		},
		{
			name: "not ready when only some gateways have matching pool parents",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "gateway-a", Namespace: ptr.To(igwapiv1alpha2.Namespace("gw-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{
				gw("gateway-a", "gw-ns"),
				gw("gateway-b", "gw-ns"),
			},
			expected: false,
		},
		{
			name:        "nil pool is not ready",
			pool:        nil,
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "pool with empty status parents is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{Parents: []igwapiv1alpha2.PoolStatus{}},
			},
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "relevant parent with missing Accepted condition is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "gw", Namespace: ptr.To(igwapiv1alpha2.Namespace("ns")),
							},
							Conditions: []metav1.Condition{
								{Type: "ResolvedRefs", Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "relevant parent with stale Accepted condition is not ready",
			pool: &igwapiv1alpha2.InferencePool{
				ObjectMeta: metav1.ObjectMeta{Generation: 5},
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "gw", Namespace: ptr.To(igwapiv1alpha2.Namespace("ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue, ObservedGeneration: 3},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("gw", "ns")},
			expected:    false,
		},
		{
			name: "stale accepted parent from old gateway does not make pool ready when current is rejected",
			pool: &igwapiv1alpha2.InferencePool{
				Status: igwapiv1alpha2.InferencePoolStatus{
					Parents: []igwapiv1alpha2.PoolStatus{
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "old-gateway", Namespace: ptr.To(igwapiv1alpha2.Namespace("old-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionTrue},
							},
						},
						{
							GatewayRef: igwapiv1alpha2.ParentGatewayReference{
								Name: "current-gateway", Namespace: ptr.To(igwapiv1alpha2.Namespace("current-ns")),
							},
							Conditions: []metav1.Condition{
								{Type: string(igwapiv1alpha2.InferencePoolConditionAccepted), Status: metav1.ConditionFalse, Reason: "NotAllowedByListeners"},
							},
						},
					},
				},
			},
			resolvedGWs: []types.NamespacedName{gw("current-gateway", "current-ns")},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmisvc.IsInferencePoolV1Alpha2ReadyForGateways(tt.pool, tt.resolvedGWs)
			if got != tt.expected {
				t.Errorf("IsInferencePoolV1Alpha2ReadyForGateways() = %v, want %v", got, tt.expected)
			}
		})
	}
}
