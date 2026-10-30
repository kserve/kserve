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

// CacheEntry is a LocalModelCache or LocalModelNamespaceCache reference.
// For the LoRA annotation, only Cache and Namespace are serialized. SourceURI and PVCName
// are runtime-only fields populated by MatchCacheForURI (and ignored by JSON marshal).
type CacheEntry struct {
	Cache     string `json:"cache"`
	Namespace string `json:"namespace,omitempty"` // set for LocalModelNamespaceCache; empty for cluster-scoped
	SourceURI string `json:"-"`
	PVCName   string `json:"-"`
}

// MatchCacheForURI finds a namespace-scoped or cluster-scoped cache for a workload.
// Namespace-scoped LocalModelNamespaceCache takes precedence over cluster-scoped LocalModelCache.
//
// Namespace-scoped selection is deterministic and binding-aware: see
// v1alpha1.SelectLocalModelNamespaceCache. A not-ready shared-PVC
// LocalModelNamespaceCache is only matched as a fail-closed wait intent: the
// binding records the intent to serve from that cache, and callers must wait for
// the cache to become Ready instead of falling back to the workload's original
// source URI. A servable cluster-scoped cache still wins over a wait intent, so
// workloads that a ready cluster cache can serve keep being served (including
// workloads bound to that cluster cache).
func MatchCacheForURI(
	sel v1alpha1.LocalModelCacheSelection,
	models *v1alpha1.LocalModelCacheList,
	nsModels *v1alpha1.LocalModelNamespaceCacheList,
) *CacheEntry {
	if sel.StorageURI == "" {
		return nil
	}

	// Namespace-scoped caches take precedence, except for the not-ready
	// shared-PVC wait intent: remember it and prefer any servable
	// cluster-scoped cache below before settling for the wait.
	var waitIntent *CacheEntry
	if nsModels != nil {
		if selected, wait := v1alpha1.SelectLocalModelNamespaceCache(sel, nsModels); selected != nil {
			if !wait {
				return namespaceScopedCacheEntry(selected, sel)
			}
			waitIntent = namespaceScopedCacheEntry(selected, sel)
		}
	}

	if models != nil {
		for i := range models.Items {
			model := &models.Items[i]
			if !model.Spec.MatchStorageURI(sel.StorageURI) {
				continue
			}
			pvcName, ok := PVCNameForNodeGroup(model.Spec.NodeGroups, sel.NodeGroup, sel.NodeGroupExists, model.Name)
			if !ok {
				continue
			}
			return &CacheEntry{
				Cache:     model.Name,
				SourceURI: model.Spec.SourceModelUri,
				PVCName:   pvcName,
			}
		}
	}
	return waitIntent
}

// namespaceScopedCacheEntry builds the CacheEntry for a selected namespace-scoped
// cache. Node-local caches are only selected when they serve the workload's node
// group, so the PVC name derivation cannot fail.
func namespaceScopedCacheEntry(selected *v1alpha1.LocalModelNamespaceCache, sel v1alpha1.LocalModelCacheSelection) *CacheEntry {
	entry := &CacheEntry{
		Cache:     selected.Name,
		Namespace: selected.Namespace,
		SourceURI: selected.Spec.SourceModelUri,
	}
	if selected.Spec.SharedPVCMode() {
		entry.PVCName = *selected.Spec.PVCRef
		return entry
	}
	entry.PVCName = nodeLocalPVCName(selected.Spec.NodeGroups, sel.NodeGroup, sel.NodeGroupExists, selected.Name)
	return entry
}

// PVCNameForNodeGroup returns the serving PVC name for a cache and node group selection.
func PVCNameForNodeGroup(nodeGroups []string, nodeGroup string, nodeGroupExists bool, cacheName string) (string, bool) {
	if !v1alpha1.NodeLocalCacheServesNodeGroup(nodeGroups, nodeGroup, nodeGroupExists) {
		return "", false
	}
	return nodeLocalPVCName(nodeGroups, nodeGroup, nodeGroupExists, cacheName), true
}

// nodeLocalPVCName derives the serving PVC name for a node-local cache that is
// known to serve the workload's node group (see
// v1alpha1.NodeLocalCacheServesNodeGroup).
func nodeLocalPVCName(nodeGroups []string, nodeGroup string, nodeGroupExists bool, cacheName string) string {
	if nodeGroupExists {
		return cacheName + "-" + nodeGroup
	}
	return cacheName + "-" + nodeGroups[0]
}

