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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// newCacheReconcilerClientset returns a fake-client scheme plus a typed clientset
// backed by an in-memory server that serves the inferenceservice-config
// ConfigMap read at the start of every reconcile.
func newCacheReconcilerClientset(t *testing.T) (*kubernetes.Clientset, *runtime.Scheme) {
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
		_, _ = fmt.Fprint(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"inferenceservice-config","namespace":"kserve"}}`)
	}))
	t.Cleanup(srv.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("unable to build clientset: %v", err)
	}
	return clientset, scheme
}

func TestLocalModelCacheDeletionSucceedsWhenNodeGroupDeletedFirst(t *testing.T) {
	clientset, scheme := newCacheReconcilerClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			// The referenced node group no longer exists: it was deleted
			// (or renamed) before the cache. Nothing guards that order.
			NodeGroups: []string{"decommissioned-group"},
		},
	}

	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()

	r := &LocalModelReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name},
	})
	if err != nil {
		t.Fatalf("reconcile of a deleting cache must not fail when its node group is gone: %v", err)
	}

	// Removing the finalizer lets the API server reap the deleting object; the
	// fake client mirrors that by dropping the object once it carries a deletion
	// timestamp and no finalizers. A cache whose finalizer was not removed would
	// still exist here, so "gone" is exactly the property under test.
	updated := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), updated); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be reaped after deletion reconcile, got: %v", err)
	}
}

func TestNonDeletingCacheWithMissingNodeGroupStillErrors(t *testing.T) {
	clientset, scheme := newCacheReconcilerClientset(t)

	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-model",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"decommissioned-group"},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()
	r := &LocalModelReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name},
	})
	if err == nil {
		t.Fatalf("live cache with a missing node group should keep erroring (loud), not silently succeed")
	}
}

func TestDeletionSweepsStaleEntriesFromAllLocalModelNodes(t *testing.T) {
	clientset, scheme := newCacheReconcilerClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"decommissioned-group"},
		},
	}
	// Node of the already-deleted group still carries the model entry.
	node := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-of-decommissioned-group"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
				{ModelName: "other-model", Namespace: ""},
			},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, node).Build()
	r := &LocalModelReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name},
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	updatedNode := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(node), updatedNode); err != nil {
		t.Fatalf("failed to get node after reconcile: %v", err)
	}
	unrelatedEntryPresent := false
	for _, m := range updatedNode.Spec.LocalModels {
		if m.ModelName == "my-model" {
			t.Fatalf("stale entry for deleted cache still present on node")
		}
		if m.ModelName == "other-model" {
			unrelatedEntryPresent = true
		}
	}
	if !unrelatedEntryPresent {
		t.Fatalf("unrelated model entry was removed by the sweep")
	}
}

func TestDeletionSweepFailureKeepsFinalizerForRetry(t *testing.T) {
	clientset, scheme := newCacheReconcilerClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"decommissioned-group"},
		},
	}
	// Node of the already-deleted group still carries the model entry.
	node := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-of-decommissioned-group"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
			},
		},
	}
	failSweepOnce := true
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, node).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if _, ok := obj.(*v1alpha1.LocalModelNode); ok && failSweepOnce {
					failSweepOnce = false
					return apierr.NewInternalError(errors.New("injected sweep failure"))
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := &LocalModelReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cache.Name}}

	// The sweep must run before the finalizer is removed: a failed sweep leaves
	// the cache in place (finalizer intact) so the next reconcile retries it.
	if _, err := r.Reconcile(context.Background(), req); err == nil {
		t.Fatalf("expected reconcile to fail when the sweep fails")
	}
	kept := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), kept); err != nil {
		t.Fatalf("cache disappeared after a failed sweep; expected it to be retained for retry: %v", err)
	}
	if !slices.Contains(kept.Finalizers, FinalizerName) {
		t.Fatalf("finalizer was removed although the sweep failed")
	}

	// Retry: sweep succeeds, finalizer is removed, cache is reaped.
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile after sweep failure should succeed: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), kept); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after successful retry, got: %v", err)
	}
}

func TestDeletionHandlesMixedExistingAndMissingNodeGroups(t *testing.T) {
	clientset, scheme := newCacheReconcilerClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"live-group", "decommissioned-group"},
		},
	}
	// Existing group with a matching node; its entries go through the regular
	// group-membership path.
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "live-group"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"node-of-live-group"},
							}},
						}},
					},
				},
			},
		},
	}
	coreNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-of-live-group",
			Labels: map[string]string{"kubernetes.io/hostname": "node-of-live-group"},
		},
	}
	liveNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-of-live-group"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
				{ModelName: "other-model", Namespace: ""},
			},
		},
	}
	// Node of the already-deleted group still carries the model entry; it can
	// only be reached by the all-node sweep.
	orphanNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-of-decommissioned-group"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
			},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, nodeGroup, coreNode, liveNode, orphanNode).Build()
	r := &LocalModelReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name},
	}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	updatedLive := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: liveNode.Name}, updatedLive); err != nil {
		t.Fatalf("failed to get live node after reconcile: %v", err)
	}
	if len(updatedLive.Spec.LocalModels) != 1 || updatedLive.Spec.LocalModels[0].ModelName != "other-model" {
		t.Fatalf("live group node entries not cleaned precisely: %+v", updatedLive.Spec.LocalModels)
	}
	updatedOrphan := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: orphanNode.Name}, updatedOrphan); err != nil {
		t.Fatalf("failed to get orphan node after reconcile: %v", err)
	}
	if len(updatedOrphan.Spec.LocalModels) != 0 {
		t.Fatalf("orphan node entry was not swept: %+v", updatedOrphan.Spec.LocalModels)
	}
	gone := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), gone); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after deletion, got: %v", err)
	}
}

func TestDeletionCleansEntriesFromExistingNodeGroupNodes(t *testing.T) {
	clientset, scheme := newCacheReconcilerClientset(t)

	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-model",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"live-group"},
		},
	}
	// Every referenced node group still exists, so cleanup must go through
	// group membership (ready and not-ready nodes alike) and the all-node
	// sweep must not run.
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "live-group"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"node-of-live-group", "node-of-live-group-notready"},
							}},
						}},
					},
				},
			},
		},
	}
	readyCoreNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-of-live-group",
			Labels: map[string]string{"kubernetes.io/hostname": "node-of-live-group"},
		},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	}
	notReadyCoreNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-of-live-group-notready",
			Labels: map[string]string{"kubernetes.io/hostname": "node-of-live-group-notready"},
		},
	}
	readyModelNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: readyCoreNode.Name},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
				{ModelName: "other-model", Namespace: ""},
			},
		},
	}
	notReadyModelNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: notReadyCoreNode.Name},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
			},
		},
	}
	// Not part of the group's affinity; if the all-node sweep ran, it would
	// remove this entry too, so its survival proves deletion stayed on the
	// group-membership path.
	outsiderNode := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-outside-the-group"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "my-model", Namespace: ""},
			},
		},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(cache, nodeGroup, readyCoreNode, notReadyCoreNode, readyModelNode, notReadyModelNode, outsiderNode).
		Build()
	r := &LocalModelReconciler{
		Client:    cl,
		Clientset: clientset,
		Log:       ctrl.Log.WithName("test"),
		Scheme:    scheme,
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cache.Name},
	}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	for _, want := range []struct {
		name string
		kept []string // model names that must survive on the node
	}{
		{name: readyCoreNode.Name, kept: []string{"other-model"}},
		{name: notReadyCoreNode.Name, kept: nil},
	} {
		got := &v1alpha1.LocalModelNode{}
		if err := cl.Get(context.Background(), types.NamespacedName{Name: want.name}, got); err != nil {
			t.Fatalf("failed to get node %q after reconcile: %v", want.name, err)
		}
		if len(got.Spec.LocalModels) != len(want.kept) {
			t.Fatalf("node %q entries not cleaned precisely: got %+v, want %d entries", want.name, got.Spec.LocalModels, len(want.kept))
		}
		for i, m := range got.Spec.LocalModels {
			if m.ModelName != want.kept[i] {
				t.Fatalf("node %q entry %d = %q, want %q", want.name, i, m.ModelName, want.kept[i])
			}
		}
	}
	outsider := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: outsiderNode.Name}, outsider); err != nil {
		t.Fatalf("failed to get outsider node: %v", err)
	}
	if len(outsider.Spec.LocalModels) != 1 || outsider.Spec.LocalModels[0].ModelName != "my-model" {
		t.Fatalf("outsider entry was touched; deletion should stay on the group-membership path: %+v", outsider.Spec.LocalModels)
	}
	gone := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cache), gone); !apierr.IsNotFound(err) {
		t.Fatalf("expected cache to be gone after deletion, got: %v", err)
	}
}
