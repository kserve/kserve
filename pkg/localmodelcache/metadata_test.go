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

// A not-ready shared-PVC cache is never selected for a default (FallbackToSource)
// workload: no wait intent exists and the match falls through to nil so the
// workload keeps downloading from its original source URI. Only a CacheRequired
// workload gets the not-ready cache as a fail-closed wait intent.
func TestMatchCacheForURI_SharedPVCNotReadySelected(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache("ns-cache", "shared-pvc", false)},
	}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil, nsModels)
	assert.Nil(t, match)

	match = MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model", CacheRequired: true}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

func TestMatchCacheForURI_ClusterScopedOldestWins(t *testing.T) {
	newer := metav1.NewTime(time.Now())
	older := metav1.NewTime(time.Now().Add(-time.Hour))
	model := func(name string, created metav1.Time) v1alpha1.LocalModelCache {
		return v1alpha1.LocalModelCache{
			ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: created},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: "hf://org/model",
				ModelSize:      resource.MustParse("1Gi"),
				NodeGroups:     []string{"gpu1"},
			},
		}
	}
	newestFirst := model("z-newest", newer)
	olderFirst := model("a-oldest", older)

	// The oldest cache wins regardless of list order.
	for _, list := range []v1alpha1.LocalModelCacheList{
		{Items: []v1alpha1.LocalModelCache{newestFirst, olderFirst}},
		{Items: []v1alpha1.LocalModelCache{olderFirst, newestFirst}},
	} {
		match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, &list, nil)
		assert.NotNil(t, match)
		assert.Equal(t, "a-oldest", match.Cache)
		assert.Equal(t, "a-oldest-gpu1", match.PVCName)
	}

	// Equal creation timestamps fall back to name order.
	tied := metav1.NewTime(time.Now())
	list := v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{model("z-cache", tied), model("a-cache", tied)}}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, &list, nil)
	assert.NotNil(t, match)
	assert.Equal(t, "a-cache", match.Cache)
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

	for name, sel := range map[string]v1alpha1.LocalModelCacheSelection{
		"default policy": {StorageURI: "hf://org/model"},
		"CacheRequired":  {StorageURI: "hf://org/model", CacheRequired: true},
	} {
		t.Run(name, func(t *testing.T) {
			match := MatchCacheForURI(sel, models, nsModels)
			assert.NotNil(t, match)
			assert.Equal(t, "cluster-cache", match.Cache)
			assert.Empty(t, match.Namespace)
			assert.Equal(t, "cluster-cache-gpu1", match.PVCName)
		})
	}
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
// preempt the not-ready shared-PVC wait intent of a CacheRequired workload;
// with the default policy the not-ready cache matches nothing at all.
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
	assert.Nil(t, match)

	match = MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
		StorageURI:      "hf://org/model",
		NodeGroup:       "gpu2",
		NodeGroupExists: true,
		CacheRequired:   true,
	}, models, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

// A workload already bound to a cache stays bound to it even while the cache is
// not Ready and a newer ready cache for the same source exists. For the default
// policy that requires the import record to still attest data on the current
// claim; a CacheRequired workload stays bound regardless.
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
		CacheRequired:  true,
	}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "bound-cache", match.Cache)
	assert.Equal(t, "bound-pvc", match.PVCName)

	// Default policy without a record: the binding is released and the ready
	// cache wins.
	nsModels.Items[0].Status.SharedPVCImport = nil
	match = MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
		StorageURI:     "hf://org/model",
		BoundCache:     "bound-cache",
		BoundNamespace: "default",
	}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ready-cache", match.Cache)
	assert.Equal(t, "ready-pvc", match.PVCName)

	// Default policy with a record: the binding is kept (#6280 rule).
	nsModels.Items[0].Status.SharedPVCImport = &v1alpha1.SharedPVCImportStatus{PVCUID: "bound-pvc-uid"}
	match = MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
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

