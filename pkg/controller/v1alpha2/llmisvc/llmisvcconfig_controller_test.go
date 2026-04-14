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

package llmisvc

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const testConfigFinalizerName = "serving.kserve.io/llmisvcconfig-finalizer"

func newConfigTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	return scheme
}

func newConfigTestReconciler(t *testing.T, objs ...client.Object) (*LLMISVCConfigReconciler, client.Client) {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newConfigTestScheme(t)).
		WithObjects(objs...).
		Build()

	return &LLMISVCConfigReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}, fakeClient
}

func reconcileConfig(t *testing.T, r *LLMISVCConfigReconciler, namespace, name string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: namespace, Name: name},
	})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	return result
}

func getConfig(t *testing.T, c client.Client, namespace, name string) *v1alpha2.LLMInferenceServiceConfig {
	t.Helper()
	config := &v1alpha2.LLMInferenceServiceConfig{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, config); err != nil {
		t.Fatalf("failed to get config %s/%s: %v", namespace, name, err)
	}
	return config
}

func TestLLMISVCConfigReconciler_FinalizerAdded(t *testing.T) {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-ns",
		},
	}

	r, c := newConfigTestReconciler(t, config)

	result := reconcileConfig(t, r, "test-ns", "test-config")
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}

	updated := getConfig(t, c, "test-ns", "test-config")
	if !controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Errorf("expected finalizer %q to be present, got finalizers: %v", testConfigFinalizerName, updated.Finalizers)
	}
}

func TestLLMISVCConfigReconciler_FinalizerIdempotent(t *testing.T) {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-config",
			Namespace:  "test-ns",
			Finalizers: []string{testConfigFinalizerName},
		},
	}

	r, c := newConfigTestReconciler(t, config)

	result := reconcileConfig(t, r, "test-ns", "test-config")
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}

	updated := getConfig(t, c, "test-ns", "test-config")
	if len(updated.Finalizers) != 1 {
		t.Errorf("expected exactly 1 finalizer, got %v", updated.Finalizers)
	}
}

func TestLLMISVCConfigReconciler_NotFound(t *testing.T) {
	r, _ := newConfigTestReconciler(t) // No objects

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "nonexistent"},
	})
	if err != nil {
		t.Fatalf("expected no error for not-found, got: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
}

func TestLLMISVCConfigReconciler_DeletionBlocked_BaseRefs(t *testing.T) {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-config",
			Namespace:  "test-ns",
			Finalizers: []string{testConfigFinalizerName},
		},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "my-config"}},
		},
	}

	r, c := newConfigTestReconciler(t, config, llmSvc)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, "test-ns", "my-config")
	if result.RequeueAfter != 10*time.Second {
		t.Errorf("expected requeue after 10s, got %v", result.RequeueAfter)
	}

	updated := getConfig(t, c, "test-ns", "my-config")
	if !controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to still be present when config is referenced via baseRefs")
	}
}

func TestLLMISVCConfigReconciler_DeletionBlocked_StatusAnnotations(t *testing.T) {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pinned-config",
			Namespace:  "test-ns",
			Finalizers: []string{testConfigFinalizerName},
		},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "test-ns",
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Status: duckv1.Status{
				Annotations: map[string]string{
					"serving.kserve.io/config-llm-template": "pinned-config",
				},
			},
		},
	}

	r, c := newConfigTestReconciler(t, config, llmSvc)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, "test-ns", "pinned-config")
	if result.RequeueAfter != 10*time.Second {
		t.Errorf("expected requeue after 10s, got %v", result.RequeueAfter)
	}

	updated := getConfig(t, c, "test-ns", "pinned-config")
	if !controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to still be present when config is referenced via status annotations")
	}
}

func TestLLMISVCConfigReconciler_DeletionBlocked_WellKnownConfig(t *testing.T) {
	// Well-known configs are implicitly used by all services
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       configTemplateName, // e.g. "kserve-config-llm-template"
			Namespace:  constants.KServeNamespace,
			Finalizers: []string{testConfigFinalizerName},
		},
	}
	// Service in a different namespace — system namespace configs are global
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-service",
			Namespace: "user-ns",
		},
	}

	r, c := newConfigTestReconciler(t, config, llmSvc)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, constants.KServeNamespace, configTemplateName)
	if result.RequeueAfter != 10*time.Second {
		t.Errorf("expected requeue after 10s, got %v", result.RequeueAfter)
	}

	updated := getConfig(t, c, constants.KServeNamespace, configTemplateName)
	if !controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to still be present for well-known config when any service exists")
	}
}

func TestLLMISVCConfigReconciler_DeletionBlocked_SystemNamespaceConfig(t *testing.T) {
	// Non-well-known config in system namespace, explicitly referenced by a service in another namespace
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "custom-system-config",
			Namespace:  constants.KServeNamespace,
			Finalizers: []string{testConfigFinalizerName},
		},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "user-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "custom-system-config"}},
		},
	}

	r, c := newConfigTestReconciler(t, config, llmSvc)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, constants.KServeNamespace, "custom-system-config")
	if result.RequeueAfter != 10*time.Second {
		t.Errorf("expected requeue after 10s, got %v", result.RequeueAfter)
	}

	updated := getConfig(t, c, constants.KServeNamespace, "custom-system-config")
	if !controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to still be present for system namespace config referenced across namespaces")
	}
}

