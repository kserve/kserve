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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// StorageReservation describes the storage requested by a local model cache.
type StorageReservation struct {
	Name       string
	Namespace  string
	ModelSize  resource.Quantity
	NodeGroups []string
}

// ValidateStorageCapacity validates that the requested storage fits within the
// storage limit of every targeted LocalModelNodeGroup. Existing reservations
// from both LocalModelCache and LocalModelNamespaceCache are included.
func ValidateStorageCapacity(
	ctx context.Context,
	c client.Client,
	current StorageReservation,
) error {
	localModelCaches := &v1alpha1.LocalModelCacheList{}
	if err := c.List(ctx, localModelCaches); err != nil {
		return fmt.Errorf("unable to list LocalModelCaches: %w", err)
	}

	localModelNamespaceCaches := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := c.List(ctx, localModelNamespaceCaches); err != nil {
		return fmt.Errorf("unable to list LocalModelNamespaceCaches: %w", err)
	}

	for _, nodeGroupName := range current.NodeGroups {
		nodeGroup := &v1alpha1.LocalModelNodeGroup{}
		if err := c.Get(ctx, client.ObjectKey{Name: nodeGroupName}, nodeGroup); err != nil {
			return fmt.Errorf("unable to get LocalModelNodeGroup %s: %w", nodeGroupName, err)
		}

		var totalModelSize resource.Quantity
		totalModelSize.Add(current.ModelSize)

		for _, cache := range localModelCaches.Items {
			if cache.Name == current.Name && current.Namespace == "" {
				continue
			}

			if containsNodeGroup(cache.Spec.NodeGroups, nodeGroupName) {
				totalModelSize.Add(cache.Spec.ModelSize)
			}
		}

		for _, cache := range localModelNamespaceCaches.Items {
			if cache.Name == current.Name && cache.Namespace == current.Namespace {
				continue
			}

			if containsNodeGroup(cache.Spec.NodeGroups, nodeGroupName) {
				totalModelSize.Add(cache.Spec.ModelSize)
			}
		}

		if totalModelSize.Cmp(nodeGroup.Spec.StorageLimit) > 0 {
			return fmt.Errorf(
				"local model cache %s would use %s of %s storage in LocalModelNodeGroup %s, exceeding the storage limit of %s",
				current.Name,
				totalModelSize.String(),
				nodeGroup.Spec.StorageLimit.String(),
				nodeGroupName,
				nodeGroup.Spec.StorageLimit.String(),
			)
		}
	}

	return nil
}

func containsNodeGroup(nodeGroups []string, nodeGroupName string) bool {
	for _, name := range nodeGroups {
		if name == nodeGroupName {
			return true
		}
	}
	return false
}
