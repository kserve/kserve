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
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

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
