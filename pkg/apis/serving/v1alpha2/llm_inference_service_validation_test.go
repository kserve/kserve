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
	"strconv"
	"strings"
	"testing"

	"github.com/kserve/kserve/pkg/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParentRefsMatchGatewayRefs(t *testing.T) {
	tests := []struct {
		name       string
		parentRefs []gwapiv1.ParentReference
		gwRefs     []GatewayObjectReference
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
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
			},
			want: true,
		},
		{
			name: "single ref, different name",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-other", Namespace: "ns-a"}},
			},
			want: false,
		},
		{
			name: "single ref, different namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-b"}},
			},
			want: false,
		},
		{
			name: "parentRef with nil namespace matches empty namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1"},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: ""}},
			},
			want: true,
		},
		{
			name: "parentRef with nil namespace does not match non-empty namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1"},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
			},
			want: false,
		},
		{
			name: "different lengths",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
			},
			want: false,
		},
		{
			name: "multiple refs, same order",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-2", Namespace: "ns-b"}},
			},
			want: true,
		},
		{
			name: "multiple refs, different order",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-2", Namespace: "ns-b"}},
			},
			want: true,
		},
		{
			name: "multiple refs, one mismatch",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-3", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-2", Namespace: "ns-b"}},
			},
			want: false,
		},
		{
			name: "matching sectionName",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a")), SectionName: ptr.To(gwapiv1.SectionName("https"))},
			},
			gwRefs: []GatewayObjectReference{
				{
					UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"},
					SectionName:            ptr.To(gwapiv1.SectionName("https")),
				},
			},
			want: true,
		},
		{
			name: "different sectionName - same gateway otherwise",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a")), SectionName: ptr.To(gwapiv1.SectionName("https"))},
			},
			gwRefs: []GatewayObjectReference{
				{
					UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"},
					SectionName:            ptr.To(gwapiv1.SectionName("http")),
				},
			},
			want: false,
		},
		{
			name: "parentRef has sectionName, gwRef does not",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a")), SectionName: ptr.To(gwapiv1.SectionName("https"))},
			},
			gwRefs: []GatewayObjectReference{
				{UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"}},
			},
			want: false,
		},
		{
			name: "gwRef has sectionName, parentRef does not",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []GatewayObjectReference{
				{
					UntypedObjectReference: UntypedObjectReference{Name: "gw-1", Namespace: "ns-a"},
					SectionName:            ptr.To(gwapiv1.SectionName("https")),
				},
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

func newBaseLLMInferenceServiceV1Alpha2() *LLMInferenceService {
	return &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI: apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			},
		},
	}
}

func TestValidateUpdate_DeletionBypass(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	oldSvc := newBaseLLMInferenceServiceV1Alpha2()
	newSvc := newBaseLLMInferenceServiceV1Alpha2()
	newSvc.Spec.WorkloadSpec = WorkloadSpec{
		Replicas: ptr.To(int32(3)),
		Scaling:  &ScalingSpec{MaxReplicas: 5},
	}

	// Without DeletionTimestamp, this should be rejected (replicas + scaling are mutually exclusive)
	warnings, err := validator.ValidateUpdate(t.Context(), oldSvc, newSvc)
	assert.Empty(t, warnings)
	assert.Error(t, err)

	// With DeletionTimestamp set, the same object should be accepted
	deletingSvc := newSvc.DeepCopy()
	now := metav1.Now()
	deletingSvc.DeletionTimestamp = &now
	warnings, err = validator.ValidateUpdate(t.Context(), oldSvc, deletingSvc)
	assert.Empty(t, warnings)
	assert.NoError(t, err)
}

