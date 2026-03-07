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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
)

func TestEPPServiceName(t *testing.T) {
	llmSvc := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm"},
	}
	defaultName := "my-llm-epp-service"

	tests := []struct {
		name     string
		router   *RouterSpec
		expected string
	}{
		{
			name:     "nil router",
			router:   nil,
			expected: defaultName,
		},
		{
			name:     "nil scheduler",
			router:   &RouterSpec{},
			expected: defaultName,
		},
		{
			name: "nil pool",
			router: &RouterSpec{
				Scheduler: &SchedulerSpec{},
			},
			expected: defaultName,
		},
		{
			name: "inline pool with default EndpointPickerRef name",
			router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Pool: &InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							EndpointPickerRef: igwapi.EndpointPickerRef{
								Name: igwapi.ObjectName(defaultName),
							},
						},
					},
				},
			},
			expected: defaultName,
		},
		{
			// This is the key case: a user overrides EndpointPickerRef.Name
			// to a custom value in inline mode. The function should return
			// the custom name so the EPP service matches what the
			// InferencePool references. Currently returns the default name
			// because !HasRef() short-circuits the condition chain before
			// Spec is ever checked.
			name: "inline pool with custom EndpointPickerRef name - should use custom name",
			router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Pool: &InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							EndpointPickerRef: igwapi.EndpointPickerRef{
								Name: "my-custom-epp",
							},
						},
					},
				},
			},
			expected: "my-custom-epp",
		},
		{
			name: "ref pool (external) - default name for deletion lookup",
			router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Pool: &InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "external-pool"},
					},
				},
			},
			expected: defaultName,
		},
		{
			name: "inline pool with empty EndpointPickerRef name",
			router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Pool: &InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							EndpointPickerRef: igwapi.EndpointPickerRef{
								Name: "",
							},
						},
					},
				},
			},
			expected: defaultName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.router.EPPServiceName(llmSvc)
			if got != tt.expected {
				t.Errorf("EPPServiceName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestInferencePoolName(t *testing.T) {
	llmSvc := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm"},
	}

	tests := []struct {
		name      string
		scheduler *SchedulerSpec
		expected  string
	}{
		{
			name:      "nil scheduler",
			scheduler: nil,
			expected:  "my-llm-inference-pool",
		},
		{
			name:      "nil pool",
			scheduler: &SchedulerSpec{},
			expected:  "my-llm-inference-pool",
		},
		{
			name: "inline pool (no ref)",
			scheduler: &SchedulerSpec{
				Pool: &InferencePoolSpec{
					Spec: &igwapi.InferencePoolSpec{},
				},
			},
			expected: "my-llm-inference-pool",
		},
		{
			name: "ref pool",
			scheduler: &SchedulerSpec{
				Pool: &InferencePoolSpec{
					Ref: &corev1.LocalObjectReference{Name: "external-pool"},
				},
			},
			expected: "external-pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.scheduler.InferencePoolName(llmSvc)
			if got != tt.expected {
				t.Errorf("InferencePoolName() = %q, want %q", got, tt.expected)
			}
		})
	}
}
