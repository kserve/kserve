// Copyright 2026 The KServe Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconcilers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func newNamespaceCacheClientset(t *testing.T) (*kubernetes.Clientset, *runtime.Scheme) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to add core scheme: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "configmaps") {
			_, _ = fmt.Fprint(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"inferenceservice-config","namespace":"kserve"}}`)
			return
		}
		// PV/PVC lookups during deletion must 404 so cleanup treats them as absent.
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"not found","reason":"NotFound","code":404}`)
	}))
	t.Cleanup(srv.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("unable to build clientset: %v", err)
	}
	return clientset, scheme
}

func TestNamespaceCacheDeletionSucceedsWhenNodeGroupDeletedFirst(t *testing.T) {
	clientset, scheme := newNamespaceCacheClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			Namespace:         "team-a",
			DeletionTimestamp: &now,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"decommissioned-group"},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()
	r := &LocalModelNamespaceCacheReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile of a deleting namespace cache must not fail when its node group is gone: %v", err)
	}

	updated := &v1alpha1.LocalModelNamespaceCache{}
	err = cl.Get(context.Background(), client.ObjectKeyFromObject(cache), updated)
	if err == nil {
		t.Fatalf("cache still exists after deletion reconcile, expected it to be reaped")
	}
	if !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after deletion reconcile, got: %v", err)
	}
}

func TestNamespaceCacheDeletionSweepsStaleEntriesFromAllLocalModelNodes(t *testing.T) {
	clientset, scheme := newNamespaceCacheClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			Namespace:         "team-a",
			DeletionTimestamp: &now,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"decommissioned-group"},
		},
	}
	// The node of the already-deleted group still carries the model entry; it
	// can only be reached by the all-node sweep.
	orphanNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-of-decommissioned-group"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: "team-a"},
			},
		},
	}
	// A node serving the same model name from another namespace, or another
	// model from this one, must be left alone: the sweep matches the exact
	// (model name, namespace) pair of the cache being deleted.
	unrelatedNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-serving-other-caches"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: "team-b"},
				{ModelName: "other-model", Namespace: "team-a"},
			},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, orphanNode, unrelatedNode).Build()
	r := &LocalModelNamespaceCacheReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	updatedOrphan := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(orphanNode), updatedOrphan); err != nil {
		t.Fatalf("failed to get node after reconcile: %v", err)
	}
	if len(updatedOrphan.Spec.LocalModels) != 0 {
		t.Fatalf("stale entry for deleted namespace cache was not swept: %+v", updatedOrphan.Spec.LocalModels)
	}
	updatedUnrelated := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(unrelatedNode), updatedUnrelated); err != nil {
		t.Fatalf("failed to get unrelated node after reconcile: %v", err)
	}
	kept := map[string]bool{}
	for _, m := range updatedUnrelated.Spec.LocalModels {
		kept[m.ModelName+"/"+m.Namespace] = true
	}
	if len(kept) != 2 || !kept["my-model/team-b"] || !kept["other-model/team-a"] {
		t.Fatalf("sweep touched entries of other caches: %+v", updatedUnrelated.Spec.LocalModels)
	}
	gone := &v1alpha1.LocalModelNamespaceCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), gone); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after deletion reconcile, got: %v", err)
	}
}

func TestNamespaceCacheDeletionAbortsWhenConfigMapUnavailable(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to add core scheme: %v", err)
	}
	// Namespace-scoped caches must explicitly delete their PVs/PVCs (no owner
	// references), which needs the job namespace from the config map. If the
	// config map cannot be read, deletion must abort so the download PVC is not
	// leaked while the finalizer is removed anyway.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "configmaps") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"not found","reason":"NotFound","code":404}`)
	}))
	defer srv.Close()
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("unable to build clientset: %v", err)
	}

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			Namespace:         "team-a",
			DeletionTimestamp: &now,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"decommissioned-group"},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()
	nodeGroups := map[string]*v1alpha1.LocalModelNodeGroup{"decommissioned-group": nil}
	if _, err := DeleteModelFromNodes(context.Background(), cl, clientset, ctrl.Log.WithName("test"), nil, cache, nodeGroups); err == nil {
		t.Fatalf("expected deletion to abort when the config map cannot be read")
	}
	kept := &v1alpha1.LocalModelNamespaceCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), kept); err != nil {
		t.Fatalf("cache disappeared despite failed cleanup: %v", err)
	}
	if !slices.Contains(kept.Finalizers, NamespaceCacheFinalizerName) {
		t.Fatalf("finalizer was removed although download PVC cleanup failed")
	}
}

