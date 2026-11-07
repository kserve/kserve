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

package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func selectionSharedCache(name, pvcRef string, created metav1.Time, ready bool) LocalModelNamespaceCache {
	ref := pvcRef
	c := LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: created,
			Generation:        1,
		},
		Spec: LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://org/model",
			ModelSize:      resource.MustParse("1Gi"),
			PVCRef:         &ref,
		},
	}
	if ready {
		c.Status.MarkReady(1)
	} else {
		c.Status.MarkNotReady(ReasonImportRunning, "Import Job is running", 1)
	}
	return c
}

func selectionNodeLocalCache(name string, nodeGroups []string, created metav1.Time) LocalModelNamespaceCache {
	return LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: created,
		},
		Spec: LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://org/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     nodeGroups,
		},
	}
}

func selectionList(items ...LocalModelNamespaceCache) *LocalModelNamespaceCacheList {
	return &LocalModelNamespaceCacheList{Items: items}
}

// cachePermutations returns every permutation of items, so selection tests can
// prove order-independence without a random number generator.
func cachePermutations(items []LocalModelNamespaceCache) [][]LocalModelNamespaceCache {
	var out [][]LocalModelNamespaceCache
	var permute func(current, remaining []LocalModelNamespaceCache)
	permute = func(current, remaining []LocalModelNamespaceCache) {
		if len(remaining) == 0 {
			out = append(out, current)
			return
		}
		for i := range remaining {
			next := make([]LocalModelNamespaceCache, 0, len(current)+1)
			next = append(next, current...)
			next = append(next, remaining[i])
			rest := make([]LocalModelNamespaceCache, 0, len(remaining)-1)
			rest = append(rest, remaining[:i]...)
			rest = append(rest, remaining[i+1:]...)
			permute(next, rest)
		}
	}
	permute(nil, items)
	return out
}

func TestSelectLocalModelNamespaceCache_DeterministicAcrossListOrder(t *testing.T) {
	older := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	newer := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	caches := []LocalModelNamespaceCache{
		selectionSharedCache("cache-a", "pvc-a", older, true),
		selectionSharedCache("cache-b", "pvc-b", newer, true),
		selectionSharedCache("cache-c", "pvc-c", newer, true),
	}

	// Selection over every list order must always pick the same cache.
	for i, ordering := range cachePermutations(caches) {
		selected, waitIntent := SelectLocalModelNamespaceCache(LocalModelCacheSelection{StorageURI: "hf://org/model"}, selectionList(ordering...))
		assert.False(t, waitIntent, "permutation %d", i)
		assert.NotNil(t, selected, "permutation %d", i)
		assert.Equal(t, "cache-a", selected.Name, "permutation %d", i)
	}
}

func TestSelectLocalModelNamespaceCache_NameBreaksTimestampTie(t *testing.T) {
	created := metav1.NewTime(time.Now())
	selected, waitIntent := SelectLocalModelNamespaceCache(
		LocalModelCacheSelection{StorageURI: "hf://org/model"},
		selectionList(
			selectionSharedCache("cache-b", "pvc-b", created, true),
			selectionSharedCache("cache-a", "pvc-a", created, true),
		))
	assert.False(t, waitIntent)
	assert.NotNil(t, selected)
	assert.Equal(t, "cache-a", selected.Name)
}

func TestSelectLocalModelNamespaceCache_StaysBound(t *testing.T) {
	older := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	newer := metav1.NewTime(time.Now().Add(-1 * time.Hour))

	scenarios := map[string]struct {
		boundReady bool
	}{
		"bound cache is ready":     {boundReady: true},
		"bound cache is not ready": {boundReady: false},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			selected, waitIntent := SelectLocalModelNamespaceCache(
				LocalModelCacheSelection{
					StorageURI:     "hf://org/model",
					BoundCache:     "cache-b",
					BoundNamespace: "default",
				},
				selectionList(
					selectionSharedCache("cache-a", "pvc-a", older, true),
					selectionSharedCache("cache-b", "pvc-b", newer, scenario.boundReady),
				))
			assert.NotNil(t, selected)
			assert.Equal(t, "cache-b", selected.Name)
			// A stay-bound result is committed even while the cache is not Ready.
			assert.False(t, waitIntent)
		})
	}
}

