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

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// ClaimHoldsImportedModel reports whether the claim (PVC) that nsCache.Spec.PVCRef
// names in the cache namespace currently holds the cache's imported model: the PVC
// exists and the cache's import record names this exact claim (same UID). It is
// the serving gate for workloads already bound to a shared-PVC cache under the
// local-model-cache-policy: CacheRequired policy, keyed on the import record
// instead of the cache's Ready condition (which also folds in node-agent
// propagation and re-import state).
//
// A missing PVC or a record naming a different (recreated) claim means no usable
// data is on the current claim; a cache lookup failure is reported as an error so
// callers fail closed rather than silently serving from an unverified claim.
func ClaimHoldsImportedModel(ctx context.Context, c client.Client, nsCache *v1alpha1.LocalModelNamespaceCache) (bool, error) {
	if nsCache == nil || nsCache.Spec.PVCRef == nil || *nsCache.Spec.PVCRef == "" {
		return false, fmt.Errorf("namespace cache %s/%s has no PVC reference", nsCache.GetNamespace(), nsCache.GetName())
	}
	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: nsCache.GetNamespace(), Name: *nsCache.Spec.PVCRef}, pvc); err != nil {
		if apierrs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get PVC %s/%s for cache %s: %w", nsCache.GetNamespace(), *nsCache.Spec.PVCRef, nsCache.GetName(), err)
	}
	if pvc.DeletionTimestamp != nil {
		// Kubernetes refuses to schedule new pods onto a claim being deleted, so
		// a match here would wedge the workload behind the gate while the claim
		// drains. Existing pods keep reading the claim until it is gone.
		return false, nil
	}
	return nsCache.ImportedOntoClaim(pvc.UID), nil
}