func TestValidateWVAConfiguration(t *testing.T) {
	makeWVAWorkload := func() WorkloadSpec {
		return WorkloadSpec{Scaling: &ScalingSpec{
			MinReplicas: ptr.To(int32(1)),
			MaxReplicas: 2,
			WVA: &WVASpec{
				VariantCost: "10.0",
				ActuatorSpec: ActuatorSpec{KEDA: &KEDAScalingSpec{
					PollingInterval:  ptr.To(int32(30)),
					IdleReplicaCount: ptr.To(int32(0)),
				}},
			},
		}}
	}

	type testCase struct {
		name    string
		old     *LLMInferenceService
		current *LLMInferenceService
		wantErr bool
	}
	tests := make([]testCase, 0, 8)
	tests = append(tests, []testCase{
		{
			name: "create with WVA is rejected",
			current: func() *LLMInferenceService {
				svc := newBaseLLMInferenceServiceV1Alpha2()
				svc.Spec.WorkloadSpec = makeWVAWorkload()
				return svc
			}(),
			wantErr: true,
		},
		{
			name: "update adding WVA is rejected",
			old:  newBaseLLMInferenceServiceV1Alpha2(),
			current: func() *LLMInferenceService {
				svc := newBaseLLMInferenceServiceV1Alpha2()
				svc.Spec.WorkloadSpec = makeWVAWorkload()
				return svc
			}(),
			wantErr: true,
		},
		{
			name: "update retaining existing WVA is allowed",
			old: func() *LLMInferenceService {
				svc := newBaseLLMInferenceServiceV1Alpha2()
				svc.Spec.WorkloadSpec = makeWVAWorkload()
				return svc
			}(),
			current: func() *LLMInferenceService {
				svc := newBaseLLMInferenceServiceV1Alpha2()
				svc.Spec.WorkloadSpec = makeWVAWorkload()
				svc.Labels = map[string]string{"updated": "true"}
				svc.Spec.Scaling.MaxReplicas = 3
				return svc
			}(),
		},
		{
			name: "update removing existing WVA is allowed",
			old: func() *LLMInferenceService {
				svc := newBaseLLMInferenceServiceV1Alpha2()
				svc.Spec.WorkloadSpec = makeWVAWorkload()
				return svc
			}(),
			current: newBaseLLMInferenceServiceV1Alpha2(),
		},
	}...)

	for _, mutation := range []struct {
		name   string
		modify func(*WVASpec)
	}{
		{"variant cost", func(wva *WVASpec) { wva.VariantCost = "20.0" }},
		{"polling interval", func(wva *WVASpec) { wva.KEDA.PollingInterval = ptr.To(int32(60)) }},
		{"idle replica count", func(wva *WVASpec) { wva.KEDA.IdleReplicaCount = ptr.To(int32(1)) }},
		{"actuator", func(wva *WVASpec) {
			wva.KEDA = nil
			wva.HPA = &HPAScalingSpec{}
		}},
	} {
		old := newBaseLLMInferenceServiceV1Alpha2()
		old.Spec.WorkloadSpec = makeWVAWorkload()
		current := old.DeepCopy()
		mutation.modify(current.Spec.Scaling.WVA)
		tests = append(tests, testCase{
			name:    "update changing " + mutation.name + " is rejected",
			old:     old,
			current: current,
			wantErr: true,
		})
	}

	for _, workload := range []string{"main", "prefill"} {
		t.Run(workload, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					old, current := tt.old.DeepCopy(), tt.current.DeepCopy()
					path := "spec.scaling.wva"
					if workload == "prefill" {
						path = "spec.prefill.scaling.wva"
						if old != nil {
							old.Spec.Prefill = &WorkloadSpec{Scaling: old.Spec.Scaling}
							old.Spec.Scaling = nil
						}
						current.Spec.Prefill = &WorkloadSpec{Scaling: current.Spec.Scaling}
						current.Spec.Scaling = nil
					}
					err := (&LLMInferenceServiceValidator{}).validateWVAConfig(old, current)
					if tt.wantErr {
						require.Len(t, err, 1)
						assert.Equal(t, path, err[0].Field)
						assert.Equal(t, field.ErrorTypeForbidden, err[0].Type)
						assert.ErrorContains(t, err[0], "WVA autoscaling is no longer supported")
						return
					}
					assert.Empty(t, err)
				})
			}
		})
	}
}

