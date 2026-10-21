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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// serviceAccountImagePullSecretsChangedPredicate accepts ServiceAccount updates that change
// the image pull secrets, and creates and deletes of ServiceAccounts that have any.
func serviceAccountImagePullSecretsChangedPredicate() predicate.Funcs {
	hasImagePullSecrets := func(obj client.Object) bool {
		sa, ok := obj.(*corev1.ServiceAccount)
		return ok && len(sa.ImagePullSecrets) > 0
	}
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSA, ok := e.ObjectOld.(*corev1.ServiceAccount)
			if !ok {
				return false
			}
			newSA, ok := e.ObjectNew.(*corev1.ServiceAccount)
			if !ok {
				return false
			}
			return !equality.Semantic.DeepEqual(oldSA.ImagePullSecrets, newSA.ImagePullSecrets)
		},
		CreateFunc:  func(e event.CreateEvent) bool { return hasImagePullSecrets(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return hasImagePullSecrets(e.Object) },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// enqueueOnServiceAccountChange enqueues every LLMInferenceService in the ServiceAccount's namespace.
func (r *LLMISVCReconciler) enqueueOnServiceAccountChange(logger logr.Logger) handler.MapFunc {
	logger = logger.WithName("enqueueOnServiceAccountChange")
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		sa, ok := obj.(*corev1.ServiceAccount)
		if !ok || sa == nil {
			return nil
		}

		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: sa.Namespace}); err != nil {
			logger.Error(err, "failed to list LLMInferenceServices", "serviceAccount", sa.Name, "namespace", sa.Namespace)
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(llmSvcList.Items))
		for _, llmSvc := range llmSvcList.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: llmSvc.Namespace,
				Name:      llmSvc.Name,
			}})
		}
		return reqs
	}
}
