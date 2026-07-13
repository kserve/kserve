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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

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
