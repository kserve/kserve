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

package v1alpha1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/constants"
)

// The v1alpha1 validator keeps its own checklist, so the shared annotation
// check must be wired here explicitly - the python SDK creates v1alpha1 objects.
func TestValidateCreateRejectsUnsupportedLoRARoutingStrategyAnnotation(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}
	for _, tt := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid value is admitted", value: " Regex "},
		{name: "typo is rejected", value: "regexp", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceService()
			svc.Spec.Annotations = map[string]string{constants.LoRAModelRoutingStrategyAnnotationKey: tt.value}

			_, err := validator.ValidateCreate(t.Context(), svc)

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, constants.LoRAModelRoutingStrategyAnnotationKey)
		})
	}
}

func newBaseLLMInferenceService() *LLMInferenceService {
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

	oldSvc := newBaseLLMInferenceService()
	newSvc := newBaseLLMInferenceService()
	newSvc.Spec.Worker = &corev1.PodSpec{}

	// Without DeletionTimestamp, this should be rejected (worker without parallelism)
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

func TestValidateTrafficFields_V1Alpha1(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	tests := []struct {
		name      string
		llmSvc    *LLMInferenceService
		wantErr   bool
		errFields []string
	}{
		{
			name: "valid: group + weight + http",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Group:  ptr.To("llama-70b"),
							Weight: ptr.To(int32(9)),
							HTTP:   &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid: weight without group",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Weight: ptr.To(int32(9)),
							HTTP:   &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
					},
				},
			},
			wantErr:   true,
			errFields: []string{"spec.router.route.group"},
		},
		{
			name: "invalid: group without weight",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Group: ptr.To("llama-70b"),
							HTTP:  &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
					},
				},
			},
			wantErr:   true,
			errFields: []string{"spec.router.route.weight"},
		},
		{
			name: "invalid: group + weight with ingress",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Group:  ptr.To("llama-70b"),
							Weight: ptr.To(int32(9)),
							HTTP:   &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
						Ingress: &IngressSpec{},
					},
				},
			},
			wantErr:   true,
			errFields: []string{"spec.router.route.group"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateTrafficFields(tt.llmSvc)
			if tt.wantErr {
				require.NotEmpty(t, errs, "expected validation errors")
				for _, expectedField := range tt.errFields {
					found := false
					for _, err := range errs {
						if err.Field == expectedField {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error on field %s, got errors: %v", expectedField, errs)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateLoRAAdapters_V1Alpha1(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	makeAdapter := func(name, uri string) LLMModelSpec {
		return LLMModelSpec{URI: apis.URL{Scheme: "hf", Host: uri}, Name: ptr.To(name)}
	}

	makeSvc := func(modelName string, loraSpec *LoRASpec) *LLMInferenceService {
		svc := newBaseLLMInferenceService()
		svc.Spec.Model.Name = ptr.To(modelName)
		svc.Spec.Model.LoRA = loraSpec
		return svc
	}

	tests := []struct {
		name           string
		svc            *LLMInferenceService
		wantErrCount   int
		wantErrStrings []string
	}{
		{
			name:         "no lora",
			svc:          makeSvc("base", nil),
			wantErrCount: 0,
		},
		{
			name: "valid single adapter",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{makeAdapter("adapter-1", "adapter-1")},
			}),
			wantErrCount: 0,
		},
		{
			name: "adapter name missing",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{{URI: apis.URL{Scheme: "hf", Host: "adapter-1"}}},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"spec.model.lora.adapters[0].name"},
		},
		{
			name: "path traversal rejected",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{makeAdapter("..", "adapter-dotdot")},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"path traversal"},
		},
		{
			name: "duplicate adapter names",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{
					makeAdapter("dup", "adapter-1"),
					makeAdapter("dup", "adapter-2"),
				},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"duplicate"},
		},
		{
			name: "adapter name same as base model",
			svc: makeSvc("base-model", &LoRASpec{
				Adapters: []LLMModelSpec{makeAdapter("base-model", "adapter-1")},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"adapter name must differ from base model name"},
		},
		{
			name: "maxRank zero invalid",
			svc: makeSvc("base", &LoRASpec{
				MaxRank:  ptr.To(int32(0)),
				Adapters: []LLMModelSpec{makeAdapter("a", "a")},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"maxRank"},
		},
		{
			name: "all lora params valid",
			svc: makeSvc("base", &LoRASpec{
				MaxRank:        ptr.To(int32(128)),
				MaxAdapters:    ptr.To(int32(4)),
				MaxCpuAdapters: ptr.To(int32(8)),
				Adapters: []LLMModelSpec{
					makeAdapter("adapter-1", "adapter-1"),
					makeAdapter("adapter-2", "adapter-2"),
				},
			}),
			wantErrCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateLoRAAdapters(tt.svc)
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
		})
	}
}

func TestValidateManagedDRAAnnotations_V1Alpha1(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

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
				constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			},
			wantErrCount: 0,
		},
		{
			name: "invalid: device count without device class",
			annotations: map[string]string{
				constants.ManagedDRADeviceCountAnnotationKey: "2",
			},
			wantErrCount: 1,
			wantErrField: constants.ManagedDRADeviceClassAnnotationKey,
		},
		{
			name: "invalid: empty device class",
			annotations: map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "   ",
			},
			wantErrCount: 1,
			wantErrField: constants.ManagedDRADeviceClassAnnotationKey,
		},
		{
			name: "invalid: non-numeric device count",
			annotations: map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
				constants.ManagedDRADeviceCountAnnotationKey: "abc",
			},
			wantErrCount: 1,
			wantErrField: constants.ManagedDRADeviceCountAnnotationKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceService()
			svc.Annotations = tt.annotations

			// Exercise the full validate() path to ensure DRA validation is wired in.
			err := validator.validate(t.Context(), nil, svc)
			if tt.wantErrCount == 0 {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrField)
			}
		})
	}
}

// The v1alpha1 validator keeps its own checklist, so the shared DisaggregatedSet
// annotation check must be wired here explicitly. The python SDK creates v1alpha1
// objects, so both versions have to reject the same inputs.
func TestValidateCreateDisaggregatedSetAnnotation(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}
	for _, tt := range []struct {
		name    string
		value   string
		scaling bool
		wantErr bool
	}{
		{name: "opted in is admitted", value: "true"},
		{name: "opted out is admitted", value: "false"},
		// Admission cannot read the feature gate, and presets merged after admission
		// can still add spec.scaling, so this combination is the reconciler's problem.
		{name: "opted in with scaling is admitted", value: "true", scaling: true},
		{name: "malformed value is rejected", value: "yes", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceService()
			svc.Annotations = map[string]string{constants.LLMDisaggregatedSetAnnotationKey: tt.value}
			if tt.scaling {
				svc.Spec.Scaling = validDisaggScalingSpec()
				svc.Spec.Prefill = &WorkloadSpec{Scaling: validDisaggScalingSpec()}
			}

			_, err := validator.ValidateCreate(t.Context(), svc)

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, constants.LLMDisaggregatedSetAnnotationKey)
		})
	}
}

// validDisaggScalingSpec returns a ScalingSpec that satisfies the unrelated scaling
// validation rules, so these cases exercise the DisaggregatedSet check rather than
// tripping over an incomplete scaling block.
func validDisaggScalingSpec() *ScalingSpec {
	return &ScalingSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 5,
		WVA: &WVASpec{
			VariantCost: "10.0",
			ActuatorSpec: ActuatorSpec{
				HPA: &HPAScalingSpec{},
			},
		},
	}
}