func TestValidateLoRAAdapters(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	makeAdapter := func(name, uri string) LLMModelSpec {
		return LLMModelSpec{URI: apis.URL{Scheme: "hf", Host: uri}, Name: ptr.To(name)}
	}

	makeSvc := func(modelName string, loraSpec *LoRASpec) *LLMInferenceService {
		return &LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: "default"},
			Spec: LLMInferenceServiceSpec{
				Model: LLMModelSpec{
					URI:  apis.URL{Scheme: "hf", Host: "base-model"},
					Name: ptr.To(modelName),
					LoRA: loraSpec,
				},
			},
		}
	}

	t.Run("no lora", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", nil))
		assert.Empty(t, errs)
	})

	t.Run("valid single adapter", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			Adapters: []LLMModelSpec{makeAdapter("adapter-1", "adapter-1")},
		}))
		assert.Empty(t, errs)
	})

	t.Run("adapter name missing", func(t *testing.T) {
		svc := makeSvc("base", &LoRASpec{
			Adapters: []LLMModelSpec{{URI: apis.URL{Scheme: "hf", Host: "adapter-1"}}},
		})
		errs := validator.validateLoRAAdapters(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.adapters[0].name")
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
	})

	t.Run("adapter name is dot (path traversal)", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			Adapters: []LLMModelSpec{makeAdapter(".", "adapter-dot")},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.adapters[0].name")
		assert.Contains(t, errs[0].Detail, "path traversal")
	})

	t.Run("adapter name is dotdot (path traversal)", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			Adapters: []LLMModelSpec{makeAdapter("..", "adapter-dotdot")},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.adapters[0].name")
		assert.Contains(t, errs[0].Detail, "path traversal")
	})

	t.Run("duplicate adapter names", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			Adapters: []LLMModelSpec{
				makeAdapter("dup", "adapter-1"),
				makeAdapter("dup", "adapter-2"),
			},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.adapters[1].name")
		assert.Contains(t, errs[0].Detail, "duplicate")
	})

	t.Run("adapter name same as base model name", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base-model", &LoRASpec{
			Adapters: []LLMModelSpec{makeAdapter("base-model", "adapter-1")},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.adapters[0].name")
		assert.Contains(t, errs[0].Detail, "adapter name must differ from base model name")
	})

	t.Run("maxRank zero is invalid", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			MaxRank:  ptr.To(int32(0)),
			Adapters: []LLMModelSpec{makeAdapter("adapter-1", "adapter-1")},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.maxRank")
	})

	t.Run("maxAdapters zero is invalid", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			MaxAdapters: ptr.To(int32(0)),
			Adapters:    []LLMModelSpec{makeAdapter("adapter-1", "adapter-1")},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.maxAdapters")
	})

	t.Run("maxCpuAdapters zero is invalid", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			MaxCpuAdapters: ptr.To(int32(0)),
			Adapters:       []LLMModelSpec{makeAdapter("adapter-1", "adapter-1")},
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.model.lora.maxCpuAdapters")
	})

	t.Run("all lora params valid", func(t *testing.T) {
		errs := validator.validateLoRAAdapters(makeSvc("base", &LoRASpec{
			MaxRank:        ptr.To(int32(128)),
			MaxAdapters:    ptr.To(int32(4)),
			MaxCpuAdapters: ptr.To(int32(8)),
			Adapters: []LLMModelSpec{
				makeAdapter("adapter-1", "adapter-1"),
				makeAdapter("adapter-2", "adapter-2"),
			},
		}))
		assert.Empty(t, errs)
	})
}

func TestValidateLoRAModelRoutingStrategyAnnotation(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}
	for _, tt := range []struct {
		name    string
		value   *string
		wantErr bool
	}{
		{name: "absent defers to the ConfigMap", value: nil},
		{name: "empty defers to the ConfigMap", value: ptr.To("")},
		{name: "exact", value: ptr.To("exact")},
		{name: "regex", value: ptr.To("regex")},
		{name: "trimmed and case-insensitive like the consumer", value: ptr.To(" Regex ")},
		{name: "typo is rejected at admission", value: ptr.To("regexp"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceServiceV1Alpha2()
			if tt.value != nil {
				svc.Spec.Annotations = map[string]string{constants.LoRAModelRoutingStrategyAnnotationKey: *tt.value}
			}

			errs := validator.validateLoRAModelRoutingStrategyAnnotation(svc)

			if !tt.wantErr {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			assert.Equal(t, field.ErrorTypeNotSupported, errs[0].Type)
			assert.Contains(t, errs[0].Field, constants.LoRAModelRoutingStrategyAnnotationKey)
		})
	}
}

