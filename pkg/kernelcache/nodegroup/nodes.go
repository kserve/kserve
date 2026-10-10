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

// Package nodegroup contains helpers that resolve KernelCacheNodeGroup selectors
// to concrete Kubernetes nodes. The package is deliberately small so that the
// selection contract can be reused by both admission and controller code.
package nodegroup

import (
	"context"
	"errors"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// MatchingGroups returns the usable groups whose selectors match the node.
func MatchingGroups(node *corev1.Node, groups []v1alpha1.KernelCacheNodeGroup) []v1alpha1.KernelCacheNodeGroup {
	if node == nil {
		return nil
	}
	matches := make([]v1alpha1.KernelCacheNodeGroup, 0, len(groups))
	for index := range groups {
		group := &groups[index]
		if !matchesLabels(node.Labels, group.Spec.NodeSelector) {
			continue
		}
		matches = append(matches, *group.DeepCopy())
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches
}

// ResolveKernelCacheNodeGroupName returns the explicit node group name or the configured default.
func ResolveKernelCacheNodeGroupName(kernelCache *v1alpha1.KernelCache, defaultNodeGroup string) string {
	if kernelCache != nil && kernelCache.Spec.NodeGroupRef != nil && kernelCache.Spec.NodeGroupRef.Name != "" {
		return kernelCache.Spec.NodeGroupRef.Name
	}
	return defaultNodeGroup
}

// GetNodes returns the ready and not-ready nodes that match the group selector.
// It never returns nodes that fail the selector, so callers can safely treat
// the union as the group membership.
func GetNodes(ctx context.Context, group *v1alpha1.KernelCacheNodeGroup, c client.Client) (*corev1.NodeList, *corev1.NodeList, error) {
	if group == nil {
		return nil, nil, errors.New("kernelcache node group is required")
	}
	selector := group.Spec.NodeSelector
	if len(selector) == 0 {
		return nil, nil, errors.New("kernelcache node group requires a non-empty nodeSelector")
	}

	nodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes); err != nil {
		return nil, nil, err
	}
	ready := &corev1.NodeList{}
	notReady := &corev1.NodeList{}
	for i := range nodes.Items {
		node := nodes.Items[i]
		if !matchesLabels(node.Labels, selector) {
			continue
		}
		if IsNodeReady(node) {
			ready.Items = append(ready.Items, node)
		} else {
			notReady.Items = append(notReady.Items, node)
		}
	}
	return ready, notReady, nil
}

// IsNodeReady returns true when the node reports the Ready condition as True.
func IsNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func matchesLabels(nodeLabels, selector map[string]string) bool {
	for key, value := range selector {
		nodeValue, exists := nodeLabels[key]
		if !exists || nodeValue != value {
			return false
		}
	}
	return true
}
