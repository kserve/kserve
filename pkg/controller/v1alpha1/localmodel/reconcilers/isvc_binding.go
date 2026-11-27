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

package reconcilers

import (
	"context"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// bindWaitingInferenceServices re-runs the local model cache selection that
// the admission defaulter performs, for CacheRequired InferenceServices that
// are still waiting without a usable binding, or that carry a stale wait
// intent recorded when they were bound to a not-yet-Ready shared-PVC cache.
//
// A CacheRequired InferenceService created before its matching cache existed
// never received a binding from defaulting: the pod mutator holds its pods
// (local-model-cache-policy: CacheRequired, no cache bound), but nothing
// re-selects, so the service only recovers when its spec is edited again.
// Running the selection here binds the service as soon as a matching cache
// appears - the ISVC update re-triggers the workload controller, whose new pod
// template passes the pod mutator's gate - and clears a wait intent once the
// selection resolves to a servable cache, releasing the held pods.
//
// FallbackToSource services never wait for a cache, so they are skipped: only
// objects carrying the internal CacheRequired mirror (preserved through
// defaulting even when no cache matches) or a wait intent are visited.
//
// namespace scopes the ISVC list (the namespace-scoped cache reconciler);
// empty lists all namespaces (the cluster-scoped cache reconciler, whose
// node-local caches can serve any namespace).
func bindWaitingInferenceServices(ctx context.Context, c client.Client, namespace string) error {
	waiting := &v1beta1.InferenceServiceList{}
	listOpts := []client.ListOption{}
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if err := c.List(ctx, waiting, listOpts...); err != nil {
		return fmt.Errorf("list InferenceServices waiting for a local model cache: %w", err)
	}
	if len(waiting.Items) == 0 {
		return nil
	}

	models := &v1alpha1.LocalModelCacheList{}
	if err := c.List(ctx, models); err != nil {
		return fmt.Errorf("list local model caches: %w", err)
	}
	nsModelsByNamespace := map[string]*v1alpha1.LocalModelNamespaceCacheList{}
	for i := range waiting.Items {
		isvc := &waiting.Items[i]
		if _, done := nsModelsByNamespace[isvc.Namespace]; done {
			continue
		}
		nsModels := &v1alpha1.LocalModelNamespaceCacheList{}
		if err := c.List(ctx, nsModels, client.InNamespace(isvc.Namespace)); err != nil {
			return fmt.Errorf("list namespace-scoped local model caches in %s: %w", isvc.Namespace, err)
		}
		nsModelsByNamespace[isvc.Namespace] = nsModels
	}

	for i := range waiting.Items {
		isvc := &waiting.Items[i]
		if !isvc.DeletionTimestamp.IsZero() {
			continue
		}
		if _, localModelDisabled := isvc.Annotations[constants.DisableLocalModelKey]; localModelDisabled {
			continue
		}
		cacheRequired := isvc.Annotations[constants.LocalModelCachePolicyInternalAnnotationKey] == constants.LocalModelCachePolicyCacheRequired
		waitIntent := isvc.Annotations[constants.LocalModelCacheWaitIntentInternalAnnotationKey] == constants.LocalModelCacheWaitIntentValue
		if !cacheRequired && !waitIntent {
			continue
		}

		labelsBefore, annotationsBefore := cloneStringMap(isvc.Labels), cloneStringMap(isvc.Annotations)
		isvc.RefreshLocalModelCacheBinding(models, nsModelsByNamespace[isvc.Namespace])
		if reflect.DeepEqual(labelsBefore, isvc.Labels) && reflect.DeepEqual(annotationsBefore, isvc.Annotations) {
			// Still no matching cache (or an unchanged wait intent): keep
			// waiting for one.
			continue
		}

		// Patch only the metadata the selection changed, from a base whose
		// spec is absent so the merge patch cannot touch anything else.
		base := &v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{
			Namespace:   isvc.Namespace,
			Name:        isvc.Name,
			Labels:      labelsBefore,
			Annotations: annotationsBefore,
		}}
		target := base.DeepCopy()
		target.Labels = isvc.Labels
		target.Annotations = isvc.Annotations
		if err := c.Patch(ctx, target, client.MergeFrom(base)); err != nil {
			return fmt.Errorf("persist local model cache binding for InferenceService %s/%s: %w", isvc.Namespace, isvc.Name, err)
		}
	}
	return nil
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
