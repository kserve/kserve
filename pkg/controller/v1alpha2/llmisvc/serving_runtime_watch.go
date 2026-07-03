/*
Copyright 2025 The KServe Authors.

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
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kserveapiv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// servingRuntimeSpecChangedPredicate accepts only Update events whose Spec has
// changed. Creates and deletes are dropped: a new runtime cannot retroactively
// affect existing services, and deletions fall through silently in the merge
// chain (see resolveRuntimeSpec).
func servingRuntimeSpecChangedPredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			switch oldObj := e.ObjectOld.(type) {
			case *kserveapiv1alpha1.ClusterServingRuntime:
				newObj, ok := e.ObjectNew.(*kserveapiv1alpha1.ClusterServingRuntime)
				if !ok {
					return false
				}
				return !reflect.DeepEqual(oldObj.Spec, newObj.Spec)
			case *kserveapiv1alpha1.ServingRuntime:
				newObj, ok := e.ObjectNew.(*kserveapiv1alpha1.ServingRuntime)
				if !ok {
					return false
				}
				return !reflect.DeepEqual(oldObj.Spec, newObj.Spec)
			}
			return false
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// enqueueOnClusterServingRuntimeChange lists all LLMInferenceServices whose
// spec.runtime matches the changed ClusterServingRuntime and enqueues them for
// reconciliation. Because ClusterServingRuntime is cluster-scoped, we scan every
// namespace — matches are name-only.
func (r *LLMISVCReconciler) enqueueOnClusterServingRuntimeChange(logger logr.Logger) handler.MapFunc {
	logger = logger.WithName("enqueueOnClusterServingRuntimeChange")
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		csr, ok := obj.(*kserveapiv1alpha1.ClusterServingRuntime)
		if !ok || csr == nil {
			return nil
		}
		return r.matchingLLMInferenceServiceRequests(ctx, logger, csr.Name, corev1.NamespaceAll)
	}
}

// enqueueOnServingRuntimeChange lists LLMInferenceServices in the ServingRuntime's
// own namespace whose spec.runtime matches the changed runtime.
func (r *LLMISVCReconciler) enqueueOnServingRuntimeChange(logger logr.Logger) handler.MapFunc {
	logger = logger.WithName("enqueueOnServingRuntimeChange")
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		sr, ok := obj.(*kserveapiv1alpha1.ServingRuntime)
		if !ok || sr == nil {
			return nil
		}
		return r.matchingLLMInferenceServiceRequests(ctx, logger, sr.Name, sr.Namespace)
	}
}

// matchingLLMInferenceServiceRequests returns reconcile requests for every
// LLMInferenceService in the given namespace whose spec.runtime equals name.
// Pass corev1.NamespaceAll to scan the whole cluster (used for
// ClusterServingRuntime, which is cluster-scoped).
func (r *LLMISVCReconciler) matchingLLMInferenceServiceRequests(ctx context.Context, logger logr.Logger, name, namespace string) []reconcile.Request {
	llmSvcList := &v1alpha2.LLMInferenceServiceList{}
	if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: namespace}); err != nil {
		logger.Error(err, "failed to list LLMInferenceServices", "runtime", name, "namespace", namespace)
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(llmSvcList.Items))
	for _, llmSvc := range llmSvcList.Items {
		if llmSvc.Spec.Runtime == nil || *llmSvc.Spec.Runtime != name {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: llmSvc.Namespace,
			Name:      llmSvc.Name,
		}})
	}
	return reqs
}