func TestValidateManagedDRAAnnotations(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	const (
		deviceClassKey   = "serving.kserve.io/exp-dra-device-class"
		deviceCountKey   = "serving.kserve.io/exp-dra-device-count"
		celSelectorKey   = "serving.kserve.io/exp-dra-cel-selector"
		containerNameKey = "serving.kserve.io/exp-dra-container-name"
	)

	tests := []struct {
		name         string
		annotations  map[string]string
		wantErrCount int
		wantErrField string
	}{
		{
			name:         "no DRA annotations",
			annotations:  nil,
			wantErrCount: 0,
		},
		{
			name: "valid: device class only",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
			},
			wantErrCount: 0,
		},
		{
			name: "valid: device class + device count + cel selectors",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				deviceCountKey: "4",
				celSelectorKey: "device.attributes['gpu.nvidia.com']['type'] == 'A100'\n" +
					"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			},
			wantErrCount: 0,
		},
		{
			name: "valid: device class with dotted name",
			annotations: map[string]string{
				deviceClassKey: "mig-3g.40gb",
			},
			wantErrCount: 0,
		},
		{
			name: "invalid: empty device class",
			annotations: map[string]string{
				deviceClassKey: "   ",
			},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "invalid: device class with uppercase",
			annotations: map[string]string{
				deviceClassKey: "GPU.Nvidia.com",
			},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "invalid: device count without device class",
			annotations: map[string]string{
				deviceCountKey: "2",
			},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "invalid: cel selector without device class",
			annotations: map[string]string{
				celSelectorKey: "device.attributes['gpu.nvidia.com']['type'] == 'A100'",
			},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "invalid: device count is non-numeric (the foot-gun the webhook is meant to catch)",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				deviceCountKey: "abc",
			},
			wantErrCount: 1,
			wantErrField: deviceCountKey,
		},
		{
			name: "invalid: device count is zero",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				deviceCountKey: "0",
			},
			wantErrCount: 1,
			wantErrField: deviceCountKey,
		},
		{
			name: "invalid: device count is negative",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				deviceCountKey: "-1",
			},
			wantErrCount: 1,
			wantErrField: deviceCountKey,
		},
		{
			name: "invalid: cel selector annotation set but contains no expressions",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				celSelectorKey: "\n  \n",
			},
			wantErrCount: 1,
			wantErrField: celSelectorKey,
		},
		{
			name: "invalid: multiple errors are surfaced together",
			annotations: map[string]string{
				deviceClassKey: "BAD CLASS",
				deviceCountKey: "abc",
			},
			wantErrCount: 2,
		},
		{
			name: "invalid: device class with consecutive dots (rejected by IsDNS1123Subdomain)",
			annotations: map[string]string{
				deviceClassKey: "gpu..nvidia.com",
			},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "valid: device class + explicit container name",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "vllm",
			},
			wantErrCount: 0,
		},
		{
			name: "invalid: container name without device class",
			annotations: map[string]string{
				containerNameKey: "vllm",
			},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "invalid: empty container name",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "   ",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "invalid: container name with uppercase (not a DNS label)",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "VLLM",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "invalid: container name contains dots (DNS label disallows dots)",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "vllm.main",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "valid: hyphenated container name (normal DNS label)",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "kserve-container",
			},
			wantErrCount: 0,
		},
		{
			name: "valid: container name with surrounding whitespace is trimmed before validation",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "  vllm  ",
			},
			wantErrCount: 0,
		},
		{
			name: "invalid: container name with embedded space",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "vllm main",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "invalid: container name with underscore (DNS label disallows underscores)",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "vllm_main",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "invalid: container name with trailing hyphen",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "vllm-",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "invalid: container name longer than 63 characters",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: strings.Repeat("a", 64),
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceServiceV1Alpha2()
			svc.Annotations = tt.annotations

			errs := validator.validateManagedDRAAnnotations(svc)

			require.Len(t, errs, tt.wantErrCount, "errors: %v", errs)
			if tt.wantErrField != "" {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Field, tt.wantErrField) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error on field %q, got: %v", tt.wantErrField, errs)
			}
		})
	}
}

