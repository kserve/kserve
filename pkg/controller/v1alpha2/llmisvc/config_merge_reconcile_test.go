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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestReconcileBaseRefs_DryRunValidatesRenderedSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "custom-config"}},
			Model: v1alpha2.LLMModelSpec{
				Name: ptr.To("test-model"),
			},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			AppliedConfigRefs: []v1alpha2.AppliedConfigRef{{
				Name:      "stale-config",
				Namespace: "test-ns",
				Source:    v1alpha2.AppliedConfigSourcePreset,
			}},
		},
	}
	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configTemplateName,
			Namespace: constants.KServeNamespace,
		},
	}
	customConfig := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-config",
			Namespace: llmSvc.Namespace,
		},
	}

	validationErr := apierrors.NewInvalid(
		v1alpha2.LLMInferenceServiceGVK.GroupKind(),
		"config-validation-random",
		field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "router", "route", "http", "spec", "rules").Index(0).
					Child("matches").Index(0).Child("headers").Index(0).Child("name"),
				"invalid header",
				"must match the HTTP header name pattern",
			),
		},
	)
	var validated *v1alpha2.LLMInferenceService
	var createOptions client.CreateOptions
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(template, customConfig).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				var ok bool
				validated, ok = obj.(*v1alpha2.LLMInferenceService)
				require.True(t, ok)
				for _, opt := range opts {
					opt.ApplyToCreate(&createOptions)
				}
				return validationErr
			},
		}).
		Build()
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(1),
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})

	require.NoError(t, err, "permanent validation errors should not trigger requeue")
	assert.Nil(t, combined)
	require.NotNil(t, validated)
	assert.Equal(t, "test-llm-validation", validated.Name)
	assert.Equal(t, llmSvc.Namespace, validated.Namespace)
	assert.Equal(t, llmSvc.Spec, validated.Spec)
	assert.Equal(t, []string{metav1.DryRunAll}, createOptions.DryRun)
	condition := llmSvc.Status.GetCondition(v1alpha2.PresetsCombined)
	require.NotNil(t, condition)
	assert.Equal(t, "InvalidRenderedConfig", condition.Reason)
	assert.Contains(t, condition.Message, "Preset:kserve/kserve-config-llm-template")
	assert.Contains(t, condition.Message, "UserRef:test-ns/custom-config")
	assert.NotEmpty(t, llmSvc.Status.AppliedConfigRefs, "AppliedConfigRefs should be retained for provenance")
}

func TestCombineBaseRefsConfig_ResolvesAndClearsSchedulerConfigRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const (
		namespace     = "test-ns"
		configMapName = "scheduler-config"
	)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "external-pool"},
					},
					Config: &v1alpha2.SchedulerConfigSpec{
						Ref: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
						},
					},
				},
			},
		},
	}
	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
	}
	schedulerConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: namespace},
		Data: map[string]string{"epp": `
apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins: []
`},
	}
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(template, schedulerConfig).
			Build(),
	}

	combined, err := reconciler.combineBaseRefsConfig(t.Context(), llmSvc, &Config{})

	require.NoError(t, err)
	require.NotNil(t, combined.ResolvedSchedulerConfigMap)
	assert.Equal(t, types.NamespacedName{Namespace: namespace, Name: configMapName}, *combined.ResolvedSchedulerConfigMap)
	require.NotNil(t, combined.Config.Spec.Router.Scheduler.Config.Inline)
	assert.Contains(t, string(combined.Config.Spec.Router.Scheduler.Config.Inline.Raw), "EndpointPickerConfig")
	assert.Nil(t, combined.Config.Spec.Router.Scheduler.Config.Ref)
}

func TestReconcileBaseRefs_MissingConfigDoesNotPanicOrPopulateAppliedRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{
				{Name: "missing-config"},
			},
		},
	}

	assert.NotPanics(t, func() {
		combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
		// configNotFoundError is handled gracefully: no error returned,
		// watch on LLMInferenceServiceConfig re-triggers when the config appears.
		assert.NoError(t, err)
		assert.Nil(t, combined)
	})

	assert.Empty(t, llmSvc.Status.AppliedConfigRefs)
}

func TestReconcileBaseRefs_ClearsAppliedConfigRefsOnError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{
				{Name: "missing-config"},
			},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			AppliedConfigRefs: []v1alpha2.AppliedConfigRef{
				{Name: "stale-config", Namespace: "test-ns", Source: v1alpha2.AppliedConfigSourcePreset},
			},
		},
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
	assert.NoError(t, err)
	assert.Nil(t, combined)

	assert.Nil(t, llmSvc.Status.AppliedConfigRefs, "stale AppliedConfigRefs should be cleared on error")
}

func TestReconcileBaseRefs_PreservesAppliedConfigRefsWhenStopped(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	existingRefs := []v1alpha2.AppliedConfigRef{
		{Name: "prev-config", Namespace: "test-ns", Source: v1alpha2.AppliedConfigSourcePreset},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
			Annotations: map[string]string{
				constants.StopAnnotationKey: "true",
			},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{
				{Name: "missing-config"},
			},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			AppliedConfigRefs: existingRefs,
		},
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
	assert.NoError(t, err)
	assert.NotNil(t, combined)

	assert.Equal(t, existingRefs, llmSvc.Status.AppliedConfigRefs, "AppliedConfigRefs should be preserved when service is stopped")
}