func TestSelectLocalModelNamespaceCache_UnboundPrefersReadySharedPVC(t *testing.T) {
	older := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	newer := metav1.NewTime(time.Now().Add(-1 * time.Hour))

	t.Run("a ready cache wins over an older not-ready cache", func(t *testing.T) {
		selected, waitIntent := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{StorageURI: "hf://org/model"},
			selectionList(
				selectionSharedCache("cache-a", "pvc-a", older, false),
				selectionSharedCache("cache-b", "pvc-b", newer, true),
			))
		assert.NotNil(t, selected)
		assert.Equal(t, "cache-b", selected.Name)
		assert.False(t, waitIntent)
	})

	t.Run("only not-ready caches: the oldest is selected as wait intent", func(t *testing.T) {
		selected, waitIntent := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{StorageURI: "hf://org/model"},
			selectionList(
				selectionSharedCache("cache-a", "pvc-a", older, false),
				selectionSharedCache("cache-b", "pvc-b", newer, false),
			))
		assert.NotNil(t, selected)
		assert.Equal(t, "cache-a", selected.Name)
		// Wait intent: callers must prefer a servable cluster-scoped cache over it.
		assert.True(t, waitIntent)
	})
}

func TestSelectLocalModelNamespaceCache_ClassOrder(t *testing.T) {
	created := metav1.NewTime(time.Now())

	t.Run("ready shared-PVC wins over node-local", func(t *testing.T) {
		selected, _ := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{StorageURI: "hf://org/model"},
			selectionList(
				selectionNodeLocalCache("node-cache", []string{"gpu1"}, created),
				selectionSharedCache("shared-cache", "pvc", created, true),
			))
		assert.NotNil(t, selected)
		assert.Equal(t, "shared-cache", selected.Name)
	})

	t.Run("node-local wins over not-ready shared-PVC", func(t *testing.T) {
		selected, _ := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{StorageURI: "hf://org/model"},
			selectionList(
				selectionNodeLocalCache("node-cache", []string{"gpu1"}, created),
				selectionSharedCache("shared-cache", "pvc", created, false),
			))
		assert.NotNil(t, selected)
		assert.Equal(t, "node-cache", selected.Name)
	})
}

func TestSelectLocalModelNamespaceCache_NodeGroup(t *testing.T) {
	created := metav1.NewTime(time.Now())

	t.Run("node group annotation must be served by node-local cache", func(t *testing.T) {
		selected, _ := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{StorageURI: "hf://org/model", NodeGroup: "gpu2", NodeGroupExists: true},
			selectionList(
				selectionNodeLocalCache("node-cache", []string{"gpu1"}, created),
				selectionSharedCache("shared-cache", "pvc", created, false),
			))
		// The node-local cache does not serve gpu2, so the not-ready shared cache
		// is selected as the fail-closed wait intent.
		assert.NotNil(t, selected)
		assert.Equal(t, "shared-cache", selected.Name)
	})

	t.Run("bound node-local cache that no longer serves the node group is dropped", func(t *testing.T) {
		selected, _ := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{
				StorageURI:      "hf://org/model",
				NodeGroup:       "gpu2",
				NodeGroupExists: true,
				BoundCache:      "node-cache",
				BoundNamespace:  "default",
			},
			selectionList(
				selectionNodeLocalCache("node-cache", []string{"gpu1"}, created),
				selectionNodeLocalCache("node-cache-2", []string{"gpu2"}, created),
			))
		assert.NotNil(t, selected)
		assert.Equal(t, "node-cache-2", selected.Name)
	})

	t.Run("without annotation the first node group of a node-local cache is used", func(t *testing.T) {
		selected, _ := SelectLocalModelNamespaceCache(
			LocalModelCacheSelection{StorageURI: "hf://org/model"},
			selectionList(selectionNodeLocalCache("node-cache", []string{"gpu1", "gpu2"}, created)))
		assert.NotNil(t, selected)
		assert.Equal(t, "node-cache", selected.Name)
	})
}

func TestSelectLocalModelNamespaceCache_NoMatch(t *testing.T) {
	created := metav1.NewTime(time.Now())
	assert.Nil(t, mustNoSelection(t,
		LocalModelCacheSelection{StorageURI: "hf://org/other-model"},
		selectionList(selectionSharedCache("cache-a", "pvc-a", created, true))))
	assert.Nil(t, mustNoSelection(t, LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil))
	assert.Nil(t, mustNoSelection(t, LocalModelCacheSelection{}, selectionList(selectionSharedCache("cache-a", "pvc-a", created, true))))
}

func mustNoSelection(t *testing.T, sel LocalModelCacheSelection, nsModels *LocalModelNamespaceCacheList) *LocalModelNamespaceCache {
	t.Helper()
	selected, _ := SelectLocalModelNamespaceCache(sel, nsModels)
	return selected
}

func TestNodeLocalCacheServesNodeGroup(t *testing.T) {
	assert.True(t, NodeLocalCacheServesNodeGroup([]string{"gpu1", "gpu2"}, "gpu2", true))
	assert.False(t, NodeLocalCacheServesNodeGroup([]string{"gpu1"}, "gpu2", true))
	assert.True(t, NodeLocalCacheServesNodeGroup([]string{"gpu1"}, "", false))
	assert.False(t, NodeLocalCacheServesNodeGroup(nil, "", false))
}
