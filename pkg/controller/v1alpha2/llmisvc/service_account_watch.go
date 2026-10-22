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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
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

// enqueueOnServiceAccountChange enqueues, for a ServiceAccount the controller manages, its owning
// LLMInferenceService and every LLMInferenceService whose tokenizer runs as that ServiceAccount.
// For any other ServiceAccount it enqueues every LLMInferenceService in the namespace.
func (r *LLMISVCReconciler) enqueueOnServiceAccountChange(logger logr.Logger) handler.MapFunc {
	logger = logger.WithName("enqueueOnServiceAccountChange")
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		sa, ok := obj.(*corev1.ServiceAccount)
		if !ok || sa == nil {
			return nil
		}

		if ownerRef := llmSvcControllerOf(sa); ownerRef != nil {
			reqs := []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: sa.Namespace, Name: ownerRef.Name}}}

			tokenizers := &appsv1.DeploymentList{}
			if err := r.List(ctx, tokenizers, client.InNamespace(sa.Namespace), client.MatchingLabels{
				constants.KubernetesComponentLabelKey: constants.LLMComponentTokenizer,
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
			}); err != nil {
				logger.Error(err, "failed to list tokenizer Deployments", "serviceAccount", sa.Name, "namespace", sa.Namespace)
				return reqs
			}
			for i := range tokenizers.Items {
				tokenizer := &tokenizers.Items[i]
				if tokenizer.Spec.Template.Spec.ServiceAccountName != sa.Name {
					continue
				}
				if tokenizerOwnerRef := llmSvcControllerOf(tokenizer); tokenizerOwnerRef != nil && tokenizerOwnerRef.Name != ownerRef.Name {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: sa.Namespace, Name: tokenizerOwnerRef.Name}})
				}
			}
			return reqs
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

// llmSvcControllerOf returns the controller reference of obj when it points to an LLMInferenceService.
func llmSvcControllerOf(obj metav1.Object) *metav1.OwnerReference {
	ownerRef := metav1.GetControllerOf(obj)
	if ownerRef == nil ||
		ownerRef.Kind != v1alpha2.LLMInferenceServiceGVK.Kind ||
		ownerRef.APIVersion != v1alpha2.LLMInferenceServiceGVK.GroupVersion().String() {
		return nil
	}
	return ownerRef
}
