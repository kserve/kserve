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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

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
