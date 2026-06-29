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

package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
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
		{
			name: "match via Status.AppliedConfigRefs name",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					AppliedConfigRefs: []AppliedConfigRef{
						{Name: "kserve-config-llm-template", Namespace: "kserve", Source: AppliedConfigSourcePreset},
						{Name: "my-custom-config", Namespace: "test-ns", Source: AppliedConfigSourceUserRef},
					},
				},
			},
			configName: "my-custom-config",
			want:       true,
		},
		{
			name: "no false positive on substring match in appliedConfigRefs",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					AppliedConfigRefs: []AppliedConfigRef{
						{Name: "kserve-config-llm-template-extended", Namespace: "kserve", Source: AppliedConfigSourcePreset},
					},
				},
			},
			configName: "kserve-config-llm-template",
			want:       false,
		},
		{
			name: "appliedConfigRefs short-circuits annotation and baseRef checks",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "unrelated-config"},
					},
				},
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "also-unrelated",
						},
					},
					AppliedConfigRefs: []AppliedConfigRef{
						{Name: "target-config", Namespace: "kserve", Source: AppliedConfigSourcePreset},
					},
				},
			},
			configName: "target-config",
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

func TestIsUsingLLMInferenceServiceConfigInNamespace(t *testing.T) {
	tests := []struct {
		name            string
		llmSvc          *LLMInferenceService
		configName      string
		configNamespace string
		want            bool
	}{
		{
			name: "matches applied config in same namespace",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					AppliedConfigRefs: []AppliedConfigRef{
						{Name: "shared-config", Namespace: "tenant-a", Source: AppliedConfigSourceUserRef},
					},
				},
			},
			configName:      "shared-config",
			configNamespace: "tenant-a",
			want:            true,
		},
		{
			name: "does not match applied config in different namespace",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					AppliedConfigRefs: []AppliedConfigRef{
						{Name: "shared-config", Namespace: "tenant-a", Source: AppliedConfigSourceUserRef},
					},
				},
			},
			configName:      "shared-config",
			configNamespace: "kserve",
			want:            false,
		},
		{
			name: "when applied configs exist, annotation fallback is not used",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "kserve-config-llm-template",
						},
					},
					AppliedConfigRefs: []AppliedConfigRef{
						{Name: "different-config", Namespace: "kserve", Source: AppliedConfigSourcePreset},
					},
				},
			},
			configName:      "kserve-config-llm-template",
			configNamespace: "kserve",
			want:            false,
		},
		{
			name: "cold start falls back to annotations",
			llmSvc: &LLMInferenceService{
				Status: LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{
							"serving.kserve.io/config-llm-template": "kserve-config-llm-template",
						},
					},
				},
			},
			configName:      "kserve-config-llm-template",
			configNamespace: "kserve",
			want:            true,
		},
		{
			name: "cold start falls back to base refs",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{
						{Name: "tenant-config"},
					},
				},
			},
			configName:      "tenant-config",
			configNamespace: "tenant-a",
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.llmSvc.IsUsingLLMInferenceServiceConfigInNamespace(tt.configName, tt.configNamespace)
			if got != tt.want {
				t.Errorf("IsUsingLLMInferenceServiceConfigInNamespace(%q, %q) = %v, want %v", tt.configName, tt.configNamespace, got, tt.want)
			}
		})
	}
}

func newTestLLMISVC() *LLMInferenceService {
	svc := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
		Status: LLMInferenceServiceStatus{
			Status: duckv1.Status{},
		},
	}
	svc.GetConditionSet().Manage(svc.GetStatus()).InitializeConditions()
	return svc
}

func getConditionStatus(svc *LLMInferenceService, ct apis.ConditionType) string {
	cond := svc.GetStatus().GetCondition(ct)
	if cond == nil {
		return "nil"
	}
	return string(cond.Status)
}

