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
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func nodeGroupForPool(name, pool string) *v1alpha1.LocalModelNodeGroup {
	return &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{pool},
						}},
					}},
				}},
			},
		},
	}
}

func workerNode(pool string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "worker",
		Labels: map[string]string{"pool": pool},
	}}
}

// Records are fanned out to every configured node group, so a node that only matches a later group
// has to reconcile the cache too, or the cleanup of a deleted node never runs for it.
func TestNodeFuncEnqueuesCacheMatchingLaterNodeGroup(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache"},
		Spec:       v1alpha1.LocalModelCacheSpec{NodeGroups: []string{"gpu", "cpu"}},
	}
	node := workerNode("cpu")
	cl := newLocalModelCacheClient(t, cache, nodeGroupForPool("gpu", "gpu"), nodeGroupForPool("cpu", "cpu"))
	r := &LocalModelReconciler{Client: cl, Log: logr.Discard()}

	requests := r.nodeFunc(context.Background(), node)
	if len(requests) != 1 || requests[0].Name != cache.Name {
		t.Fatalf("nodeFunc() = %#v, want a single request for %q", requests, cache.Name)
	}
}

func TestNodeFuncNamespaceCacheEnqueuesCacheMatchingLaterNodeGroup(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns"},
		Spec:       v1alpha1.LocalModelNamespaceCacheSpec{NodeGroups: []string{"gpu", "cpu"}},
	}
	node := workerNode("cpu")
	cl := newLocalModelCacheClient(t, cache, nodeGroupForPool("gpu", "gpu"), nodeGroupForPool("cpu", "cpu"))
	r := &LocalModelNamespaceCacheReconciler{Client: cl, Log: logr.Discard()}

	requests := r.nodeFuncNamespaceCache(context.Background(), node)
	if len(requests) != 1 || requests[0].Name != cache.Name || requests[0].Namespace != cache.Namespace {
		t.Fatalf("nodeFuncNamespaceCache() = %#v, want a single request for %q", requests, cache.Namespace+"/"+cache.Name)
	}
}

// A node that matches several groups must still enqueue the cache once.
func TestNodeFuncEnqueuesCacheOnceWhenNodeMatchesSeveralGroups(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache"},
		Spec:       v1alpha1.LocalModelCacheSpec{NodeGroups: []string{"cpu-a", "cpu-b"}},
	}
	node := workerNode("cpu")
	cl := newLocalModelCacheClient(t, cache, nodeGroupForPool("cpu-a", "cpu"), nodeGroupForPool("cpu-b", "cpu"))
	r := &LocalModelReconciler{Client: cl, Log: logr.Discard()}

	if requests := r.nodeFunc(context.Background(), node); len(requests) != 1 {
		t.Fatalf("nodeFunc() = %#v, want exactly one request", requests)
	}
}
