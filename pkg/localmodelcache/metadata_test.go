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

package localmodelcache

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestMatchCacheForURI_ClusterScoped(t *testing.T) {
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cache"},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: "hf://org/model",
				ModelSize:      resource.MustParse("1Gi"),
				NodeGroups:     []string{"gpu1"},
			},
		}},
	}

	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, models, nil)
	assert.NotNil(t, match)
	assert.Equal(t, "my-cache", match.Cache)
	assert.Empty(t, match.Namespace)
	assert.Equal(t, "my-cache-gpu1", match.PVCName)
}

func TestMatchCacheForURI_NamespaceScopedPrecedence(t *testing.T) {
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: "hf://org/model",
				ModelSize:      resource.MustParse("1Gi"),
				NodeGroups:     []string{"gpu1"},
			},
		}},
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-cache", Namespace: "default"},
			Spec: v1alpha1.LocalModelNamespaceCacheSpec{
				SourceModelUri: "hf://org/model",
				ModelSize:      resource.MustParse("1Gi"),
				NodeGroups:     []string{"gpu2"},
			},
		}},
	}

	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, models, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "ns-cache-gpu2", match.PVCName)
}

func sharedNSCache(name, pvcRef string, ready bool) v1alpha1.LocalModelNamespaceCache {
	ref := pvcRef
	c := v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Generation: 1},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://org/model",
			ModelSize:      resource.MustParse("1Gi"),
			PVCRef:         &ref,
		},
	}
	if ready {
		c.Status.MarkReady(1)
	}
	return c
}

func TestMatchCacheForURI_SharedPVCReady(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache("ns-cache", "shared-pvc", true)},
	}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

// A not-ready shared-PVC cache is still selected for an unbound workload: the
// binding records the intent to serve from that cache and callers must fail
// closed instead of falling back to the original source URI.
func TestMatchCacheForURI_SharedPVCNotReadySelected(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache("ns-cache", "shared-pvc", false)},
	}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

// A servable cluster-scoped cache wins over the not-ready shared-PVC wait
// intent, so workloads that a ready cluster cache can serve keep being served.
func TestMatchCacheForURI_ClusterCacheBeatsNotReadySharedPVC(t *testing.T) {
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: "hf://org/model",
				NodeGroups:     []string{"gpu1"},
			},
		}},
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache("ns-cache", "shared-pvc", false)},
	}

	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, models, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "cluster-cache", match.Cache)
	assert.Empty(t, match.Namespace)
	assert.Equal(t, "cluster-cache-gpu1", match.PVCName)
}

// A workload bound to a cluster-scoped cache keeps that binding while a
// not-ready namespace-scoped cache for the same source exists.
func TestMatchCacheForURI_ClusterBindingKeptWhenSharedPVCNotReady(t *testing.T) {
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: "hf://org/model",
				NodeGroups:     []string{"gpu1"},
			},
		}},
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache("ns-cache", "shared-pvc", false)},
	}

	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
		StorageURI: "hf://org/model",
		BoundCache: "cluster-cache",
	}, models, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "cluster-cache", match.Cache)
	assert.Empty(t, match.Namespace)
}

// A cluster-scoped cache whose node groups cannot serve the workload does not
// preempt the not-ready shared-PVC wait intent.
func TestMatchCacheForURI_ClusterCacheNodeGroupMismatchKeepsWaitIntent(t *testing.T) {
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: "hf://org/model",
				NodeGroups:     []string{"gpu1"},
			},
		}},
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache("ns-cache", "shared-pvc", false)},
	}

	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
		StorageURI:      "hf://org/model",
		NodeGroup:       "gpu2",
		NodeGroupExists: true,
	}, models, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

// A workload already bound to a cache stays bound to it even while the cache is
// not Ready and a newer ready cache for the same source exists.
func TestMatchCacheForURI_SharedPVCStaysBound(t *testing.T) {
	older := metav1.NewTime(metav1.Now().Add(-time.Hour))
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{
			sharedNSCache("bound-cache", "bound-pvc", false),
			sharedNSCache("ready-cache", "ready-pvc", true),
		},
	}
	nsModels.Items[0].CreationTimestamp = older
	nsModels.Items[1].CreationTimestamp = older

	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
		StorageURI:     "hf://org/model",
		BoundCache:     "bound-cache",
		BoundNamespace: "default",
	}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "bound-cache", match.Cache)
	assert.Equal(t, "bound-pvc", match.PVCName)
}

// nsCachePermutations returns every permutation of items, so matching tests can
// prove order-independence without a random number generator.
func nsCachePermutations(items []v1alpha1.LocalModelNamespaceCache) [][]v1alpha1.LocalModelNamespaceCache {
	var out [][]v1alpha1.LocalModelNamespaceCache
	var permute func(current, remaining []v1alpha1.LocalModelNamespaceCache)
	permute = func(current, remaining []v1alpha1.LocalModelNamespaceCache) {
		if len(remaining) == 0 {
			out = append(out, current)
			return
		}
		for i := range remaining {
			next := make([]v1alpha1.LocalModelNamespaceCache, 0, len(current)+1)
			next = append(next, current...)
			next = append(next, remaining[i])
			rest := make([]v1alpha1.LocalModelNamespaceCache, 0, len(remaining)-1)
			rest = append(rest, remaining[:i]...)
			rest = append(rest, remaining[i+1:]...)
			permute(next, rest)
		}
	}
	permute(nil, items)
	return out
}