func TestValidateConfidential(t *testing.T) {
	tests := []struct {
		name           string
		confidential   *ConfidentialSpec
		modelURI       apis.URL
		wantErrCount   int
		wantErrStrings []string
		wantWarnings   []string
	}{
		{
			name:         "nil confidential spec",
			confidential: nil,
			modelURI:     apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			wantErrCount: 0,
		},
		{
			name:         "confidential disabled",
			confidential: &ConfidentialSpec{Enabled: false},
			modelURI:     apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			wantErrCount: 0,
		},
		{
			name:         "confidential enabled with valid resourceId",
			confidential: &ConfidentialSpec{Enabled: true, ResourceId: ptr.To("kbs:///default/key/model-key")},
			modelURI:     apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			wantErrCount: 0,
		},
		{
			name:         "confidential enabled without resourceId",
			confidential: &ConfidentialSpec{Enabled: true},
			modelURI:     apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			wantErrCount: 0,
		},
		{
			name:         "confidential enabled with OCI URI warns",
			confidential: &ConfidentialSpec{Enabled: true},
			modelURI:     apis.URL{Scheme: "oci", Host: "registry/model:latest"},
			wantErrCount: 0,
			wantWarnings: []string{"OCI URIs"},
		},
		{
			name:         "confidential enabled with PVC URI warns",
			confidential: &ConfidentialSpec{Enabled: true},
			modelURI:     apis.URL{Scheme: "pvc", Host: "my-pvc/model-dir"},
			wantErrCount: 0,
			wantWarnings: []string{"PVC URIs"},
		},
		{
			name:           "confidential with malformed resourceId",
			confidential:   &ConfidentialSpec{Enabled: true, ResourceId: ptr.To("invalid-id")},
			modelURI:       apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			wantErrCount:   1,
			wantErrStrings: []string{"kbs:///<repo>/<type>/<tag>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &LLMInferenceServiceValidator{}
			llmSvc := &LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-isvc",
					Namespace: "default",
				},
				Spec: LLMInferenceServiceSpec{
					Model: LLMModelSpec{
						URI:          tt.modelURI,
						Confidential: tt.confidential,
					},
				},
			}
			warnings, errs := validator.validateConfidential(llmSvc)

			assert.Len(t, errs, tt.wantErrCount, "expected %d errors, got %d: %v", tt.wantErrCount, len(errs), errs)
			for _, wantStr := range tt.wantErrStrings {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Error(), wantStr) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error containing %q, got: %v", wantStr, errs)
			}
			for _, wantWarning := range tt.wantWarnings {
				found := false
				for _, w := range warnings {
					if strings.Contains(w, wantWarning) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected warning containing %q, got: %v", wantWarning, warnings)
			}
		})
	}
}

