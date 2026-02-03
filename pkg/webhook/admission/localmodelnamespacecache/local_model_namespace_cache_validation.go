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
	"fmt"

	"github.com/kserve/kserve/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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

	// Validate node groups exist
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
	localModelNamespaceCacheValidatorLogger.Info("validate update", "name", localModelNamespaceCache.Name, "namespace", localModelNamespaceCache.Namespace)

	// Validate node groups exist
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

	// Check if current LocalModelNamespaceCache is being used by InferenceServices in the same namespace
	for _, isvcMeta := range localModelNamespaceCache.Status.InferenceServices {
		isvc := v1beta1.InferenceService{}
		if err := v.Client.Get(ctx, client.ObjectKey(isvcMeta), &isvc); err != nil {
			localModelNamespaceCacheValidatorLogger.Error(err, "Error getting InferenceService", "name", isvcMeta.Name, "namespace", isvcMeta.Namespace)
			return nil, err
		}
		modelName, ok := isvc.Labels[constants.LocalModelLabel]
		if !ok {
			continue
		}
		modelNamespace := isvc.Labels[constants.LocalModelNamespaceLabel]
		// Check if this ISVC is using this specific namespace-scoped cache
		if modelName == localModelNamespaceCache.Name && modelNamespace == localModelNamespaceCache.Namespace {
			return admission.Warnings{}, fmt.Errorf("LocalModelNamespaceCache %s/%s is being used by InferenceService %s/%s",
				localModelNamespaceCache.Namespace, localModelNamespaceCache.Name, isvcMeta.Namespace, isvcMeta.Name)
		}
	}
	return nil, nil
}

// validateNodeGroups checks that all node groups specified in the spec exist
func (v *LocalModelNamespaceCacheValidator) validateNodeGroups(ctx context.Context, cache *v1alpha1.LocalModelNamespaceCache) error {
	for _, nodeGroupName := range cache.Spec.NodeGroups {
		nodeGroup := &v1alpha1.LocalModelNodeGroup{}
		if err := v.Client.Get(ctx, client.ObjectKey{Name: nodeGroupName}, nodeGroup); err != nil {
			return fmt.Errorf("NodeGroup %s not found: %w", nodeGroupName, err)
		}
	}
	return nil
}