const (
	namespaceCacheStorageConfigMap = `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"inferenceservice-config","namespace":"kserve"},"data":{"localModel":"{\"enabled\":true,\"jobNamespace\":\"kserve-localmodel-jobs\"}"}}`
	namespaceCachePVJSON           = `{"kind":"PersistentVolume","apiVersion":"v1","metadata":{"name":"placeholder"},"spec":{}}`
	namespaceCachePVCJSON          = `{"kind":"PersistentVolumeClaim","apiVersion":"v1","metadata":{"name":"placeholder","namespace":"kserve-localmodel-jobs"},"spec":{}}`
)

// storageRecorder records which PV/PVC paths a deletion run asked the API
// server to delete, so tests can assert storage cleanup actually happened.
type storageRecorder struct {
	mu      sync.Mutex
	deleted []string
}

func (r *storageRecorder) recordDelete(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, path)
}

func (r *storageRecorder) deletedPaths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.deleted...)
}

// newNamespaceCacheStorageClientset serves the inferenceservice-config
// ConfigMap and reports PV/PVC objects as present, so the namespace-scoped
// deletion path exercises real GET/DELETE calls instead of the not-found
// shortcut. When failFirstPVGet is set, the first PersistentVolume GET fails,
// simulating a transient API error during cleanup.
func newNamespaceCacheStorageClientset(t *testing.T, failFirstPVGet *atomic.Bool) (*kubernetes.Clientset, *runtime.Scheme, *storageRecorder) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to add core scheme: %v", err)
	}
	rec := &storageRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/configmaps/"):
			_, _ = fmt.Fprint(w, namespaceCacheStorageConfigMap)
		case strings.Contains(r.URL.Path, "/persistentvolumeclaims/"):
			if r.Method == http.MethodDelete {
				rec.recordDelete(r.URL.Path)
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = fmt.Fprint(w, namespaceCachePVCJSON)
		case strings.Contains(r.URL.Path, "/persistentvolumes/"):
			if failFirstPVGet != nil && failFirstPVGet.CompareAndSwap(true, false) && r.Method == http.MethodGet {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if r.Method == http.MethodDelete {
				rec.recordDelete(r.URL.Path)
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = fmt.Fprint(w, namespaceCachePVJSON)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","reason":"NotFound","code":404}`)
		}
	}))
	t.Cleanup(srv.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("unable to build clientset: %v", err)
	}
	return clientset, scheme, rec
}

func newDeletingNamespaceCache(name, namespace string, nodeGroups ...string) *v1alpha1.LocalModelNamespaceCache {
	now := metav1.NewTime(time.Now())
	return &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			DeletionTimestamp: &now,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     nodeGroups,
		},
	}
}

func TestNamespaceCacheDeletionDeletesStorageLeftBehindForMissingNodeGroup(t *testing.T) {
	clientset, scheme, rec := newNamespaceCacheStorageClientset(t, nil)

	cache := newDeletingNamespaceCache("my-model", "team-a", "decommissioned-group")
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()
	r := &LocalModelNamespaceCacheReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
	}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// The download PV/PVC and the serving PV are all named after the deleted
	// node group; cleanup must reach them even though the group object is gone.
	wantPaths := []string{
		"/api/v1/persistentvolumes/my-model-decommissioned-group-team-a-download",
		"/api/v1/namespaces/kserve-localmodel-jobs/persistentvolumeclaims/my-model-decommissioned-group-team-a-download",
		"/api/v1/persistentvolumes/my-model-decommissioned-group-team-a",
	}
	deleted := rec.deletedPaths()
	for _, want := range wantPaths {
		if !slices.Contains(deleted, want) {
			t.Fatalf("expected storage deletion for %q; deleted: %v", want, deleted)
		}
	}
	gone := &v1alpha1.LocalModelNamespaceCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), gone); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after deletion reconcile, got: %v", err)
	}
}

func TestNamespaceCacheDeletionRetriesWhenStorageCleanupFails(t *testing.T) {
	failFirstPVGet := &atomic.Bool{}
	failFirstPVGet.Store(true)
	clientset, scheme, rec := newNamespaceCacheStorageClientset(t, failFirstPVGet)

	cache := newDeletingNamespaceCache("my-model", "team-a", "decommissioned-group")
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()
	r := &LocalModelNamespaceCacheReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}}

	if _, err := r.Reconcile(context.Background(), req); err == nil {
		t.Fatalf("expected reconcile to fail when storage cleanup fails")
	}
	kept := &v1alpha1.LocalModelNamespaceCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), kept); err != nil {
		t.Fatalf("cache disappeared after a failed storage cleanup: %v", err)
	}
	if !slices.Contains(kept.Finalizers, NamespaceCacheFinalizerName) {
		t.Fatalf("finalizer was removed although storage cleanup failed")
	}

	// Retry: the transient failure is gone, cleanup completes and the cache is
	// reaped once the finalizer is removed.
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile after storage failure should succeed: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), kept); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after successful retry, got: %v", err)
	}
	if len(rec.deletedPaths()) != 3 {
		t.Fatalf("expected all three storage objects to be deleted on retry, deleted: %v", rec.deletedPaths())
	}
}