func TestValidateKVCacheOffloading(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	makeSvc := func(kv *KVCacheOffloadingSpec) *LLMInferenceService {
		return &LLMInferenceService{
			Spec: LLMInferenceServiceSpec{
				WorkloadSpec: WorkloadSpec{KVCacheOffloading: kv},
			},
		}
	}

	t.Run("nil spec produces no errors", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(nil))
		assert.Empty(t, errs)
	})

	t.Run("cpu-only (no secondary) produces no errors", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
		}))
		assert.Empty(t, errs)
	})

	t.Run("valid emptyDir secondary tier", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{
				{FileSystem: &FileSystemTierSpec{
					EmptyDir: &EmptyDirTierSpec{Size: resource.MustParse("100Gi")},
				}},
			},
		}))
		assert.Empty(t, errs)
	})

	t.Run("secondary without cpu produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			Secondary: []SecondaryTierSpec{
				{FileSystem: &FileSystemTierSpec{
					EmptyDir: &EmptyDirTierSpec{Size: resource.MustParse("100Gi")},
				}},
			},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
		assert.Contains(t, errs[0].Field, "cpu")
	})

	t.Run("nil fileSystem produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU:       resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{{FileSystem: nil}},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
		assert.Contains(t, errs[0].Field, "fileSystem")
	})

	t.Run("both emptyDir and pvc set produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{
				{FileSystem: &FileSystemTierSpec{
					EmptyDir: &EmptyDirTierSpec{Size: resource.MustParse("100Gi")},
					PVC:      &PVCTierSpec{},
				}},
			},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
		assert.Contains(t, errs[0].Field, "fileSystem")
	})

	t.Run("none of emptyDir/pvc set produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU:       resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{{FileSystem: &FileSystemTierSpec{}}},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
	})

	t.Run("pvc with neither spec nor ref produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{
				{FileSystem: &FileSystemTierSpec{
					PVC: &PVCTierSpec{},
				}},
			},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
	})

	t.Run("pvc.ref with empty name produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{
				{FileSystem: &FileSystemTierSpec{
					PVC: &PVCTierSpec{Ref: &PVCRefTierSpec{Name: ""}},
				}},
			},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
		assert.Contains(t, errs[0].Field, "name")
	})

	t.Run("pvc with both spec and ref produces error", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{
				{FileSystem: &FileSystemTierSpec{
					PVC: &PVCTierSpec{
						Spec: &corev1.PersistentVolumeClaimSpec{},
						Ref:  &PVCRefTierSpec{Name: "my-pvc"},
					},
				}},
			},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
	})

	t.Run("pvc.spec rejects ReadOnlyMany", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []SecondaryTierSpec{{FileSystem: &FileSystemTierSpec{
				PVC: &PVCTierSpec{
					Spec: &corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
					},
				},
			}}},
		}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
		assert.Contains(t, errs[0].Field, "accessModes")
	})

	t.Run("prefill kvCacheOffloading is also validated", func(t *testing.T) {
		svc := &LLMInferenceService{
			Spec: LLMInferenceServiceSpec{
				WorkloadSpec: WorkloadSpec{
					KVCacheOffloading: &KVCacheOffloadingSpec{
						CPU: resource.MustParse("10Gi"),
					},
				},
				Prefill: &WorkloadSpec{
					KVCacheOffloading: &KVCacheOffloadingSpec{
						// secondary set but cpu is zero
						Secondary: []SecondaryTierSpec{
							{FileSystem: &FileSystemTierSpec{
								EmptyDir: &EmptyDirTierSpec{Size: resource.MustParse("100Gi")},
							}},
						},
					},
				},
			},
		}
		errs := validator.validateKVCacheOffloading(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "prefill")
		assert.Contains(t, errs[0].Field, "cpu")
	})
}