// BuildCachedPVCURI rewrites storageURI to a local model cache PVC path.
// subPath under sourceURI is preserved (e.g. hf://org/model/subdir).
// Trailing slashes are trimmed only for CutPrefix so subdirectory matching works;
// GetStorageKey uses sourceURI as stored so the hash matches the download job folder.
func BuildCachedPVCURI(sourceURI, pvcName, storageURI string) string {
	subPath, _ := strings.CutPrefix(storageURI, strings.TrimSuffix(sourceURI, "/"))
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	storageKey := v1alpha1.GetStorageKey(sourceURI)
	return "pvc://" + pvcName + "/models/" + storageKey + subPath
}

// MarshalLoRACacheAnnotation serializes adapter cache refs (cache + optional namespace) to JSON.
func MarshalLoRACacheAnnotation(entries map[string]CacheEntry) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal LoRA local model cache annotation: %w", err)
	}
	return string(data), nil
}

// ParseLoRACacheAnnotation deserializes adapter cache refs from JSON.
func ParseLoRACacheAnnotation(raw string) (map[string]CacheEntry, error) {
	if raw == "" {
		return nil, nil
	}
	var entries map[string]CacheEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("parse LoRA local model cache annotation: %w", err)
	}
	return entries, nil
}

func loRACacheAnnotationRaw(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations[constants.LocalModelLoRAAnnotationKey]
}

// LLMISVCClusterCacheNames returns cluster-scoped LocalModelCache names referenced by an
// LLMInferenceService via the base-model label or LoRA adapter annotation.
// Malformed LoRA annotation JSON is ignored here; LLMISVCReferencesClusterCache fails closed instead.
func LLMISVCClusterCacheNames(labels, annotations map[string]string) []string {
	var names []string
	if labels != nil {
		if name, ok := labels[constants.LocalModelLabel]; ok && name != "" {
			if _, nsOk := labels[constants.LocalModelNamespaceLabel]; !nsOk {
				names = append(names, name)
			}
		}
	}
	if entries, err := ParseLoRACacheAnnotation(loRACacheAnnotationRaw(annotations)); err == nil {
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
// Malformed LoRA annotation JSON is ignored here; LLMISVCReferencesNamespaceCache fails closed instead.
func LLMISVCNamespaceCacheNames(llmSvcNamespace string, labels, annotations map[string]string) []string {
	var names []string
	if labels != nil {
		name, hasModel := labels[constants.LocalModelLabel]
		modelNamespace, hasNamespace := labels[constants.LocalModelNamespaceLabel]
		if hasModel && hasNamespace && llmSvcNamespace == modelNamespace {
			names = append(names, name)
		}
	}
	if entries, err := ParseLoRACacheAnnotation(loRACacheAnnotationRaw(annotations)); err == nil {
		for _, entry := range entries {
			if entry.Cache != "" && entry.Namespace == llmSvcNamespace {
				names = append(names, entry.Cache)
			}
		}
	}
	return uniqueSorted(names)
}

// CacheNamesEqual reports whether two cache name lists are equal (order ignored).
func CacheNamesEqual(a, b []string) bool {
	return slices.Equal(uniqueSorted(a), uniqueSorted(b))
}

// LLMISVCReferencesClusterCache reports whether labels/annotations reference a cluster-scoped cache.
// A non-empty but malformed LoRA annotation is treated as a reference (fail-closed) so cache
// deletion stays blocked until the annotation is fixed or removed.
func LLMISVCReferencesClusterCache(cacheName string, labels, annotations map[string]string) bool {
	if slices.Contains(LLMISVCClusterCacheNames(labels, annotations), cacheName) {
		return true
	}
	raw := loRACacheAnnotationRaw(annotations)
	if raw == "" {
		return false
	}
	if _, err := ParseLoRACacheAnnotation(raw); err != nil {
		return true
	}
	return false
}

// LLMISVCReferencesNamespaceCache reports whether labels/annotations reference a namespace-scoped cache.
// A non-empty but malformed LoRA annotation is treated as a reference (fail-closed) so cache
// deletion stays blocked until the annotation is fixed or removed.
func LLMISVCReferencesNamespaceCache(cacheName, cacheNamespace, llmSvcNamespace string, labels, annotations map[string]string) bool {
	if llmSvcNamespace != cacheNamespace {
		return false
	}
	if slices.Contains(LLMISVCNamespaceCacheNames(llmSvcNamespace, labels, annotations), cacheName) {
		return true
	}
	raw := loRACacheAnnotationRaw(annotations)
	if raw == "" {
		return false
	}
	if _, err := ParseLoRACacheAnnotation(raw); err != nil {
		return true
	}
	return false
}

func uniqueSorted(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	names = slices.Clone(names)
	slices.Sort(names)
	return slices.Compact(names)
}