func TestDetermineWorkloadReadiness_ScalingConditions(t *testing.T) {
	t.Run("ScalingReady=False blocks WorkloadsReady", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkMainWorkloadReady()
		svc.MarkScalingNotReady("FailedGetExternalMetric", "metric not found")

		svc.DetermineWorkloadReadiness()

		assert.Equal(t, "False", getConditionStatus(svc, WorkloadReady))
		cond := svc.GetStatus().GetCondition(WorkloadReady)
		assert.Equal(t, "FailedGetExternalMetric", cond.Reason)
	})

	t.Run("PrefillScalingReady=False blocks WorkloadsReady", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkMainWorkloadReady()
		svc.MarkScalingReady()
		svc.MarkPrefillScalingNotReady("TriggerError", "prometheus query failed")

		svc.DetermineWorkloadReadiness()

		assert.Equal(t, "False", getConditionStatus(svc, WorkloadReady))
		cond := svc.GetStatus().GetCondition(WorkloadReady)
		assert.Equal(t, "TriggerError", cond.Reason)
	})

	t.Run("all conditions ready including scaling -> WorkloadsReady=True", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkMainWorkloadReady()
		svc.MarkScalingReady()

		svc.DetermineWorkloadReadiness()

		assert.Equal(t, "True", getConditionStatus(svc, WorkloadReady))
	})

	t.Run("absent ScalingReady does not block WorkloadsReady", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkMainWorkloadReady()

		svc.DetermineWorkloadReadiness()

		assert.Equal(t, "True", getConditionStatus(svc, WorkloadReady))
	})

	t.Run("absent PrefillScalingReady does not block WorkloadsReady", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkMainWorkloadReady()
		svc.MarkScalingReady()

		svc.DetermineWorkloadReadiness()

		assert.Equal(t, "True", getConditionStatus(svc, WorkloadReady))
	})

	t.Run("MarkScalingUnset clears the condition", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkScalingNotReady("test", "msg")
		assert.Equal(t, "False", getConditionStatus(svc, ScalingReady))

		svc.MarkScalingUnset()
		assert.Equal(t, "nil", getConditionStatus(svc, ScalingReady))
	})

	t.Run("MarkPrefillScalingUnset clears the condition", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkPrefillScalingNotReady("test", "msg")
		assert.Equal(t, "False", getConditionStatus(svc, PrefillScalingReady))

		svc.MarkPrefillScalingUnset()
		assert.Equal(t, "nil", getConditionStatus(svc, PrefillScalingReady))
	})

	t.Run("MainWorkloadReady=False takes priority over ScalingReady=False", func(t *testing.T) {
		svc := newTestLLMISVC()
		svc.MarkMainWorkloadNotReady("DeploymentUnavailable", "pods crashing")
		svc.MarkScalingNotReady("FailedGetExternalMetric", "metric not found")

		svc.DetermineWorkloadReadiness()

		assert.Equal(t, "False", getConditionStatus(svc, WorkloadReady))
		cond := svc.GetStatus().GetCondition(WorkloadReady)
		assert.Equal(t, "DeploymentUnavailable", cond.Reason, "MainWorkloadReady comes first in order")
	})
}

