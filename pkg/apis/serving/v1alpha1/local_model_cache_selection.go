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
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	// CacheRequired marks the workload as having opted into the
	// serving.kserve.io/local-model-cache-policy: CacheRequired policy (fail
	// closed). It is false for the default FallbackToSource workloads, which
	// never get routed to a not-ready shared-PVC cache and get released from a
	// bound cache as soon as its import record for the current claim is gone.
	CacheRequired bool
}

// SelectLocalModelNamespaceCache returns the namespace-scoped cache that should
// serve sel.StorageURI, or nil when no cache in nsModels matches. The boolean
// waitIntent return reports that the selected cache is a not-ready shared-PVC
// cache, which only happens for CacheRequired workloads (see below).
//
// Selection is deterministic and independent of the Kubernetes API list order:
//
//  1. A cache the workload is already bound to keeps winning while it still
//     matches the storage URI (and node group for node-local caches). For the
//     default (FallbackToSource) workload a bound shared-PVC cache keeps the
//     binding only while its import record still attests that usable data is
//     on the current claim (#6280): once the record is gone the claim holds
//     no usable data (it was recreated and a fresh import is underway), so the
//     binding is released and the workload falls back to its original source
//     URI. For a CacheRequired workload the binding is kept instead, and the
//     serving gates key on the import record (the workload waits, fail
//     closed, through ReimportBlocked and fresh imports alike).
//  2. Otherwise, ready shared-PVC caches win over node-local caches, which win
//     over not-ready shared-PVC caches. Within both classes the oldest cache
//     wins (creation timestamp, then name), mirroring the reconciler's
//     first-writer destination ownership.
//
// A not-ready shared-PVC cache is never selected for a default (FallbackToSource)
// workload. For a CacheRequired workload it is selected only as a fail-closed
// wait intent, reported by the waitIntent return value: the binding records
// the intent to serve from that cache, and admission and reconciliation gate
// on the cache's import record for the current claim instead of falling back
// to the workload's original source URI. Callers should still prefer any
// servable cluster-scoped LocalModelCache over a wait intent (see
// MatchCacheForURI).
func SelectLocalModelNamespaceCache(sel LocalModelCacheSelection, nsModels *LocalModelNamespaceCacheList) (selected *LocalModelNamespaceCache, waitIntent bool) {
	if nsModels == nil || sel.StorageURI == "" {
		return nil, false
	}

	var sharedReady, nodeLocal, sharedNotReady *LocalModelNamespaceCache
	for i := range nsModels.Items {
		candidate := &nsModels.Items[i]
		if !candidate.Spec.MatchStorageURI(sel.StorageURI) {
			continue
		}
		if isBoundLocalModelCache(sel, candidate) {
			if candidate.Spec.SharedPVCMode() {
				if sel.CacheRequired {
					// Fail closed: the binding is kept while it still matches,
					// including through a fresh import onto a recreated claim;
					// the serving gates decide on the import record instead.
					return candidate, false
				}
				// #6280 rule (default policy): a bound shared-PVC cache keeps
				// the binding while its import record still attests that the
				// data is on the current claim (not-ready with a record means
				// a spec-generation bump or a blocked re-import). Once the
				// record is cleared the claim holds no usable data yet, so the
				// binding is released.
				if candidate.IsReady() || candidate.Status.SharedPVCImport != nil {
					return candidate, false
				}
				continue
			}
			if NodeLocalCacheServesNodeGroup(candidate.Spec.NodeGroups, sel.NodeGroup, sel.NodeGroupExists) {
				return candidate, false
			}
			continue
		}
		if candidate.Spec.SharedPVCMode() {
			if candidate.IsReady() {
				sharedReady = OlderLocalModelCache(sharedReady, candidate)
			} else if sel.CacheRequired {
				sharedNotReady = OlderLocalModelCache(sharedNotReady, candidate)
			}
			continue
		}
		if !NodeLocalCacheServesNodeGroup(candidate.Spec.NodeGroups, sel.NodeGroup, sel.NodeGroupExists) {
			continue
		}
		nodeLocal = OlderLocalModelCache(nodeLocal, candidate)
	}
	if sharedReady != nil {
		return sharedReady, false
	}
	if nodeLocal != nil {
		return nodeLocal, false
	}
	return sharedNotReady, sharedNotReady != nil
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

// OlderLocalModelCache returns whichever of a and b is older by creation
// timestamp, then by name, so selection does not depend on list order. It works
// for both LocalModelCache and LocalModelNamespaceCache.
func OlderLocalModelCache[T any, PT interface {
	metav1.Object
	*T
}](a, b PT) PT {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	at, bt := a.GetCreationTimestamp(), b.GetCreationTimestamp()
	if !at.Equal(&bt) {
		if at.Time.Before(bt.Time) {
			return a
		}
		return b
	}
	if a.GetName() < b.GetName() {
		return a
	}
	return b
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