// importedSharedNSCache is a shared-PVC cache that imported successfully and then dropped to
// NotReady (re-import blocked, spec-generation bump): the import record still names the claim.
func importedSharedNSCache() v1alpha1.LocalModelNamespaceCache {
	c := sharedNSCache("ns-cache", "shared-pvc", false)
	c.Status.SharedPVCImport = &v1alpha1.SharedPVCImportStatus{PVCUID: "claim-uid"}
	return c
}

// A consumer already bound to a shared-PVC cache keeps its binding on the default
// (FallbackToSource) policy while the import record still attests that data is on
// the current claim; once the record is cleared a fresh import is underway onto a
// recreated claim and the consumer is released (it served #6280 semantics). A
// CacheRequired consumer keeps the binding in both cases (fail closed): with the
// record absent it waits for the fresh import instead of being released to
// download from its source URI. Stripping the binding would also drop the
// consumer out of the cache's re-import gate while its pods still read the
// destination.
func TestMatchCacheForURI_SharedPVCNotReadyKeepsBoundConsumer(t *testing.T) {
	for name, cache := range map[string]v1alpha1.LocalModelNamespaceCache{
		"import record present": importedSharedNSCache(),
		"import record cleared": sharedNSCache("ns-cache", "shared-pvc", false),
	} {
		t.Run(name, func(t *testing.T) {
			nsModels := &v1alpha1.LocalModelNamespaceCacheList{
				Items: []v1alpha1.LocalModelNamespaceCache{cache},
			}
			defaultMatch := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
				StorageURI:     "hf://org/model",
				BoundCache:     "ns-cache",
				BoundNamespace: "default",
			}, nil, nsModels)
			match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
				StorageURI:     "hf://org/model",
				BoundCache:     "ns-cache",
				BoundNamespace: "default",
				CacheRequired:  true,
			}, nil, nsModels)
			assert.NotNil(t, match)
			assert.Equal(t, "ns-cache", match.Cache)
			assert.Equal(t, "default", match.Namespace)
			assert.Equal(t, "shared-pvc", match.PVCName)

			if cache.Status.SharedPVCImport != nil {
				// #6280 rule: the record keeps the default-policy binding too.
				assert.NotNil(t, defaultMatch)
				assert.Equal(t, "ns-cache", defaultMatch.Cache)
			} else {
				assert.Nil(t, defaultMatch)
			}
		})
	}
}

// A consumer whose binding names a different cache is treated as unbound. A
// CacheRequired consumer still gets the not-ready shared-PVC cache as a
// fail-closed wait intent (reported like any other match; callers gate on the
// import record), never as a nil match that would fall back to the original
// source URI. A default (FallbackToSource) consumer is never routed to a
// not-ready shared-PVC cache.
func TestMatchCacheForURI_SharedPVCNotReadyWaitIntentForConsumerBoundElsewhere(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{importedSharedNSCache()},
	}
	for name, bound := range map[string]v1alpha1.LocalModelCacheSelection{
		"other cache": {
			StorageURI: "hf://org/model", BoundCache: "other-cache", BoundNamespace: "default",
		},
		"same name, cluster-scoped": {
			StorageURI: "hf://org/model", BoundCache: "ns-cache",
		},
		"same name, different namespace": {
			StorageURI: "hf://org/model", BoundCache: "ns-cache", BoundNamespace: "elsewhere",
		},
	} {
		t.Run(name, func(t *testing.T) {
			match := MatchCacheForURI(bound, nil, nsModels)
			assert.Nil(t, match)

			cacheRequired := bound
			cacheRequired.CacheRequired = true
			match = MatchCacheForURI(cacheRequired, nil, nsModels)
			assert.NotNil(t, match)
			assert.Equal(t, "ns-cache", match.Cache)
			assert.Equal(t, "default", match.Namespace)
			assert.Equal(t, "shared-pvc", match.PVCName)
		})
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