func TestValidateKVCacheOffloadingNegativeCPU(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	makeSvc := func(kv *KVCacheOffloadingSpec) *LLMInferenceService {
		return &LLMInferenceService{
			Spec: LLMInferenceServiceSpec{
				WorkloadSpec: WorkloadSpec{KVCacheOffloading: kv},
			},
		}
	}
	negative := &KVCacheOffloadingSpec{CPU: resource.MustParse("-1Gi")}

	t.Run("rejected", func(t *testing.T) {
		errs := validator.validateKVCacheOffloading(makeSvc(negative))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
		assert.Contains(t, errs[0].Field, "cpu")
	})

	// Correcting the field is accepted, so a stored negative is not stranded even
	// though the rule is not ratcheted against the previous object.
	t.Run("correcting it is accepted", func(t *testing.T) {
		assert.Empty(t, validator.validateKVCacheOffloading(
			makeSvc(&KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")})))
	})

	t.Run("zero is left alone", func(t *testing.T) {
		assert.Empty(t, validator.validateKVCacheOffloading(makeSvc(&KVCacheOffloadingSpec{})))
	})

	t.Run("prefill is checked too", func(t *testing.T) {
		svc := makeSvc(&KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")})
		svc.Spec.Prefill = &WorkloadSpec{KVCacheOffloading: negative}
		errs := validator.validateKVCacheOffloading(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "prefill")
	})
}

func TestValidateRolloutStrategy(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	makeSvc := func(rs *RolloutStrategy) *LLMInferenceService {
		return &LLMInferenceService{
			Spec: LLMInferenceServiceSpec{
				WorkloadSpec: WorkloadSpec{RolloutStrategy: rs},
			},
		}
	}

	t.Run("nil rollout strategy produces no errors", func(t *testing.T) {
		errs := validator.validateRolloutStrategy(makeSvc(nil))
		assert.Empty(t, errs)
	})

	t.Run("valid integer maxUnavailable", func(t *testing.T) {
		v := intstr.FromInt32(1)
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{MaxUnavailable: &v}))
		assert.Empty(t, errs)
	})

	t.Run("valid percentage maxSurge", func(t *testing.T) {
		v := intstr.FromString("100%")
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{MaxSurge: &v}))
		assert.Empty(t, errs)
	})

	t.Run("valid combination maxUnavailable=0% maxSurge=100%", func(t *testing.T) {
		mu := intstr.FromString("0%")
		ms := intstr.FromString("100%")
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{
			MaxUnavailable: &mu,
			MaxSurge:       &ms,
		}))
		assert.Empty(t, errs)
	})

	t.Run("valid combination maxUnavailable=1 maxSurge=0", func(t *testing.T) {
		mu := intstr.FromInt32(1)
		ms := intstr.FromInt32(0)
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{
			MaxUnavailable: &mu,
			MaxSurge:       &ms,
		}))
		assert.Empty(t, errs)
	})

	t.Run("negative integer rejected", func(t *testing.T) {
		v := intstr.FromInt32(-1)
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{MaxUnavailable: &v}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
		assert.Contains(t, errs[0].Field, "maxUnavailable")
	})

	t.Run("non-percentage string rejected", func(t *testing.T) {
		v := intstr.FromString("abc")
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{MaxSurge: &v}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
		assert.Contains(t, errs[0].Field, "maxSurge")
	})

	t.Run("percentage over 100 rejected", func(t *testing.T) {
		v := intstr.FromString("150%")
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{MaxSurge: &v}))
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
	})

	t.Run("both zero deadlock rejected", func(t *testing.T) {
		mu := intstr.FromInt32(0)
		ms := intstr.FromInt32(0)
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{
			MaxUnavailable: &mu,
			MaxSurge:       &ms,
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, "both be zero")
	})

	t.Run("both zero percentage deadlock rejected", func(t *testing.T) {
		mu := intstr.FromString("0%")
		ms := intstr.FromString("0%")
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{
			MaxUnavailable: &mu,
			MaxSurge:       &ms,
		}))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, "both be zero")
	})

	t.Run("maxUnavailable=0 alone allowed for single-node", func(t *testing.T) {
		v := intstr.FromInt32(0)
		errs := validator.validateRolloutStrategy(makeSvc(&RolloutStrategy{MaxUnavailable: &v}))
		assert.Empty(t, errs)
	})

	t.Run("maxUnavailable=0 alone rejected for multi-node", func(t *testing.T) {
		v := intstr.FromInt32(0)
		svc := makeSvc(&RolloutStrategy{MaxUnavailable: &v})
		svc.Spec.Worker = &corev1.PodSpec{}
		errs := validator.validateRolloutStrategy(svc)
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeRequired, errs[0].Type)
		assert.Contains(t, errs[0].Field, "maxSurge")
	})

	t.Run("maxUnavailable=1 alone allowed for multi-node", func(t *testing.T) {
		v := intstr.FromInt32(1)
		svc := makeSvc(&RolloutStrategy{MaxUnavailable: &v})
		svc.Spec.Worker = &corev1.PodSpec{}
		errs := validator.validateRolloutStrategy(svc)
		assert.Empty(t, errs)
	})

	t.Run("prefill rollout strategy validated independently", func(t *testing.T) {
		v := intstr.FromInt32(-1)
		svc := &LLMInferenceService{
			Spec: LLMInferenceServiceSpec{
				Prefill: &WorkloadSpec{
					RolloutStrategy: &RolloutStrategy{MaxUnavailable: &v},
				},
			},
		}
		errs := validator.validateRolloutStrategy(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "prefill")
		assert.Contains(t, errs[0].Field, "maxUnavailable")
	})
}