func TestLLMISVCConfigReconciler_DeletionAllowed_NoReferences(t *testing.T) {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "unused-config",
			Namespace:  "test-ns",
			Finalizers: []string{testConfigFinalizerName},
		},
	}
	// Service exists but does NOT reference unused-config
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "other-config"}},
		},
	}

	r, c := newConfigTestReconciler(t, config, llmSvc)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, "test-ns", "unused-config")
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}

	// Finalizer should be removed (object may or may not still exist depending on fake client behavior)
	updated := &v1alpha2.LLMInferenceServiceConfig{}
	err := c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "unused-config"}, updated)
	if apierrors.IsNotFound(err) {
		return // Object was deleted after finalizer removal — expected
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to be removed when config is not referenced")
	}
}

func TestLLMISVCConfigReconciler_DeletionAllowed_NoServices(t *testing.T) {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "lonely-config",
			Namespace:  "test-ns",
			Finalizers: []string{testConfigFinalizerName},
		},
	}

	r, c := newConfigTestReconciler(t, config)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, "test-ns", "lonely-config")
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}

	updated := &v1alpha2.LLMInferenceServiceConfig{}
	err := c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "lonely-config"}, updated)
	if apierrors.IsNotFound(err) {
		return // Object was deleted — expected
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to be removed when no services exist")
	}
}

func TestLLMISVCConfigReconciler_DeletionAllowed_DifferentNamespace(t *testing.T) {
	// Config in user namespace, service in a DIFFERENT user namespace.
	// Non-system namespace configs are scoped to their own namespace.
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ns-scoped-config",
			Namespace:  "ns-a",
			Finalizers: []string{testConfigFinalizerName},
		},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-in-other-ns",
			Namespace: "ns-b",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "ns-scoped-config"}},
		},
	}

	r, c := newConfigTestReconciler(t, config, llmSvc)
	ctx := context.Background()

	if err := c.Delete(ctx, config); err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	result := reconcileConfig(t, r, "ns-a", "ns-scoped-config")
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for cross-namespace non-system config, got %v", result.RequeueAfter)
	}

	updated := &v1alpha2.LLMInferenceServiceConfig{}
	err := c.Get(ctx, types.NamespacedName{Namespace: "ns-a", Name: "ns-scoped-config"}, updated)
	if apierrors.IsNotFound(err) {
		return // Object was deleted — expected
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if controllerutil.ContainsFinalizer(updated, testConfigFinalizerName) {
		t.Error("expected finalizer to be removed — config in ns-a should not be blocked by service in ns-b")
	}
}

func TestLLMISVCConfigReconciler_IsConfigInUse(t *testing.T) {
	tests := []struct {
		name       string
		config     *v1alpha2.LLMInferenceServiceConfig
		services   []*v1alpha2.LLMInferenceService
		wantInUse  bool
		wantRefLen int
	}{
		{
			name: "referenced via baseRefs",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns"},
			},
			services: []*v1alpha2.LLMInferenceService{{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{{Name: "cfg"}},
				},
			}},
			wantInUse:  true,
			wantRefLen: 1,
		},
		{
			name: "referenced via status annotations",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns"},
			},
			services: []*v1alpha2.LLMInferenceService{{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
				Status: v1alpha2.LLMInferenceServiceStatus{
					Status: duckv1.Status{
						Annotations: map[string]string{"serving.kserve.io/some-key": "cfg"},
					},
				},
			}},
			wantInUse:  true,
			wantRefLen: 1,
		},
		{
			name: "not referenced",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns"},
			},
			services: []*v1alpha2.LLMInferenceService{{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{{Name: "other"}},
				},
			}},
			wantInUse:  false,
			wantRefLen: 0,
		},
		{
			name: "no services",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns"},
			},
			services:   nil,
			wantInUse:  false,
			wantRefLen: 0,
		},
		{
			name: "multiple services referencing",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-cfg", Namespace: "ns"},
			},
			services: []*v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "ns"},
					Spec: v1alpha2.LLMInferenceServiceSpec{
						BaseRefs: []corev1.LocalObjectReference{{Name: "shared-cfg"}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc2", Namespace: "ns"},
					Spec: v1alpha2.LLMInferenceServiceSpec{
						BaseRefs: []corev1.LocalObjectReference{{Name: "shared-cfg"}},
					},
				},
			},
			wantInUse:  true,
			wantRefLen: 2,
		},
		{
			name: "well-known config in system namespace",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
			},
			services: []*v1alpha2.LLMInferenceService{{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "user-ns"},
			}},
			wantInUse:  true,
			wantRefLen: 1,
		},
		{
			name: "system namespace config referenced cross-namespace",
			config: &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "system-cfg", Namespace: constants.KServeNamespace},
			},
			services: []*v1alpha2.LLMInferenceService{{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "other-ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{{Name: "system-cfg"}},
				},
			}},
			wantInUse:  true,
			wantRefLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []client.Object{tt.config}
			for _, svc := range tt.services {
				objs = append(objs, svc)
			}

			r, _ := newConfigTestReconciler(t, objs...)
			inUse, refs, err := r.isConfigInUse(context.Background(), tt.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inUse != tt.wantInUse {
				t.Errorf("isConfigInUse() = %v, want %v", inUse, tt.wantInUse)
			}
			if len(refs) != tt.wantRefLen {
				t.Errorf("referencing count = %d, want %d (refs: %v)", len(refs), tt.wantRefLen, refs)
			}
		})
	}
}
