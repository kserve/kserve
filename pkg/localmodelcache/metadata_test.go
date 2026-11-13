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

func TestMatchCacheForURI_ClusterScopedOldestWins(t *testing.T) {
	newer := metav1.NewTime(time.Now())
	older := metav1.NewTime(time.Now().Add(-time.Hour))
	items := func(order ...v1alpha1.LocalModelCache) []v1alpha1.LocalModelCache {
		return order
	}
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
		{Items: items(newestFirst, olderFirst)},
		{Items: items(olderFirst, newestFirst)},
	} {
		match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, &list, nil)
		assert.NotNil(t, match)
		assert.Equal(t, "a-oldest", match.Cache)
		assert.Equal(t, "a-oldest-gpu1", match.PVCName)
	}

	// Equal creation timestamps fall back to name order.
	tied := metav1.NewTime(time.Now())
	list := v1alpha1.LocalModelCacheList{Items: items(model("z-cache", tied), model("a-cache", tied))}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, &list, nil)
	assert.NotNil(t, match)
	assert.Equal(t, "a-cache", match.Cache)
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

func sharedNSCache(ready bool) v1alpha1.LocalModelNamespaceCache {
	ref := "shared-pvc"
	c := v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-cache", Namespace: "default", Generation: 1},
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
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache(true)},
	}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

func TestMatchCacheForURI_SharedPVCNotReadySkipped(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache(false)},
	}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil, nsModels)
	assert.Nil(t, match)
}

// importedSharedNSCache is a shared-PVC cache that imported successfully and then dropped to
// NotReady (re-import blocked, spec-generation bump): the import record still names the claim.
func importedSharedNSCache() v1alpha1.LocalModelNamespaceCache {
	c := sharedNSCache(false)
	c.Status.SharedPVCImport = &v1alpha1.SharedPVCImportStatus{PVCUID: "claim-uid"}
	return c
}

// A consumer already bound to a shared-PVC cache keeps its binding when the cache drops to
// NotReady after a successful import (re-import blocked, spec-generation bump): the data is
// still on the claim, and stripping the binding would roll pods back to downloading from
// source and drop the consumer out of the cache's re-import gate.
func TestMatchCacheForURI_SharedPVCNotReadyKeepsBoundConsumer(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{importedSharedNSCache()},
	}
	match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model", BoundCache: "ns-cache", BoundNamespace: "default"}, nil, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "shared-pvc", match.PVCName)
}

// The binding is held only by the import record. Once the claim is recreated under the same
// name, a fresh import starts and clears the record; the claim holds no data yet, so a bound
// consumer updated during that import is released to its source URI like any unbound one.
func TestMatchCacheForURI_SharedPVCNotReadyReleasesBoundConsumerWithoutRecord(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{sharedNSCache(false)},
	}
	assert.Nil(t, MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model", BoundCache: "ns-cache", BoundNamespace: "default"}, nil, nsModels))
}

func TestMatchCacheForURI_SharedPVCNotReadySkipsConsumerBoundElsewhere(t *testing.T) {
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{importedSharedNSCache()},
	}
	for name, sel := range map[string]v1alpha1.LocalModelCacheSelection{
		"other cache":                    {StorageURI: "hf://org/model", BoundCache: "other-cache", BoundNamespace: "default"},
		"same name, cluster-scoped":      {StorageURI: "hf://org/model", BoundCache: "ns-cache"},
		"same name, different namespace": {StorageURI: "hf://org/model", BoundCache: "ns-cache", BoundNamespace: "elsewhere"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, MatchCacheForURI(sel, nil, nsModels))
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

// nsCachePermutations returns every permutation of caches, so selection tests
// can prove order-independence without a random number generator.
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

// Selection must be deterministic across API list order: the oldest matching
// ready shared-PVC cache wins regardless of how the informer lists the caches,
// so a workload never moves between claims across reconciles.
func TestMatchCacheForURI_SharedPVCDeterministicAcrossListOrder(t *testing.T) {
	setup := func(name, pvcRef string) v1alpha1.LocalModelNamespaceCache {
		ref := pvcRef
		c := v1alpha1.LocalModelNamespaceCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         "default",
				Generation:        1,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
			},
			Spec: v1alpha1.LocalModelNamespaceCacheSpec{
				SourceModelUri: "hf://org/model",
				ModelSize:      resource.MustParse("1Gi"),
				PVCRef:         &ref,
			},
		}
		c.Status.MarkReady(1)
		return c
	}
	caches := []v1alpha1.LocalModelNamespaceCache{
		setup("ns-cache-a", "pvc-a"),
		setup("ns-cache-b", "pvc-b"),
		setup("ns-cache-c", "pvc-c"),
	}

	for i, ordering := range nsCachePermutations(caches) {
		match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil,
			&v1alpha1.LocalModelNamespaceCacheList{Items: ordering})
		assert.NotNil(t, match, "permutation %d", i)
		assert.Equal(t, "ns-cache-a", match.Cache, "permutation %d", i)
		assert.Equal(t, "pvc-a", match.PVCName, "permutation %d", i)
	}
}

// The oldest cache breaks name-order ties deterministically as well.
func TestMatchCacheForURI_SharedPVCNameBreaksTimestampTie(t *testing.T) {
	created := metav1.Now()
	setup := func(name, pvcRef string) v1alpha1.LocalModelNamespaceCache {
		ref := pvcRef
		c := v1alpha1.LocalModelNamespaceCache{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Generation: 1, CreationTimestamp: created},
			Spec: v1alpha1.LocalModelNamespaceCacheSpec{
				SourceModelUri: "hf://org/model",
				ModelSize:      resource.MustParse("1Gi"),
				PVCRef:         &ref,
			},
		}
		c.Status.MarkReady(1)
		return c
	}
	for _, items := range [][]v1alpha1.LocalModelNamespaceCache{
		{setup("ns-cache-b", "pvc-b"), setup("ns-cache-a", "pvc-a")},
		{setup("ns-cache-a", "pvc-a"), setup("ns-cache-b", "pvc-b")},
	} {
		match := MatchCacheForURI(v1alpha1.LocalModelCacheSelection{StorageURI: "hf://org/model"}, nil,
			&v1alpha1.LocalModelNamespaceCacheList{Items: items})
		assert.NotNil(t, match)
		assert.Equal(t, "ns-cache-a", match.Cache)
	}
}