func TestValidateDisaggregatedSetAnnotation(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}
	for _, tt := range []struct {
		name       string
		value      *string
		inMetadata bool
		scaling    bool
		wantErr    bool
	}{
		{name: "absent is valid", value: nil},
		{name: "opted out is valid", value: ptr.To("false")},
		{name: "opted in is valid", value: ptr.To("true")},
		{name: "case-insensitive", value: ptr.To("True")},
		{name: "trimmed value is valid", value: ptr.To("  true  ")},
		// Admission checks the annotation's shape only. It cannot read the feature
		// gate, and presets merged after admission can still add spec.scaling, so
		// opting in alongside scaling must be admitted here and constrained by the
		// reconciler instead.
		{name: "opted in with scaling is admitted", value: ptr.To("true"), scaling: true},
		// The contract is exactly true/false, narrower than strconv.ParseBool.
		{name: "unrecognised value is rejected", value: ptr.To("yes"), wantErr: true},
		{name: "empty value is rejected", value: ptr.To(""), wantErr: true},
		{name: "ParseBool shorthand is rejected", value: ptr.To("1"), wantErr: true},
		// The key is read from spec.annotations only, so a value in metadata is ignored.
		{name: "metadata annotation is not validated", value: ptr.To("yes"), inMetadata: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceServiceV1Alpha2()
			if tt.value != nil {
				annotations := map[string]string{constants.LLMDisaggregatedSetAnnotationKey: *tt.value}
				if tt.inMetadata {
					svc.Annotations = annotations
				} else {
					svc.Spec.Annotations = annotations
				}
			}
			if tt.scaling {
				svc.Spec.Scaling = &ScalingSpec{}
				svc.Spec.Prefill = &WorkloadSpec{Scaling: &ScalingSpec{}}
			}

			errs := validator.validateDisaggregatedSetAnnotation(svc)

			if !tt.wantErr {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			assert.Equal(t, field.ErrorTypeNotSupported, errs[0].Type)
			assert.Equal(t, "spec.annotations["+constants.LLMDisaggregatedSetAnnotationKey+"]", errs[0].Field)
		})
	}
}

func TestValidateModelExpress(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}
	uriField := field.NewPath("spec", "model", "uri").String()

	parse := func(t *testing.T, uri string) apis.URL {
		t.Helper()
		u, err := apis.ParseURL(uri)
		require.NoError(t, err)
		return *u
	}

	tests := []struct {
		name        string
		annotations map[string]string
		modelURI    string
		loraURIs    []string
		wantFields  []string
	}{
		{name: "disabled", modelURI: "oci://registry/model:tag"},
		{name: "native s3", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "native"}, modelURI: "s3://bucket/llama"},
		{name: "native hf with remote adapter", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "native"}, modelURI: "hf://org/model", loraURIs: []string{"s3://bucket/adapter"}},
		{name: "layered pvc", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "layered"}, modelURI: "pvc://models/llama"},
		{name: "native pvc rejected", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "native"}, modelURI: "pvc://models/llama", wantFields: []string{uriField}},
		{name: "native s3 with hf adapter rejected", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "native"}, modelURI: "s3://bucket/llama", loraURIs: []string{"hf://org/adapter"}, wantFields: []string{uriField}},
		{name: "native with preset-supplied uri skips the source check", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "native"}},
		{
			name:        "annotation errors short-circuit the source check",
			annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "turbo"},
			modelURI:    "pvc://models/llama",
			wantFields:  []string{field.NewPath("metadata", "annotations").Key(constants.ModelExpressModeAnnotationKey).String()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceServiceV1Alpha2()
			svc.Annotations = tt.annotations
			svc.Spec.Model.URI = apis.URL{}
			if tt.modelURI != "" {
				svc.Spec.Model.URI = parse(t, tt.modelURI)
			}
			if len(tt.loraURIs) > 0 {
				svc.Spec.Model.LoRA = &LoRASpec{}
				for i, uri := range tt.loraURIs {
					svc.Spec.Model.LoRA.Adapters = append(svc.Spec.Model.LoRA.Adapters, LLMModelSpec{Name: ptr.To("a" + strconv.Itoa(i)), URI: parse(t, uri)})
				}
			}

			errs := validator.validateModelExpress(svc)
			require.Len(t, errs, len(tt.wantFields), "errors: %v", errs)
			for i, e := range errs {
				assert.Equal(t, tt.wantFields[i], e.Field)
			}
		})
	}
}