func TestHasManagedDRA(t *testing.T) {
	tests := []struct {
		name        string
		llmSvc      *LLMInferenceService
		annotations map[string]string
		want        bool
	}{
		{name: "nil receiver", llmSvc: nil, want: false},
		{name: "no annotations", annotations: nil, want: false},
		{name: "unrelated annotation", annotations: map[string]string{"foo": "bar"}, want: false},
		{
			name:        "device class set",
			annotations: map[string]string{"serving.kserve.io/exp-dra-device-class": "gpu.nvidia.com"},
			want:        true,
		},
		{
			name: "only ancillary annotations set is not enough",
			annotations: map[string]string{
				"serving.kserve.io/exp-dra-device-count":   "2",
				"serving.kserve.io/exp-dra-container-name": "main",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.llmSvc
			if svc == nil && tt.name != "nil receiver" {
				svc = &LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			}
			if got := svc.HasManagedDRA(); got != tt.want {
				t.Errorf("HasManagedDRA() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManagedDRADeviceClass(t *testing.T) {
	tests := []struct {
		name        string
		nilReceiver bool
		annotations map[string]string
		wantValue   string
		wantPresent bool
	}{
		{name: "nil receiver", nilReceiver: true, wantValue: "", wantPresent: false},
		{name: "annotation not set", wantValue: "", wantPresent: false},
		{
			name:        "set with normal value",
			annotations: map[string]string{"serving.kserve.io/exp-dra-device-class": "gpu.nvidia.com"},
			wantValue:   "gpu.nvidia.com",
			wantPresent: true,
		},
		{
			name:        "set with surrounding whitespace is trimmed",
			annotations: map[string]string{"serving.kserve.io/exp-dra-device-class": "  gpu.nvidia.com  "},
			wantValue:   "gpu.nvidia.com",
			wantPresent: true,
		},
		{
			name:        "set to whitespace only is present but empty",
			annotations: map[string]string{"serving.kserve.io/exp-dra-device-class": "   "},
			wantValue:   "",
			wantPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc *LLMInferenceService
			if !tt.nilReceiver {
				svc = &LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			}
			gotValue, gotPresent := svc.ManagedDRADeviceClass()
			if gotValue != tt.wantValue || gotPresent != tt.wantPresent {
				t.Errorf("ManagedDRADeviceClass() = (%q, %v), want (%q, %v)",
					gotValue, gotPresent, tt.wantValue, tt.wantPresent)
			}
		})
	}
}

func TestManagedDRADeviceCount(t *testing.T) {
	tests := []struct {
		name        string
		nilReceiver bool
		value       string
		wantCount   int
		wantErr     bool
	}{
		{name: "nil receiver returns default", nilReceiver: true, wantCount: 1},
		{name: "annotation not set returns default", value: "", wantCount: 1},
		{name: "annotation set to whitespace returns default", value: "   ", wantCount: 1},
		{name: "valid value 1", value: "1", wantCount: 1},
		{name: "valid value 2", value: "2", wantCount: 2},
		{name: "valid value 8 with surrounding whitespace", value: " 8 ", wantCount: 8},
		{name: "invalid: zero", value: "0", wantErr: true},
		{name: "invalid: negative", value: "-1", wantErr: true},
		{name: "invalid: non-numeric", value: "abc", wantErr: true},
		{name: "invalid: float", value: "1.5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc *LLMInferenceService
			if !tt.nilReceiver {
				ann := map[string]string{}
				if tt.value != "" {
					ann["serving.kserve.io/exp-dra-device-count"] = tt.value
				}
				svc = &LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Annotations: ann}}
			}
			gotCount, gotErr := svc.ManagedDRADeviceCount()
			if (gotErr != nil) != tt.wantErr {
				t.Fatalf("ManagedDRADeviceCount() err = %v, wantErr = %v", gotErr, tt.wantErr)
			}
			if !tt.wantErr && gotCount != tt.wantCount {
				t.Errorf("ManagedDRADeviceCount() count = %d, want %d", gotCount, tt.wantCount)
			}
		})
	}
}

func TestManagedDRACelSelectors(t *testing.T) {
	tests := []struct {
		name        string
		nilReceiver bool
		value       string
		set         bool
		want        []string
	}{
		{name: "nil receiver", nilReceiver: true, want: nil},
		{name: "annotation not set", want: nil},
		{name: "annotation set to empty", set: true, value: "", want: nil},
		{name: "annotation set to whitespace only", set: true, value: "   \n  \n", want: nil},
		{
			name:  "single expression",
			set:   true,
			value: "device.attributes['gpu.nvidia.com']['type'] == 'A100'",
			want:  []string{"device.attributes['gpu.nvidia.com']['type'] == 'A100'"},
		},
		{
			name: "multiple expressions newline-separated",
			set:  true,
			value: "device.attributes['gpu.nvidia.com']['type'] == 'A100'\n" +
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			want: []string{
				"device.attributes['gpu.nvidia.com']['type'] == 'A100'",
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			},
		},
		{
			name: "blank lines and indentation are stripped",
			set:  true,
			value: "\n  device.attributes['gpu.nvidia.com']['type'] == 'A100'  \n" +
				"\n  device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0\n",
			want: []string{
				"device.attributes['gpu.nvidia.com']['type'] == 'A100'",
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc *LLMInferenceService
			if !tt.nilReceiver {
				ann := map[string]string{}
				if tt.set {
					ann["serving.kserve.io/exp-dra-cel-selector"] = tt.value
				}
				svc = &LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Annotations: ann}}
			}
			got := svc.ManagedDRACelSelectors()
			if len(got) != len(tt.want) {
				t.Fatalf("ManagedDRACelSelectors() = %v (len=%d), want %v (len=%d)",
					got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ManagedDRACelSelectors()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestManagedDRAContainerName(t *testing.T) {
	tests := []struct {
		name        string
		nilReceiver bool
		annotations map[string]string
		wantValue   string
		wantPresent bool
	}{
		{name: "nil receiver", nilReceiver: true, wantValue: "", wantPresent: false},
		{name: "annotation not set", wantValue: "", wantPresent: false},
		{
			name:        "set with normal value",
			annotations: map[string]string{"serving.kserve.io/exp-dra-container-name": "main"},
			wantValue:   "main",
			wantPresent: true,
		},
		{
			name:        "set with surrounding whitespace is trimmed",
			annotations: map[string]string{"serving.kserve.io/exp-dra-container-name": "  worker  "},
			wantValue:   "worker",
			wantPresent: true,
		},
		{
			name:        "set to whitespace only is present but empty",
			annotations: map[string]string{"serving.kserve.io/exp-dra-container-name": "   "},
			wantValue:   "",
			wantPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc *LLMInferenceService
			if !tt.nilReceiver {
				svc = &LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			}
			gotValue, gotPresent := svc.ManagedDRAContainerName()
			if gotValue != tt.wantValue || gotPresent != tt.wantPresent {
				t.Errorf("ManagedDRAContainerName() = (%q, %v), want (%q, %v)",
					gotValue, gotPresent, tt.wantValue, tt.wantPresent)
			}
		})
	}
}
