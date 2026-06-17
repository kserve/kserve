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
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// CacheMatch describes a LocalModelCache or LocalModelNamespaceCache hit for a storage URI.
type CacheMatch struct {
	Name      string
	Namespace string // set for LocalModelNamespaceCache; empty for cluster-scoped LocalModelCache
	SourceURI string
	PVCName   string
}

// LoRACacheEntry is one adapter's local model cache metadata stored in the LoRA JSON annotation.
type LoRACacheEntry struct {
	Cache     string `json:"cache"`
	Namespace string `json:"namespace,omitempty"`
	SourceURI string `json:"sourceUri"`
	PVCName   string `json:"pvcName"`
}

// MatchCacheForURI finds a namespace-scoped or cluster-scoped cache matching storageURI.
// LocalModelNamespaceCache takes precedence over LocalModelCache.
func MatchCacheForURI(
	storageURI string,
	nodeGroup string,
	nodeGroupExists bool,
	models *v1alpha1.LocalModelCacheList,
	nsModels *v1alpha1.LocalModelNamespaceCacheList,
) *CacheMatch {
	if storageURI == "" {
		return nil
	}

	if nsModels != nil {
		for i := range nsModels.Items {
			nsModel := &nsModels.Items[i]
			if !nsModel.Spec.MatchStorageURI(storageURI) {
				continue
			}
			pvcName, ok := pvcNameForNodeGroup(nsModel.Spec.NodeGroups, nodeGroup, nodeGroupExists, nsModel.Name)
			if !ok {
				continue
			}
			return &CacheMatch{
				Name:      nsModel.Name,
				Namespace: nsModel.Namespace,
				SourceURI: nsModel.Spec.SourceModelUri,
				PVCName:   pvcName,
			}
		}
	}

	if models == nil {
		return nil
	}
	for i := range models.Items {
		model := &models.Items[i]
		if !model.Spec.MatchStorageURI(storageURI) {
			continue
		}
		pvcName, ok := pvcNameForNodeGroup(model.Spec.NodeGroups, nodeGroup, nodeGroupExists, model.Name)
		if !ok {
			continue
		}
		return &CacheMatch{
			Name:      model.Name,
			SourceURI: model.Spec.SourceModelUri,
			PVCName:   pvcName,
		}
	}
	return nil
}

func pvcNameForNodeGroup(nodeGroups []string, nodeGroup string, nodeGroupExists bool, cacheName string) (string, bool) {
	if nodeGroupExists {
		if !slices.Contains(nodeGroups, nodeGroup) {
			return "", false
		}
		return cacheName + "-" + nodeGroup, true
	}
	if len(nodeGroups) == 0 {
		return "", false
	}
	return cacheName + "-" + nodeGroups[0], true
}

// BuildCachedPVCURI rewrites storageURI to a local model cache PVC path.
// subPath under sourceURI is preserved (e.g. hf://org/model/subdir).
func BuildCachedPVCURI(sourceURI, pvcName, storageURI string) string {
	subPath, _ := strings.CutPrefix(storageURI, sourceURI)
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	storageKey := v1alpha1.GetStorageKey(sourceURI)
	return "pvc://" + pvcName + "/models/" + storageKey + subPath
}

// MarshalLoRACacheAnnotation serializes adapter cache metadata to JSON.
func MarshalLoRACacheAnnotation(entries map[string]LoRACacheEntry) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal LoRA local model cache annotation: %w", err)
	}
	return string(data), nil
}

// ParseLoRACacheAnnotation deserializes adapter cache metadata from JSON.
func ParseLoRACacheAnnotation(raw string) (map[string]LoRACacheEntry, error) {
	if raw == "" {
		return nil, nil
	}
	var entries map[string]LoRACacheEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("parse LoRA local model cache annotation: %w", err)
	}
	return entries, nil
}

// LoRACacheEntryFromMatch builds a LoRACacheEntry from a cache match.
func LoRACacheEntryFromMatch(match *CacheMatch) LoRACacheEntry {
	if match == nil {
		return LoRACacheEntry{}
	}
	return LoRACacheEntry{
		Cache:     match.Name,
		Namespace: match.Namespace,
		SourceURI: match.SourceURI,
		PVCName:   match.PVCName,
	}
}

// LLMISVCClusterCacheNames returns cluster-scoped LocalModelCache names referenced by an
// LLMInferenceService via the base-model label or LoRA adapter annotation.
func LLMISVCClusterCacheNames(labels, annotations map[string]string) []string {
	var names []string
	if labels != nil {
		if name, ok := labels[constants.LocalModelLabel]; ok && name != "" {
			if _, nsOk := labels[constants.LocalModelNamespaceLabel]; !nsOk {
				names = append(names, name)
			}
		}
	}
	if entries, err := ParseLoRACacheAnnotation(annotations[constants.LocalModelLoRAAnnotationKey]); err == nil {
		for _, entry := range entries {
			if entry.Cache != "" && entry.Namespace == "" {
				names = append(names, entry.Cache)
			}
		}
	}
	return uniqueSorted(names)
}

// LLMISVCNamespaceCacheNames returns LocalModelNamespaceCache names in llmSvcNamespace
// referenced by an LLMInferenceService via the base-model labels or LoRA adapter annotation.
func LLMISVCNamespaceCacheNames(llmSvcNamespace string, labels, annotations map[string]string) []string {
	var names []string
	if labels != nil {
		name, hasModel := labels[constants.LocalModelLabel]
		modelNamespace, hasNamespace := labels[constants.LocalModelNamespaceLabel]
		if hasModel && hasNamespace && llmSvcNamespace == modelNamespace {
			names = append(names, name)
		}
	}
	if entries, err := ParseLoRACacheAnnotation(annotations[constants.LocalModelLoRAAnnotationKey]); err == nil {
		for _, entry := range entries {
			if entry.Cache != "" && entry.Namespace == llmSvcNamespace {
				names = append(names, entry.Cache)
			}
		}
	}
	return uniqueSorted(names)
}

// ClusterCacheNamesEqual reports whether two cluster cache name lists are equal (order ignored).
func ClusterCacheNamesEqual(a, b []string) bool {
	return slices.Equal(uniqueSorted(a), uniqueSorted(b))
}

// LLMISVCReferencesClusterCache reports whether labels/annotations reference a cluster-scoped cache.
func LLMISVCReferencesClusterCache(cacheName string, labels, annotations map[string]string) bool {
	return slices.Contains(LLMISVCClusterCacheNames(labels, annotations), cacheName)
}

// LLMISVCReferencesNamespaceCache reports whether labels/annotations reference a namespace-scoped cache.
func LLMISVCReferencesNamespaceCache(cacheName, cacheNamespace, llmSvcNamespace string, labels, annotations map[string]string) bool {
	if llmSvcNamespace != cacheNamespace {
		return false
	}
	return slices.Contains(LLMISVCNamespaceCacheNames(llmSvcNamespace, labels, annotations), cacheName)
}

func uniqueSorted(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	slices.Sort(names)
	return slices.Compact(names)
}
