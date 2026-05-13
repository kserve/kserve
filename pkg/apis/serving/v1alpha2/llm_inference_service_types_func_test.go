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
	duckv1 "knative.dev/pkg/apis/duck/v1"
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
			// InferencePool references. It previously returned the default
			// name because !HasRef() short-circuited the condition chain
			// before Spec was ever checked; this test guards against that regression.
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
			name: "ref pool with Spec also set - ref takes precedence",
			router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Pool: &InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "external-pool"},
						Spec: &igwapi.InferencePoolSpec{
							EndpointPickerRef: igwapi.EndpointPickerRef{
								Name: "should-be-ignored",
							},
						},
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

func TestIsUsingLLMInferenceServiceConfig(t *testing.T) {
	tests := []struct {
		name       string
		llmSvc     *LLMInferenceService
		configName string
		want       bool
	}{
		{
			name: "match via Status.Annotations value (exact match)",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "kserve-config-llm-template",
						},
					},
				},
			},
			configName: "kserve-config-llm-template",
			want:       true,
		},
		{
			name: "match via Spec.BaseRefs name",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "my-custom-config"},
					},
				},
			},
			configName: "my-custom-config",
			want:       true,
		},
		{
			name: "no false positive on annotation key (only values are matched)",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"kserve-config-llm-template": "some-other-value",
						},
					},
				},
			},
			configName: "kserve-config-llm-template",
			want:       false,
		},
		{
			name: "no false positive on substring match in annotations",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "kserve-config-llm-template-extended",
						},
					},
				},
			},
			configName: "kserve-config-llm-template",
			want:       false,
		},
		{
			name: "no false positive on substring match in baseRefs",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "my-custom-config-extended"},
					},
				},
			},
			configName: "my-custom-config",
			want:       false,
		},
		{
			name:       "empty annotations and baseRefs returns false",
			llmSvc:     &LLMInferenceService{},
			configName: "kserve-config-llm-template",
			want:       false,
		},
		{
			name: "nil annotations with empty baseRefs returns false",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{},
				},
			},
			configName: "kserve-config-llm-template",
			want:       false,
		},
		{
			name: "found in annotations but not in baseRefs",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "other-config"},
					},
				},
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "target-config",
						},
					},
				},
			},
			configName: "target-config",
			want:       true,
		},
		{
			name: "found in baseRefs but not in annotations",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "target-config"},
					},
				},
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "other-config",
						},
					},
				},
			},
			configName: "target-config",
			want:       true,
		},
		{
			name: "multiple annotations, match on one value",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template":  "kserve-config-llm-template",
							"serving.kserve.io/config-llm-scheduler": "kserve-config-llm-scheduler",
							"serving.kserve.io/config-llm-router":    "kserve-config-llm-router",
						},
					},
				},
			},
			configName: "kserve-config-llm-scheduler",
			want:       true,
		},
		{
			name: "multiple baseRefs, match on one",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "config-a"},
						{Name: "config-b"},
						{Name: "config-c"},
					},
				},
			},
			configName: "config-b",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.llmSvc.IsUsingLLMInferenceServiceConfig(tt.configName)
			if got != tt.want {
				t.Errorf("IsUsingLLMInferenceServiceConfig(%q) = %v, want %v", tt.configName, got, tt.want)
			}
		})
	}
}
