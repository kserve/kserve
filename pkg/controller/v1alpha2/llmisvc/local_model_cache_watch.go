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
)

// localModelCacheReadinessChangePredicate accepts only Update events whose
// generation or Ready condition changed: spec generation changes re-run the
// import, and Ready condition transitions unblock (or re-block) fail-closed
// workloads (see resolveBaseModelCachePVC and resolveLoRACachePVC). Creates and
// deletes are dropped: the reconcilers only match existing caches, and cache
// deletion is protected by the local model controller's consumer tracking.
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
		CreateFunc:  func(event.CreateEvent) bool { return false },
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
// changed local model cache (label internal.serving.kserve.io/localmodel) and
// enqueues them for reconciliation, so workloads waiting for a cache import
// (fail closed) resume as soon as the cache becomes Ready instead of on the
// next requeue backoff. LocalModelCache is cluster-scoped, so its consumers are
// scanned across every namespace.
func (r *LLMISVCReconciler) enqueueOnLocalModelCacheChange(logger logr.Logger) handler.MapFunc {
	logger = logger.WithName("enqueueOnLocalModelCacheChange")
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		if obj == nil {
			return nil
		}
		listOpts := []client.ListOption{
			client.MatchingLabels{constants.LocalModelLabel: obj.GetName()},
		}
		if nsCache, ok := obj.(*v1alpha1.LocalModelNamespaceCache); ok {
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
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: llmSvcList.Items[i].Namespace,
				Name:      llmSvcList.Items[i].Name,
			}})
		}
		return reqs
	}
}
