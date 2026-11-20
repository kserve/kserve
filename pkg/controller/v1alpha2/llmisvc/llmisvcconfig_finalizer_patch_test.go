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

package llmisvc_test

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

const unitConfigFinalizer = constants.KServeAPIGroupName + "/llmisvcconfig-finalizer"

type recordedPatch struct {
	patchType types.PatchType
	data      []byte
}

// newConfigReconcilerWithRecorder builds a LLMISVCConfigReconciler over a fake client that fails
// the test on any full-object Update and records every Patch it receives.
func newConfigReconcilerWithRecorder(t *testing.T, patchFn func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error, objs ...client.Object) (*llmisvc.LLMISVCConfigReconciler, client.Client, *[]recordedPatch) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}

	var patches []recordedPatch
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha2.LLMInferenceServiceConfig{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				t.Errorf("unexpected full-object Update of %T %s", obj, client.ObjectKeyFromObject(obj))
				return c.Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				data, err := patch.Data(obj)
				if err != nil {
					t.Fatalf("failed to compute patch data: %v", err)
				}
				patches = append(patches, recordedPatch{patchType: patch.Type(), data: data})
				if patchFn != nil {
					return patchFn(ctx, c, obj, patch, opts...)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	return &llmisvc.LLMISVCConfigReconciler{
		Client:        c,
		EventRecorder: record.NewFakeRecorder(10),
	}, c, &patches
}

func configWithCPU(name, namespace string, finalizers ...string) *v1alpha2.LLMInferenceServiceConfig {
	return &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: finalizers,
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "vllm/vllm-openai:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("6")},
						},
					}},
				},
			},
		},
	}
}

// assertFinalizersOnlyPatch checks the patch touches nothing but metadata.finalizers,
// carrying metadata.resourceVersion for optimistic concurrency.
func assertFinalizersOnlyPatch(t *testing.T, p recordedPatch) {
	t.Helper()

	if p.patchType != types.MergePatchType {
		t.Errorf("expected patch type %q, got %q", types.MergePatchType, p.patchType)
	}

	var body map[string]map[string]json.RawMessage
	if err := json.Unmarshal(p.data, &body); err != nil {
		t.Fatalf("patch is not a metadata-only object: %v: %s", err, p.data)
	}
	if len(body) != 1 || body["metadata"] == nil {
		t.Errorf("expected patch to contain only metadata, got %s", p.data)
	}
	metadata := body["metadata"]
	if len(metadata) != 2 || metadata["finalizers"] == nil || metadata["resourceVersion"] == nil {
		t.Errorf("expected metadata patch to contain only finalizers and resourceVersion, got %s", p.data)
	}
}

func reconcileConfig(t *testing.T, r *llmisvc.LLMISVCConfigReconciler, config *v1alpha2.LLMInferenceServiceConfig) error {
	t.Helper()
	_, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(config)})
	return err
}

func TestLLMISVCConfigReconciler_AddFinalizer_PatchesOnlyFinalizers(t *testing.T) {
	// given
	config := configWithCPU("cfg", "default", "other.io/keep")
	r, c, patches := newConfigReconcilerWithRecorder(t, nil, config)

	// when
	if err := reconcileConfig(t, r, config); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// then
	if len(*patches) != 1 {
		t.Fatalf("expected exactly one patch, got %d", len(*patches))
	}
	assertFinalizersOnlyPatch(t, (*patches)[0])

	current := &v1alpha2.LLMInferenceServiceConfig{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(config), current); err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	if want := []string{"other.io/keep", unitConfigFinalizer}; !equality.Semantic.DeepEqual(current.Finalizers, want) {
		t.Errorf("expected finalizers %v, got %v", want, current.Finalizers)
	}
	if !equality.Semantic.DeepEqual(current.Spec, config.Spec) {
		t.Errorf("spec changed by finalizer patch: got %+v, want %+v", current.Spec, config.Spec)
	}
}

func TestLLMISVCConfigReconciler_RemoveFinalizer_PatchesOnlyFinalizers(t *testing.T) {
	// given - a terminating config no service references
	config := configWithCPU("cfg", "default", "other.io/keep", unitConfigFinalizer)
	config.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	r, c, patches := newConfigReconcilerWithRecorder(t, nil, config)

	// when
	if err := reconcileConfig(t, r, config); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// then
	if len(*patches) != 1 {
		t.Fatalf("expected exactly one patch, got %d", len(*patches))
	}
	assertFinalizersOnlyPatch(t, (*patches)[0])

	current := &v1alpha2.LLMInferenceServiceConfig{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(config), current); err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	if want := []string{"other.io/keep"}; !equality.Semantic.DeepEqual(current.Finalizers, want) {
		t.Errorf("expected finalizers %v, got %v", want, current.Finalizers)
	}
	if !equality.Semantic.DeepEqual(current.Spec, config.Spec) {
		t.Errorf("spec changed by finalizer patch: got %+v, want %+v", current.Spec, config.Spec)
	}
}

func TestLLMISVCConfigReconciler_AddFinalizer_ConflictIsPropagated(t *testing.T) {
	// given - another actor adds its own finalizer between our read and our patch
	config := configWithCPU("cfg", "default")
	concurrentWrite := func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
		latest := &v1alpha2.LLMInferenceServiceConfig{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
			return err
		}
		controllerutil.AddFinalizer(latest, "other.io/concurrent")
		if err := c.Update(ctx, latest); err != nil {
			return err
		}
		return c.Patch(ctx, obj, patch, opts...)
	}
	r, c, _ := newConfigReconcilerWithRecorder(t, concurrentWrite, config)

	// when
	err := reconcileConfig(t, r, config)

	// then - the stale patch is rejected rather than overwriting the concurrent finalizer
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected conflict error, got %v", err)
	}

	current := &v1alpha2.LLMInferenceServiceConfig{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(config), current); err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	if want := []string{"other.io/concurrent"}; !equality.Semantic.DeepEqual(current.Finalizers, want) {
		t.Errorf("expected finalizers %v, got %v", want, current.Finalizers)
	}
}

func TestLLMISVCConfigReconciler_Delete_BlockedWhileReferenced(t *testing.T) {
	// given - a terminating config still referenced via spec.baseRefs
	config := configWithCPU("cfg", "default", unitConfigFinalizer)
	config.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "cfg"}},
		},
	}
	r, c, patches := newConfigReconcilerWithRecorder(t, nil, config, llmSvc)

	// when
	if err := reconcileConfig(t, r, config); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// then
	if len(*patches) != 0 {
		t.Errorf("expected no patches while deletion is blocked, got %d", len(*patches))
	}

	current := &v1alpha2.LLMInferenceServiceConfig{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(config), current); err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	if !controllerutil.ContainsFinalizer(current, unitConfigFinalizer) {
		t.Errorf("expected finalizer %q to remain while referenced", unitConfigFinalizer)
	}
	inUse := current.GetStatus().GetCondition(v1alpha2.ConfigInUseConditionType)
	if inUse == nil || inUse.Status != corev1.ConditionTrue || inUse.Reason != "DeletionBlocked" {
		t.Errorf("expected ConfigInUse=True with reason DeletionBlocked, got %+v", inUse)
	}
}
