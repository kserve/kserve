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

package reconcilers

import (
	"context"
	"slices"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func namespaceCacheISVC(labels map[string]string) *v1beta1.InferenceService {
	return &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "isvc", Namespace: "ns", Labels: labels},
	}
}

func isvcCacheLabels(cache, namespace string) map[string]string {
	return map[string]string{
		constants.LocalModelLabel:          cache,
		constants.LocalModelNamespaceLabel: namespace,
	}
}

func TestNamespaceCacheISVCPredicateUpdate(t *testing.T) {
	testCases := []struct {
		name     string
		old      client.Object
		new      client.Object
		expected bool
	}{
		{
			name:     "moving to another cache in the same namespace is an update",
			old:      namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
			new:      namespaceCacheISVC(isvcCacheLabels("cache-b", "ns")),
			expected: true,
		},
		{
			name:     "moving to another namespace is an update",
			old:      namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
			new:      namespaceCacheISVC(isvcCacheLabels("cache-a", "other")),
			expected: true,
		},
		{
			name:     "becoming a cache consumer is an update",
			old:      namespaceCacheISVC(nil),
			new:      namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
			expected: true,
		},
		{
			name:     "an unrelated label change is not an update",
			old:      namespaceCacheISVC(map[string]string{constants.LocalModelLabel: "cache-a", constants.LocalModelNamespaceLabel: "ns", "team": "a"}),
			new:      namespaceCacheISVC(map[string]string{constants.LocalModelLabel: "cache-a", constants.LocalModelNamespaceLabel: "ns", "team": "b"}),
			expected: false,
		},
		{
			name:     "an unchanged object is not an update",
			old:      namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
			new:      namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
			expected: false,
		},
	}

	predicate := NamespaceCacheISVCPredicate()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := predicate.Update(event.UpdateEvent{ObjectOld: tc.old, ObjectNew: tc.new}); got != tc.expected {
				t.Fatalf("Update() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func newNamespaceCacheReconciler(t *testing.T, objs ...client.Object) *LocalModelNamespaceCacheReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("add v1beta1 scheme: %v", err)
	}
	return &LocalModelNamespaceCacheReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Log:    logr.Discard(),
	}
}

func namespaceCache(name, namespace string) *v1alpha1.LocalModelNamespaceCache {
	return &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

func drainRequests(t *testing.T, q workqueue.TypedRateLimitingInterface[reconcile.Request]) []types.NamespacedName {
	t.Helper()
	requests := make([]types.NamespacedName, 0, q.Len())
	for q.Len() > 0 {
		request, shutdown := q.Get()
		if shutdown {
			break
		}
		requests = append(requests, request.NamespacedName)
		q.Done(request)
		q.Forget(request)
	}
	return requests
}

func TestNamespaceCacheISVCHandlerEnqueuesBothCachesOnMove(t *testing.T) {
	c := newNamespaceCacheReconciler(t, namespaceCache("cache-a", "ns"), namespaceCache("cache-b", "ns"))
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()

	cacheRequestsHandler(c.isvcFuncNamespaceCache).Update(context.Background(), event.UpdateEvent{
		ObjectOld: namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
		ObjectNew: namespaceCacheISVC(isvcCacheLabels("cache-b", "ns")),
	}, q)

	got := drainRequests(t, q)
	want := []types.NamespacedName{
		{Namespace: "ns", Name: "cache-a"},
		{Namespace: "ns", Name: "cache-b"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("enqueued %v, want %v", got, want)
	}
}

func TestNamespaceCacheISVCHandlerEnqueuesJoinedCacheOnCreate(t *testing.T) {
	c := newNamespaceCacheReconciler(t, namespaceCache("cache-a", "ns"))
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()

	cacheRequestsHandler(c.isvcFuncNamespaceCache).Create(context.Background(), event.CreateEvent{
		Object: namespaceCacheISVC(isvcCacheLabels("cache-a", "ns")),
	}, q)

	got := drainRequests(t, q)
	want := []types.NamespacedName{{Namespace: "ns", Name: "cache-a"}}
	if !slices.Equal(got, want) {
		t.Fatalf("enqueued %v, want %v", got, want)
	}
}
