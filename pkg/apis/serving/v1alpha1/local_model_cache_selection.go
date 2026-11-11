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

import "slices"

// LocalModelCacheSelection carries the inputs for selecting the local model
// cache that should serve a workload.
type LocalModelCacheSelection struct {
	// StorageURI is the workload's model storage URI.
	StorageURI string
	// NodeGroup and NodeGroupExists carry the workload's node group annotation
	// (serving.kserve.io/nodegroup). NodeGroup is ignored when NodeGroupExists
	// is false.
	NodeGroup       string
	NodeGroupExists bool
	// BoundCache and BoundNamespace carry the workload's current cache binding:
	// the internal.serving.kserve.io/localmodel and
	// internal.serving.kserve.io/localmodel-namespace label values. Both are
	// empty when the workload is not bound to a cache, and BoundNamespace is
	// empty when the binding points at a cluster-scoped LocalModelCache.
	BoundCache     string
	BoundNamespace string
}

// SelectLocalModelNamespaceCache returns the namespace-scoped cache that should
// serve sel.StorageURI, or nil when no cache in nsModels matches.
//
// Selection is deterministic and independent of the Kubernetes API list order:
//
//  1. A cache the workload is already bound to keeps winning while it still
//     matches the storage URI (and node group for node-local caches) and,
//     for shared-PVC caches, its import record still attests that usable data
//     is on the current claim: dropping the binding on an unrelated update
//     would roll the workload's pods back to downloading from source. Once the
//     record is gone the claim holds no usable data (it was recreated and a
//     fresh import is underway), so the binding is released like any other
//     not-ready match.
//  2. Otherwise, ready shared-PVC caches win over node-local caches. Within
//     both classes the oldest cache wins (creation timestamp, then name),
//     mirroring the reconciler's first-writer destination ownership.
//
// An unbound workload is never routed to a not-ready shared-PVC cache.
func SelectLocalModelNamespaceCache(sel LocalModelCacheSelection, nsModels *LocalModelNamespaceCacheList) *LocalModelNamespaceCache {
	if nsModels == nil || sel.StorageURI == "" {
		return nil
	}

	var sharedReady, nodeLocal *LocalModelNamespaceCache
	for i := range nsModels.Items {
		candidate := &nsModels.Items[i]
		if !candidate.Spec.MatchStorageURI(sel.StorageURI) {
			continue
		}
		if isBoundLocalModelCache(sel, candidate) {
			if candidate.Spec.SharedPVCMode() {
				// #6280 rule: a bound shared-PVC cache keeps the binding while
				// its import record still attests that the data is on the current
				// claim (not-ready with a record means a spec-generation bump or a
				// blocked re-import). Once the record is cleared the claim holds no
				// usable data yet, so the binding is released.
				if candidate.IsReady() || candidate.Status.SharedPVCImport != nil {
					return candidate
				}
				continue
			}
			if NodeLocalCacheServesNodeGroup(candidate.Spec.NodeGroups, sel.NodeGroup, sel.NodeGroupExists) {
				return candidate
			}
			continue
		}
		if candidate.Spec.SharedPVCMode() {
			if candidate.IsReady() {
				sharedReady = olderLocalModelCache(sharedReady, candidate)
			}
			continue
		}
		if !NodeLocalCacheServesNodeGroup(candidate.Spec.NodeGroups, sel.NodeGroup, sel.NodeGroupExists) {
			continue
		}
		nodeLocal = olderLocalModelCache(nodeLocal, candidate)
	}
	if sharedReady != nil {
		return sharedReady
	}
	return nodeLocal
}

// NodeLocalCacheServesNodeGroup reports whether a node-local cache with the
// given node groups can serve a workload with the given node group annotation.
// A workload without the annotation can be served from any node-local cache.
func NodeLocalCacheServesNodeGroup(nodeGroups []string, nodeGroup string, nodeGroupExists bool) bool {
	if nodeGroupExists {
		return slices.Contains(nodeGroups, nodeGroup)
	}
	return len(nodeGroups) > 0
}

// isBoundLocalModelCache reports whether candidate is the cache sel is bound to.
// An empty binding never matches: cache names and namespaces are never empty.
func isBoundLocalModelCache(sel LocalModelCacheSelection, candidate *LocalModelNamespaceCache) bool {
	return sel.BoundCache != "" && sel.BoundCache == candidate.Name && sel.BoundNamespace == candidate.Namespace
}

// olderLocalModelCache returns whichever of a and b is older by creation
// timestamp, then by name, so selection does not depend on list order.
func olderLocalModelCache(a, b *LocalModelNamespaceCache) *LocalModelNamespaceCache {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		if a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time) {
			return a
		}
		return b
	}
	if a.Name < b.Name {
		return a
	}
	return b
}
