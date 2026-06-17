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
	assert.Equal(t, "my-cache", match.Name)
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
	assert.Equal(t, "ns-cache", match.Name)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "ns-cache-gpu2", match.PVCName)
}

func TestMarshalParseLoRACacheAnnotation(t *testing.T) {
	raw, err := MarshalLoRACacheAnnotation(map[string]LoRACacheEntry{
		"adapter-a": {
			Cache:     "adapter-cache",
			SourceURI: "hf://org/adapter",
			PVCName:   "adapter-cache-gpu1",
		},
	})
	assert.NoError(t, err)

	entries, err := ParseLoRACacheAnnotation(raw)
	assert.NoError(t, err)
	assert.Equal(t, "adapter-cache", entries["adapter-a"].Cache)
	assert.Equal(t, "hf://org/adapter", entries["adapter-a"].SourceURI)
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

func TestLLMISVCClusterCacheNames(t *testing.T) {
	baseOnly := LLMISVCClusterCacheNames(
		map[string]string{constants.LocalModelLabel: "base-cache"},
		nil,
	)
	assert.Equal(t, []string{"base-cache"}, baseOnly)

	loraOnly := LLMISVCClusterCacheNames(
		nil,
		map[string]string{
			constants.LocalModelLoRAAnnotationKey: `{"a":{"cache":"adapter-cache","sourceUri":"hf://x"}}`,
		},
	)
	assert.Equal(t, []string{"adapter-cache"}, loraOnly)

	nsBase := LLMISVCClusterCacheNames(
		map[string]string{
			constants.LocalModelLabel:          "ns-cache",
			constants.LocalModelNamespaceLabel: "default",
		},
		nil,
	)
	assert.Empty(t, nsBase)

	mixed := LLMISVCClusterCacheNames(
		map[string]string{constants.LocalModelLabel: "base-cache"},
		map[string]string{
			constants.LocalModelLoRAAnnotationKey: `{"a":{"cache":"adapter-cache","sourceUri":"hf://x"}}`,
		},
	)
	assert.Equal(t, []string{"adapter-cache", "base-cache"}, mixed)
}

func TestLLMISVCNamespaceCacheNames(t *testing.T) {
	names := LLMISVCNamespaceCacheNames(
		"default",
		map[string]string{
			constants.LocalModelLabel:          "ns-cache",
			constants.LocalModelNamespaceLabel: "default",
		},
		map[string]string{
			constants.LocalModelLoRAAnnotationKey: `{"a":{"cache":"adapter-ns-cache","namespace":"default","sourceUri":"hf://x"}}`,
		},
	)
	assert.Equal(t, []string{"adapter-ns-cache", "ns-cache"}, names)
}
