/*
Copyright 2024 The KServe Authors.

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

package localmodelnamespacecache

import (
	"context"
	"errors"
	"fmt"

	"github.com/kserve/kserve/pkg/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apivalidation "k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
)

// logger for the validation webhook.
var localModelNamespaceCacheValidatorLogger = logf.Log.WithName("localmodelnamespacecache-v1alpha1-validation-webhook")

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false
// LocalModelNamespaceCacheValidator is responsible for validating the LocalModelNamespaceCache resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type LocalModelNamespaceCacheValidator struct {
	client.Client
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-localmodelnamespacecaches,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=localmodelnamespacecaches,versions=v1alpha1,name=localmodelnamespacecache.kserve-webhook-server.validator
var _ webhook.CustomValidator = &LocalModelNamespaceCacheValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *LocalModelNamespaceCacheValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	localModelNamespaceCache, err := utils.Convert[*v1alpha1.LocalModelNamespaceCache](obj)
	if err != nil {
		localModelNamespaceCacheValidatorLogger.Error(err, "Unable to convert object to LocalModelNamespaceCache")
		return nil, err
	}
	localModelNamespaceCacheValidatorLogger.Info("validate create", "name", localModelNamespaceCache.Name, "namespace", localModelNamespaceCache.Namespace)

	if err := validateStorageMode(localModelNamespaceCache); err != nil {
		return nil, err
	}

	if localModelNamespaceCache.Spec.SharedPVCMode() {
		if err := v.validateDestinationConflict(ctx, localModelNamespaceCache); err != nil {
			return nil, err
		}
		return nil, nil
	}

	if err := v.validateNodeGroups(ctx, localModelNamespaceCache); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *LocalModelNamespaceCacheValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	localModelNamespaceCache, err := utils.Convert[*v1alpha1.LocalModelNamespaceCache](newObj)
	if err != nil {
		localModelNamespaceCacheValidatorLogger.Error(err, "Unable to convert object to LocalModelNamespaceCache")
		return nil, err
	}
	if localModelNamespaceCache.GetDeletionTimestamp() != nil {
		return nil, nil
	}
	localModelNamespaceCacheValidatorLogger.Info("validate update", "name", localModelNamespaceCache.Name, "namespace", localModelNamespaceCache.Namespace)

	oldCache, err := utils.Convert[*v1alpha1.LocalModelNamespaceCache](oldObj)
	if err != nil {
		localModelNamespaceCacheValidatorLogger.Error(err, "Unable to convert old object to LocalModelNamespaceCache")
		return nil, err
	}

	if err := validateStorageMode(localModelNamespaceCache); err != nil {
		return nil, err
	}
	if err := validatePVCRefImmutable(oldCache, localModelNamespaceCache); err != nil {
		return nil, err
	}

	if localModelNamespaceCache.Spec.SharedPVCMode() {
		if err := v.validateDestinationConflict(ctx, localModelNamespaceCache); err != nil {
			return nil, err
		}
		return nil, nil
	}

	if err := v.validateNodeGroups(ctx, localModelNamespaceCache); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *LocalModelNamespaceCacheValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	localModelNamespaceCache, err := utils.Convert[*v1alpha1.LocalModelNamespaceCache](obj)
	if err != nil {
		localModelNamespaceCacheValidatorLogger.Error(err, "Unable to convert object to LocalModelNamespaceCache")
		return nil, err
	}
	localModelNamespaceCacheValidatorLogger.Info("validate delete", "name", localModelNamespaceCache.Name, "namespace", localModelNamespaceCache.Namespace)

	// Delete protection relies on Status.InferenceServices / Status.LLMInferenceServices
	// being up-to-date. A newly created consumer may not appear in status yet if the
	// reconciler has not run, so deletion can race and succeed until the next reconcile.
	// This gap already existed for base-model references; LoRA adapter references inherit it.
	for _, isvcMeta := range localModelNamespaceCache.Status.InferenceServices {
		isvc := v1beta1.InferenceService{}
		if err := v.Get(ctx, client.ObjectKey(isvcMeta), &isvc); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			localModelNamespaceCacheValidatorLogger.Error(err, "Error getting InferenceService", "name", isvcMeta.Name, "namespace", isvcMeta.Namespace)
			return nil, err
		}
		modelName, ok := isvc.Labels[constants.LocalModelLabel]
		if !ok {
			continue
		}
		modelNamespace := isvc.Labels[constants.LocalModelNamespaceLabel]
		if modelName == localModelNamespaceCache.Name && modelNamespace == localModelNamespaceCache.Namespace {
			return admission.Warnings{}, fmt.Errorf("LocalModelNamespaceCache %s/%s is being used by InferenceService %s/%s",
				localModelNamespaceCache.Namespace, localModelNamespaceCache.Name, isvcMeta.Namespace, isvcMeta.Name)
		}
	}

	// Check if current LocalModelNamespaceCache is being used by LLMInferenceServices in the same namespace
	for _, llmIsvcMeta := range localModelNamespaceCache.Status.LLMInferenceServices {
		llmIsvc := v1alpha2.LLMInferenceService{}
		if err := v.Get(ctx, client.ObjectKey(llmIsvcMeta), &llmIsvc); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			localModelNamespaceCacheValidatorLogger.Error(err, "Error getting LLMInferenceService", "name", llmIsvcMeta.Name, "namespace", llmIsvcMeta.Namespace)
			return nil, err
		}
		if localmodelcache.LLMISVCReferencesNamespaceCache(
			localModelNamespaceCache.Name,
			localModelNamespaceCache.Namespace,
			llmIsvc.Namespace,
			llmIsvc.Labels,
			llmIsvc.Annotations,
		) {
			return admission.Warnings{}, fmt.Errorf("LocalModelNamespaceCache %s/%s is being used by LLMInferenceService %s/%s",
				localModelNamespaceCache.Namespace, localModelNamespaceCache.Name, llmIsvcMeta.Namespace, llmIsvcMeta.Name)
		}
	}
	return nil, nil
}

// validateStorageMode enforces the mutually-exclusive, at-least-one storage-mode rule and
// validates the pvcRef value. It mirrors the CEL rules on the spec so the same behavior is
// covered where direct unit tests and API lookups are needed. It covers all four cases:
// neither, node groups only, pvcRef only, and both.
func validateStorageMode(cache *v1alpha1.LocalModelNamespaceCache) error {
	hasNodeGroups := len(cache.Spec.NodeGroups) > 0
	hasPVCRef := cache.Spec.PVCRef != nil

	switch {
	case hasNodeGroups && hasPVCRef:
		return errors.New("nodeGroups and pvcRef are mutually exclusive")
	case !hasNodeGroups && !hasPVCRef:
		return errors.New("one of nodeGroups or pvcRef must be set")
	}

	if hasPVCRef {
		pvcName := *cache.Spec.PVCRef
		if pvcName == "" {
			return errors.New("pvcRef must not be empty")
		}
		if errs := apivalidation.IsDNS1123Subdomain(pvcName); len(errs) > 0 {
			return fmt.Errorf("pvcRef %q is not a valid PersistentVolumeClaim name: %v", pvcName, errs)
		}
	}
	return nil
}

// validatePVCRefImmutable rejects changes to pvcRef after creation.
func validatePVCRefImmutable(oldCache, newCache *v1alpha1.LocalModelNamespaceCache) error {
	oldRef := ""
	if oldCache.Spec.PVCRef != nil {
		oldRef = *oldCache.Spec.PVCRef
	}
	newRef := ""
	if newCache.Spec.PVCRef != nil {
		newRef = *newCache.Spec.PVCRef
	}
	if oldRef != newRef {
		return errors.New("pvcRef is immutable")
	}
	return nil
}

// validateDestinationConflict rejects a cache whose (namespace, pvcRef, storageKey) tuple is
// already claimed by another shared-PVC cache. Reconciliation remains authoritative against
// admission races, but this provides early feedback. Different storage keys on the same PVC
// are allowed.
func (v *LocalModelNamespaceCacheValidator) validateDestinationConflict(ctx context.Context, cache *v1alpha1.LocalModelNamespaceCache) error {
	storageKey := v1alpha1.GetStorageKey(cache.Spec.SourceModelUri)
	caches := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := v.List(ctx, caches, client.InNamespace(cache.Namespace)); err != nil {
		return err
	}
	for i := range caches.Items {
		other := &caches.Items[i]
		if other.Name == cache.Name {
			continue
		}
		if !other.Spec.SharedPVCMode() {
			continue
		}
		if *other.Spec.PVCRef != *cache.Spec.PVCRef {
			continue
		}
		if v1alpha1.GetStorageKey(other.Spec.SourceModelUri) == storageKey {
			return fmt.Errorf("destination pvc %q already holds model %q via cache %s/%s",
				*cache.Spec.PVCRef, cache.Spec.SourceModelUri, other.Namespace, other.Name)
		}
	}
	return nil
}

// validateNodeGroups checks that all node groups specified in the spec exist
func (v *LocalModelNamespaceCacheValidator) validateNodeGroups(ctx context.Context, cache *v1alpha1.LocalModelNamespaceCache) error {
	for _, nodeGroupName := range cache.Spec.NodeGroups {
		nodeGroup := &v1alpha1.LocalModelNodeGroup{}
		if err := v.Get(ctx, client.ObjectKey{Name: nodeGroupName}, nodeGroup); err != nil {
			return fmt.Errorf("NodeGroup %s not found: %w", nodeGroupName, err)
		}
	}
	return nil
}
