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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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

	match := MatchCacheForURI("hf://org/model", "", false, models, nil)
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

	match := MatchCacheForURI("hf://org/model", "", false, models, nsModels)
	assert.NotNil(t, match)
	assert.Equal(t, "ns-cache", match.Cache)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "ns-cache-gpu2", match.PVCName)
}

func TestMatchCacheForURIWithNodeSelector(t *testing.T) {
	const (
		storageURI = "hf://org/model"
		gpuKey     = "nvidia.com/gpu.product"
		h100       = "NVIDIA-H100-80GB-HBM3"
		a100       = "NVIDIA-A100-PCIE-40GB"
	)
	models := &v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: storageURI,
			NodeGroups:     []string{"h100-workers", "a100-workers"},
		},
	}}}
	nodeGroups := &v1alpha1.LocalModelNodeGroupList{Items: []v1alpha1.LocalModelNodeGroup{
		newNodeGroup("h100-workers", gpuKey, h100),
		newNodeGroup("a100-workers", gpuKey, a100),
	}}

	tests := []struct {
		name            string
		nodeGroup       string
		nodeGroupExists bool
		nodeSelector    map[string]string
		nodeGroups      *v1alpha1.LocalModelNodeGroupList
		models          *v1alpha1.LocalModelCacheList
		wantPVC         string
	}{
		{
			name:         "selects compatible node group",
			nodeSelector: map[string]string{gpuKey: a100},
			models:       models,
			wantPVC:      "cluster-cache-a100-workers",
		},
		{
			name:         "rejects sole incompatible node group",
			nodeSelector: map[string]string{gpuKey: a100},
			models: &v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
				Spec:       v1alpha1.LocalModelCacheSpec{SourceModelUri: storageURI, NodeGroups: []string{"h100-workers"}},
			}}},
		},
		{
			name:         "rejects cache when node group inventory is empty",
			nodeSelector: map[string]string{gpuKey: a100},
			nodeGroups:   &v1alpha1.LocalModelNodeGroupList{},
			models:       models,
		},
		{
			name:            "explicit node group takes precedence",
			nodeGroup:       "h100-workers",
			nodeGroupExists: true,
			nodeSelector:    map[string]string{gpuKey: a100},
			models:          models,
			wantPVC:         "cluster-cache-h100-workers",
		},
		{
			name:    "without node selector uses first node group",
			models:  models,
			wantPVC: "cluster-cache-h100-workers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			availableNodeGroups := nodeGroups
			if tt.nodeGroups != nil {
				availableNodeGroups = tt.nodeGroups
			}
			match := MatchCacheForURIWithNodeSelector(
				storageURI, tt.nodeGroup, tt.nodeGroupExists, tt.nodeSelector, availableNodeGroups, tt.models, nil,
			)
			if tt.wantPVC == "" {
				assert.Nil(t, match)
				return
			}
			assert.NotNil(t, match)
			assert.Equal(t, tt.wantPVC, match.PVCName)
		})
	}
}

func newNodeGroup(name, key, value string) v1alpha1.LocalModelNodeGroup {
	return v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{value},
						}},
					}},
				}},
			},
		},
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
