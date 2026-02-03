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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

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
	cache := &v1alpha1.LocalModelCache{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-deployment-v1"}, cache)
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
	if cache.Labels["serving.kserve.io/localmodelcachedeployment"] != "test-deployment" {
		t.Errorf("Expected deployment label, got %v", cache.Labels)
	}
	if cache.Labels["serving.kserve.io/revision"] != "1" {
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
			Name: "test-deployment-v1",
			Labels: map[string]string{
				"serving.kserve.io/localmodelcachedeployment": "test-deployment",
				"serving.kserve.io/revision":                  "1",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, v1alpha1.SchemeGroupVersion.WithKind("LocalModelCacheDeployment")),
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

	deployment := &v1alpha1.LocalModelCacheDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Generation: 2, // New generation
			UID:        "test-uid",
		},
		Spec: v1alpha1.LocalModelCacheDeploymentSpec{
			SourceModelUri: "gs://testbucket/testmodel-v2",
			ModelSize:      resource.MustParse("8Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}

	existingCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment-v1",
			Labels: map[string]string{
				"serving.kserve.io/localmodelcachedeployment": "test-deployment",
				"serving.kserve.io/revision":                  "1",
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

	// Verify new cache created
	newCache := &v1alpha1.LocalModelCache{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-deployment-v2"}, newCache)
	if err != nil {
		t.Fatalf("New LocalModelCache not created: %v", err)
	}

	if newCache.Spec.SourceModelUri != "gs://testbucket/testmodel-v2" {
		t.Errorf("Expected sourceModelUri gs://testbucket/testmodel-v2, got %s", newCache.Spec.SourceModelUri)
	}

	// Verify old cache still exists
	oldCache := &v1alpha1.LocalModelCache{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-deployment-v1"}, oldCache)
	if err != nil {
		t.Fatalf("Old LocalModelCache should still exist: %v", err)
	}
}
