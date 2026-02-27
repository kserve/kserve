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

package localmodel

import (
	"context"
	"testing"
	"time"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func mustComputeSpecHash(t *testing.T, spec v1alpha1.LocalModelCacheDeploymentSpec) string {
	t.Helper()
	hash, err := computeSpecHash(spec)
	if err != nil {
		t.Fatalf("computeSpecHash failed: %v", err)
	}
	return hash
}

func TestLocalModelCacheDeploymentReconciler_CreateLocalModelCache(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	deployment := &v1alpha1.LocalModelCacheDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Generation: 1,
		},
		Spec: v1alpha1.LocalModelCacheDeploymentSpec{
			SourceModelUri: "gs://testbucket/testmodel",
			ModelSize:      resource.MustParse("4Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		WithStatusSubresource(deployment).
		Build()

	r := &LocalModelCacheDeploymentReconciler{
		Client: client,
		Log:    zap.New(zap.UseDevMode(true)),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-deployment"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify LocalModelCache was created
	expectedName := "test-deployment-" + mustComputeSpecHash(t, deployment.Spec)
	cache := &v1alpha1.LocalModelCache{}
	err = client.Get(context.Background(), types.NamespacedName{Name: expectedName}, cache)
	if err != nil {
		t.Fatalf("LocalModelCache not created: %v", err)
	}

	if cache.Spec.SourceModelUri != "gs://testbucket/testmodel" {
		t.Errorf("Expected sourceModelUri gs://testbucket/testmodel, got %s", cache.Spec.SourceModelUri)
	}
	if cache.Spec.ModelSize.String() != "4Gi" {
		t.Errorf("Expected modelSize 4Gi, got %s", cache.Spec.ModelSize.String())
	}
	if len(cache.Spec.NodeGroups) != 1 || cache.Spec.NodeGroups[0] != "gpu" {
		t.Errorf("Expected nodeGroups [gpu], got %v", cache.Spec.NodeGroups)
	}

	// Verify labels
	if cache.Labels[constants.LocalModelCacheDeploymentLabel] != "test-deployment" {
		t.Errorf("Expected deployment label, got %v", cache.Labels)
	}
	if cache.Labels[constants.LocalModelCacheRevisionLabel] != "1" {
		t.Errorf("Expected revision label 1, got %v", cache.Labels)
	}

	// Verify ownerReference
	if len(cache.OwnerReferences) != 1 {
		t.Fatalf("Expected 1 ownerReference, got %d", len(cache.OwnerReferences))
	}
	if cache.OwnerReferences[0].Name != "test-deployment" {
		t.Errorf("Expected ownerReference name test-deployment, got %s", cache.OwnerReferences[0].Name)
	}
}

func TestLocalModelCacheDeploymentReconciler_ExistingCache(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	deployment := &v1alpha1.LocalModelCacheDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Generation: 1,
			UID:        "test-uid",
		},
		Spec: v1alpha1.LocalModelCacheDeploymentSpec{
			SourceModelUri: "gs://testbucket/testmodel",
			ModelSize:      resource.MustParse("4Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}

	existingCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment-" + mustComputeSpecHash(t, deployment.Spec),
			Labels: map[string]string{
				constants.LocalModelCacheDeploymentLabel: "test-deployment",
				constants.LocalModelCacheRevisionLabel:   "1",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, v1alpha1.SchemeGroupVersion.WithKind(constants.LocalModelCacheDeploymentKind)),
			},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://testbucket/testmodel",
			ModelSize:      resource.MustParse("4Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, existingCache).
		WithStatusSubresource(deployment).
		Build()

	r := &LocalModelCacheDeploymentReconciler{
		Client: client,
		Log:    zap.New(zap.UseDevMode(true)),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-deployment"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify no duplicate cache created
	cacheList := &v1alpha1.LocalModelCacheList{}
	err = client.List(context.Background(), cacheList)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(cacheList.Items) != 1 {
		t.Errorf("Expected 1 cache, got %d", len(cacheList.Items))
	}
}

func TestLocalModelCacheDeploymentReconciler_NewRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	oldSpec := v1alpha1.LocalModelCacheDeploymentSpec{
		SourceModelUri: "gs://testbucket/testmodel",
		ModelSize:      resource.MustParse("4Gi"),
		NodeGroups:     []string{"gpu"},
	}

	newSpec := v1alpha1.LocalModelCacheDeploymentSpec{
		SourceModelUri: "gs://testbucket/testmodel-v2",
		ModelSize:      resource.MustParse("8Gi"),
		NodeGroups:     []string{"gpu"},
	}

	deployment := &v1alpha1.LocalModelCacheDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Generation: 2,
			UID:        "test-uid",
		},
		Spec: newSpec,
	}

	existingCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment-" + mustComputeSpecHash(t, oldSpec),
			Labels: map[string]string{
				constants.LocalModelCacheDeploymentLabel: "test-deployment",
				constants.LocalModelCacheRevisionLabel:   "1",
			},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: oldSpec.SourceModelUri,
			ModelSize:      oldSpec.ModelSize,
			NodeGroups:     oldSpec.NodeGroups,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, existingCache).
		WithStatusSubresource(deployment).
		Build()

	r := &LocalModelCacheDeploymentReconciler{
		Client: client,
		Log:    zap.New(zap.UseDevMode(true)),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-deployment"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify new cache created
	newCacheName := "test-deployment-" + mustComputeSpecHash(t, newSpec)
	newCache := &v1alpha1.LocalModelCache{}
	err = client.Get(context.Background(), types.NamespacedName{Name: newCacheName}, newCache)
	if err != nil {
		t.Fatalf("New LocalModelCache not created: %v", err)
	}

	if newCache.Spec.SourceModelUri != "gs://testbucket/testmodel-v2" {
		t.Errorf("Expected sourceModelUri gs://testbucket/testmodel-v2, got %s", newCache.Spec.SourceModelUri)
	}

	// Verify old cache still exists
	oldCacheName := "test-deployment-" + mustComputeSpecHash(t, oldSpec)
	oldCache := &v1alpha1.LocalModelCache{}
	err = client.Get(context.Background(), types.NamespacedName{Name: oldCacheName}, oldCache)
	if err != nil {
		t.Fatalf("Old LocalModelCache should still exist: %v", err)
	}
}

func TestComputeSpecHash(t *testing.T) {
	spec1 := v1alpha1.LocalModelCacheDeploymentSpec{
		SourceModelUri: "gs://bucket/model",
		ModelSize:      resource.MustParse("4Gi"),
		NodeGroups:     []string{"gpu"},
	}
	spec2 := v1alpha1.LocalModelCacheDeploymentSpec{
		SourceModelUri: "gs://bucket/model-v2",
		ModelSize:      resource.MustParse("4Gi"),
		NodeGroups:     []string{"gpu"},
	}

	// Same spec produces same hash
	hashOfSpec1 := mustComputeSpecHash(t, spec1)
	hashOfSpec1Again := mustComputeSpecHash(t, spec1)
	if hashOfSpec1 != hashOfSpec1Again {
		t.Error("Same spec should produce same hash")
	}

	// Different spec produces different hash
	hashOfSpec2 := mustComputeSpecHash(t, spec2)
	if hashOfSpec1 == hashOfSpec2 {
		t.Error("Different spec should produce different hash")
	}

	// RevisionHistoryLimit does not affect hash
	limit := int32(5)
	spec1WithLimit := spec1
	spec1WithLimit.RevisionHistoryLimit = &limit
	hashOfSpec1WithLimit := mustComputeSpecHash(t, spec1WithLimit)
	if hashOfSpec1 != hashOfSpec1WithLimit {
		t.Error("RevisionHistoryLimit should not affect hash")
	}
}

func TestLocalModelCacheDeploymentReconciler_CleanupOldRevisions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	limit := int32(1)
	deployment := &v1alpha1.LocalModelCacheDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Generation: 3,
			UID:        "test-uid",
		},
		Spec: v1alpha1.LocalModelCacheDeploymentSpec{
			SourceModelUri:       "gs://testbucket/testmodel-v3",
			ModelSize:            resource.MustParse("4Gi"),
			NodeGroups:           []string{"gpu"},
			RevisionHistoryLimit: &limit,
		},
		Status: v1alpha1.LocalModelCacheDeploymentStatus{
			CurrentRevision: "test-deployment-current",
		},
	}

	// Create 3 caches: oldest, middle, current
	oldest := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-deployment-oldest",
			CreationTimestamp:  metav1.NewTime(metav1.Now().Add(-2 * time.Hour)),
			Labels: map[string]string{
				constants.LocalModelCacheDeploymentLabel: "test-deployment",
			},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://testbucket/testmodel-v1",
			ModelSize:      resource.MustParse("4Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}
	middle := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-deployment-middle",
			CreationTimestamp:  metav1.NewTime(metav1.Now().Add(-1 * time.Hour)),
			Labels: map[string]string{
				constants.LocalModelCacheDeploymentLabel: "test-deployment",
			},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://testbucket/testmodel-v2",
			ModelSize:      resource.MustParse("4Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}
	current := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-deployment-current",
			CreationTimestamp:  metav1.Now(),
			Labels: map[string]string{
				constants.LocalModelCacheDeploymentLabel: "test-deployment",
			},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://testbucket/testmodel-v3",
			ModelSize:      resource.MustParse("4Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, oldest, middle, current).
		WithStatusSubresource(deployment).
		Build()

	r := &LocalModelCacheDeploymentReconciler{
		Client: c,
		Log:    zap.New(zap.UseDevMode(true)),
		Scheme: scheme,
	}

	err := r.updateAndCleanupRevisions(context.Background(), deployment)
	if err != nil {
		t.Fatalf("updateAndCleanupRevisions failed: %v", err)
	}

	// With limit=1, we keep current + 1 old = 2 total. 3 existed, so 1 should be deleted.
	cacheList := &v1alpha1.LocalModelCacheList{}
	if err := c.List(context.Background(), cacheList); err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(cacheList.Items) != 2 {
		t.Errorf("Expected 2 caches after cleanup, got %d", len(cacheList.Items))
	}

	// Current revision must survive
	currentCache := &v1alpha1.LocalModelCache{}
	err = c.Get(context.Background(), types.NamespacedName{Name: "test-deployment-current"}, currentCache)
	if err != nil {
		t.Error("Current revision should not be deleted")
	}
}