// Matching over every list order must select the same cache.
func TestMatchCacheForURI_SharedPVCDeterministicAcrossListOrder(t *testing.T) {
	older := metav1.NewTime(metav1.Now().Add(-time.Hour))
	caches := []v1alpha1.LocalModelNamespaceCache{
		sharedNSCache("cache-a", "pvc-a", true),
		sharedNSCache("cache-b", "pvc-b", true),
		sharedNSCache("cache-c", "pvc-c", true),
	}
	for i := range caches {
		caches[i].CreationTimestamp = older
	}

	for i, ordering := range nsCachePermutations(caches) {
		match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil,
			&v1alpha1.LocalModelNamespaceCacheList{Items: ordering})
		assert.NotNil(t, match, "permutation %d", i)
		assert.Equal(t, "cache-a", match.Cache, "permutation %d", i)
	}
}

func TestMarshalParseLoRACacheAnnotation(t *testing.T) {
	raw, err := MarshalLoRACacheAnnotation(map[string]CacheEntry{
		"adapter-a": {
			Cache:     "adapter-cache",
			SourceURI: "hf://org/adapter", // runtime-only; must not appear in JSON
			PVCName:   "adapter-cache-gpu1",
		},
	})
	assert.NoError(t, err)
	assert.NotContains(t, raw, "sourceUri")
	assert.NotContains(t, raw, "pvcName")

	entries, err := ParseLoRACacheAnnotation(raw)
	assert.NoError(t, err)
	assert.Equal(t, "adapter-cache", entries["adapter-a"].Cache)
	assert.Empty(t, entries["adapter-a"].SourceURI)
	assert.Empty(t, entries["adapter-a"].PVCName)
}

func TestBuildCachedPVCURI(t *testing.T) {
	sourceURI := "hf://org/model"
	pvcName := "my-cache-gpu1"
	got := BuildCachedPVCURI(sourceURI, pvcName, sourceURI)
	assert.True(t, strings.HasPrefix(got, "pvc://my-cache-gpu1/models/"))
	assert.True(t, strings.HasSuffix(got, "/"))

	subdir := BuildCachedPVCURI(sourceURI, pvcName, "hf://org/model/adapter-subdir")
	assert.Contains(t, subdir, "/adapter-subdir")
}

func TestBuildCachedPVCURI_TrailingSlashSourceURI(t *testing.T) {
	sourceURI := "hf://org/model/"
	pvcName := "my-cache-gpu1"
	got := BuildCachedPVCURI(sourceURI, pvcName, "hf://org/model/extra")
	assert.True(t, strings.HasPrefix(got, "pvc://my-cache-gpu1/models/"))
	assert.True(t, strings.HasSuffix(got, "/extra"))
	assert.NotContains(t, got, "hf://")
	// Hash must match download job, which keys off sourceURI as stored (with trailing slash).
	assert.Contains(t, got, v1alpha1.GetStorageKey(sourceURI))
	assert.NotContains(t, got, v1alpha1.GetStorageKey(strings.TrimSuffix(sourceURI, "/")))
}

func TestLLMISVCReferencesClusterCache_MalformedLoRAAnnotation(t *testing.T) {
	assert.True(t, LLMISVCReferencesClusterCache(
		"any-cache",
		nil,
		map[string]string{constants.LocalModelLoRAAnnotationKey: `{invalid`},
	))
	assert.False(t, LLMISVCReferencesClusterCache(
		"any-cache",
		nil,
		map[string]string{constants.LocalModelLoRAAnnotationKey: ""},
	))
}

func TestLLMISVCReferencesNamespaceCache_MalformedLoRAAnnotation(t *testing.T) {
	assert.True(t, LLMISVCReferencesNamespaceCache(
		"any-cache",
		"default",
		"default",
		nil,
		map[string]string{constants.LocalModelLoRAAnnotationKey: `{invalid`},
	))
}

func TestLLMISVCClusterCacheNames(t *testing.T) {
	t.Parallel()

	t.Run("base model label only", func(t *testing.T) {
		t.Parallel()
		got := LLMISVCClusterCacheNames(
			map[string]string{constants.LocalModelLabel: "base-cache"},
			nil,
		)
		assert.Equal(t, []string{"base-cache"}, got)
	})

	t.Run("lora annotation only", func(t *testing.T) {
		t.Parallel()
		got := LLMISVCClusterCacheNames(
			nil,
			map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"a":{"cache":"adapter-cache"}}`,
			},
		)
		assert.Equal(t, []string{"adapter-cache"}, got)
	})

	t.Run("namespace scoped base excluded", func(t *testing.T) {
		t.Parallel()
		got := LLMISVCClusterCacheNames(
			map[string]string{
				constants.LocalModelLabel:          "ns-cache",
				constants.LocalModelNamespaceLabel: "default",
			},
			nil,
		)
		assert.Empty(t, got)
	})

	t.Run("base and lora combined", func(t *testing.T) {
		t.Parallel()
		got := LLMISVCClusterCacheNames(
			map[string]string{constants.LocalModelLabel: "base-cache"},
			map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"a":{"cache":"adapter-cache"}}`,
			},
		)
		assert.Equal(t, []string{"adapter-cache", "base-cache"}, got)
	})
}

func TestLLMISVCNamespaceCacheNames(t *testing.T) {
	t.Parallel()

	t.Run("base and lora namespace caches", func(t *testing.T) {
		t.Parallel()
		got := LLMISVCNamespaceCacheNames(
			"default",
			map[string]string{
				constants.LocalModelLabel:          "ns-cache",
				constants.LocalModelNamespaceLabel: "default",
			},
			map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"a":{"cache":"adapter-ns-cache","namespace":"default"}}`,
			},
		)
		assert.Equal(t, []string{"adapter-ns-cache", "ns-cache"}, got)
	})
}
