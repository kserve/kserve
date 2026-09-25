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

package kernelcache

import (
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	kernelcacheutil "github.com/kserve/kserve/pkg/kernelcache"
)

type kernelCacheMatch string

const (
	kernelCacheMatchWorkload      kernelCacheMatch = "Workload"
	kernelCacheMatchCompatibility kernelCacheMatch = "Compatibility"
)

type kernelCacheSelection struct {
	cache     *v1alpha1.KernelCache
	matchType kernelCacheMatch
}

type kernelCacheNodeCandidate struct {
	ref   v1alpha1.NamespacedName
	infos []v1alpha1.KernelCacheNodeCacheInfo
}

func readyKernelCacheCandidates(nodes *v1alpha1.KernelCacheNodeList, namespace string) []kernelCacheNodeCandidate {
	byRef := make(map[string]kernelCacheNodeCandidate)
	for nodeIndex := range nodes.Items {
		node := &nodes.Items[nodeIndex]
		for _, info := range node.Status.CacheStatus {
			if info.State != v1alpha1.KernelCacheNodePreparationStateReady ||
				info.KernelCacheRef.Namespace != namespace ||
				info.KernelCacheRef.Name == "" {
				continue
			}

			key := info.KernelCacheRef.Namespace + "/" + info.KernelCacheRef.Name
			candidate := byRef[key]
			candidate.ref = info.KernelCacheRef
			candidate.infos = append(candidate.infos, info)
			byRef[key] = candidate
		}
	}

	candidates := make([]kernelCacheNodeCandidate, 0, len(byRef))
	for _, candidate := range byRef {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ref.Namespace != candidates[j].ref.Namespace {
			return candidates[i].ref.Namespace < candidates[j].ref.Namespace
		}
		return candidates[i].ref.Name < candidates[j].ref.Name
	})
	return candidates
}

func candidateMatchesWorkload(requested v1alpha1.KernelCacheIdentity, candidate kernelCacheNodeCandidate) bool {
	if requested.Footprints.WorkloadFootprint != "" {
		for _, info := range candidate.infos {
			if info.Footprints.WorkloadFootprint == requested.Footprints.WorkloadFootprint &&
				compatibilityFootprintsMatch(requested.Footprints, info.Footprints) {
				return true
			}
		}
	}
	return false
}

func candidateMatchesCompatibility(requested v1alpha1.KernelCacheIdentity, candidate kernelCacheNodeCandidate) bool {
	if requested.Footprints.CompatibilityFootprint != "" {
		for _, info := range candidate.infos {
			if info.Footprints.CompatibilityFootprint == requested.Footprints.CompatibilityFootprint {
				return true
			}
		}
	}
	return false
}

func candidateMatchesKernelCache(candidate kernelCacheNodeCandidate, cache *v1alpha1.KernelCache) bool {
	if cache.Namespace != candidate.ref.Namespace || cache.Name != candidate.ref.Name {
		return false
	}
	for _, info := range candidate.infos {
		if info.ImageReference == cache.Spec.Artifact.ImageReference && info.Footprints == cache.Spec.Artifact.Identity.Footprints {
			return true
		}
	}
	return false
}

func compatibilityFootprintsMatch(requested, candidate v1alpha1.KernelCacheFootprints) bool {
	return requested.CompatibilityFootprint != "" &&
		requested.CompatibilityFootprint == candidate.CompatibilityFootprint
}

func cacheCanMountToPod(cache *v1alpha1.KernelCache, pod *corev1.Pod) bool {
	if cache.Namespace != pod.Namespace || cache.Spec.Artifact.ImageReference == "" || len(cache.Spec.Artifact.CachePaths) == 0 {
		return false
	}
	if cache.Spec.MountType != "" && cache.Spec.MountType != v1alpha1.KernelCacheMountTypeOCI {
		return false
	}
	for _, cachePath := range cache.Spec.Artifact.CachePaths {
		containerName, err := kernelcacheutil.ResolveRuntimeContainerName(pod.Spec.Containers, cachePath.ContainerName)
		if err != nil {
			return false
		}
		containerIndex := findContainerIndex(pod.Spec.Containers, containerName)
		if containerIndex < 0 {
			return false
		}
		if _, err := kernelcacheutil.ResolveContainerPath(&pod.Spec.Containers[containerIndex], cachePath.ContainerPath); err != nil {
			return false
		}
		if _, err := kernelcacheutil.ResolveOCIPath(cachePath.OCIPath); err != nil {
			return false
		}
	}
	return true
}
