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

package llmisvc

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
	kservevalidation "github.com/kserve/kserve/pkg/validation"
)

// localModelCacheReadinessChangePredicate accepts only Update events whose
// generation or Ready condition changed: spec generation changes re-run the
// import, and Ready condition transitions unblock (or re-block) fail-closed
// workloads (see resolveBaseModelCachePVC and resolveLoRACachePVC). Creates
// are accepted too: a newly created cache may be the first match for a
// CacheRequired workload that has been waiting unbound (see
// ensureLocalModelCacheBinding). Deletes are dropped: the reconcilers only
// match existing caches, and cache deletion is protected by the local model
// controller's consumer tracking.
func localModelCacheReadinessChangePredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldReady := localModelCacheReadyCondition(e.ObjectOld)
			newReady := localModelCacheReadyCondition(e.ObjectNew)
			if oldReady == nil || newReady == nil {
				return true
			}
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() ||
				oldReady.Status != newReady.Status ||
				oldReady.ObservedGeneration != newReady.ObservedGeneration
		},
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func localModelCacheReadyCondition(obj client.Object) *metav1.Condition {
	switch typed := obj.(type) {
	case *v1alpha1.LocalModelCache:
		return typed.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	case *v1alpha1.LocalModelNamespaceCache:
		return typed.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	}
	return nil
}

// enqueueOnLocalModelCacheChange lists the LLMInferenceServices bound to the
// changed local model cache and enqueues them for reconciliation, so workloads
// waiting for a cache import (fail closed) resume as soon as the cache becomes
// Ready instead of on the next requeue backoff. A workload is bound to a cache
// through the internal.serving.kserve.io/localmodel label (base model) or
// through a LoRA adapter entry of the localmodel-lora annotation, so workloads
// whose base model is uncached but an adapter is cached are covered too.
// CacheRequired workloads that are still unbound but would select the changed
// cache for their base model or an adapter are enqueued as well: their
// reconciliation re-runs the selection (ensureLocalModelCacheBinding) and
// persists the binding instead of waiting for the requeue backoff.
// LocalModelCache is cluster-scoped, so its consumers are scanned across every
// namespace.
func (r *LLMISVCReconciler) enqueueOnLocalModelCacheChange(logger logr.Logger) handler.MapFunc {
	logger = logger.WithName("enqueueOnLocalModelCacheChange")
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		if obj == nil {
			return nil
		}
		var listOpts []client.ListOption
		nsCache, isNamespaceCache := obj.(*v1alpha1.LocalModelNamespaceCache)
		if isNamespaceCache {
			listOpts = append(listOpts, client.InNamespace(nsCache.Namespace))
		}
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, listOpts...); err != nil {
			logger.Error(err, "failed to list LLMInferenceServices bound to local model cache", "cache", types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			})
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(llmSvcList.Items))
		for i := range llmSvcList.Items {
			llmSvc := &llmSvcList.Items[i]
			var bound bool
			if isNamespaceCache {
				bound = localmodelcache.LLMISVCReferencesNamespaceCache(nsCache.Name, nsCache.Namespace, llmSvc.Namespace, llmSvc.Labels, llmSvc.Annotations)
			} else {
				bound = localmodelcache.LLMISVCReferencesClusterCache(obj.GetName(), llmSvc.Labels, llmSvc.Annotations)
			}
			if !bound && localModelCacheCouldBind(obj, llmSvc) {
				bound = true
			}
			if bound {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: llmSvc.Namespace,
					Name:      llmSvc.Name,
				}})
			}
		}
		return reqs
	}
}

// localModelCacheCouldBind reports whether the changed cache would be selected
// for the base model or a LoRA adapter of an unbound CacheRequired workload.
// The selection mirrors MatchCacheForURI with CacheRequired and no prior
// binding: a not-ready shared-PVC cache still matches as a wait intent, so the
// workload binds and waits on the import record rather than staying unbound.
func localModelCacheCouldBind(obj client.Object, llmSvc *v1alpha2.LLMInferenceService) bool {
	cacheRequired, _ := kservevalidation.LocalModelCachePolicyRequired(llmSvc.Annotations)
	if !cacheRequired {
		return false
	}
	// Fully bound workloads re-select nothing; everything else may pick up
	// this cache for its base model or an unbound LoRA adapter.
	if localModelCacheFullyBound(llmSvc) {
		return false
	}

	models := &v1alpha1.LocalModelCacheList{}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{}
	switch cache := obj.(type) {
	case *v1alpha1.LocalModelCache:
		models.Items = []v1alpha1.LocalModelCache{*cache}
	case *v1alpha1.LocalModelNamespaceCache:
		if cache.Namespace != llmSvc.Namespace {
			return false
		}
		nsModels.Items = []v1alpha1.LocalModelNamespaceCache{*cache}
	default:
		return false
	}

	nodeGroup, nodeGroupExists := llmSvc.Annotations[constants.NodeGroupAnnotationKey]
	if localmodelcache.MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
		StorageURI:      llmSvc.Spec.Model.URI.String(),
		NodeGroup:       nodeGroup,
		NodeGroupExists: nodeGroupExists,
		CacheRequired:   true,
	}, models, nsModels) != nil {
		return true
	}
	if llmSvc.Spec.Model.LoRA == nil {
		return false
	}
	for _, adapter := range llmSvc.Spec.Model.LoRA.Adapters {
		if adapter.Name == nil {
			continue
		}
		if localmodelcache.MatchCacheForURI(v1alpha1.LocalModelCacheSelection{
			StorageURI:      adapter.URI.String(),
			NodeGroup:       nodeGroup,
			NodeGroupExists: nodeGroupExists,
			CacheRequired:   true,
		}, models, nsModels) != nil {
			return true
		}
	}
	return false
}
