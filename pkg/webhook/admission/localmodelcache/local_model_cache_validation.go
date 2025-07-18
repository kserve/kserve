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

package localmodelcache

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
var localModelCacheValidatorLogger = logf.Log.WithName("localmodelcache-v1alpha1-validation-webhook")

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false
// LocalModelCacheValidator is responsible for validating the LocalModelCache resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type LocalModelCacheValidator struct {
	client.Client
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-localmodelcaches,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=localmodelcaches,versions=v1alpha1,name=localmodelcache.kserve-webhook-server.validator
var _ webhook.CustomValidator = &LocalModelCacheValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *LocalModelCacheValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	localModelCache, err := utils.Convert[*v1alpha1.LocalModelCache](obj)
	if err != nil {
		localModelCacheValidatorLogger.Error(err, "Unable to convert object to LocalModelCache")
		return nil, err
	}
	localModelCacheValidatorLogger.Info("validate create", "name", localModelCache.Name)
	localModelCacheWithSameStorageURI, err := v.validateUniqueStorageURI(ctx, localModelCache)
	if err != nil {
		localModelCacheValidatorLogger.Error(err, "Unable to check LocalModelCache with the same storage URI")
		return nil, err
	}
	if localModelCacheWithSameStorageURI != nil {
		return admission.Warnings{}, fmt.Errorf("LocalModelCache %s has the same StorageURI %s", localModelCacheWithSameStorageURI.Name, localModelCacheWithSameStorageURI.Spec.SourceModelUri)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *LocalModelCacheValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	localModelCache, err := utils.Convert[*v1alpha1.LocalModelCache](newObj)
	if err != nil {
		localModelCacheValidatorLogger.Error(err, "Unable to convert object to LocalModelCache")
		return nil, err
	}
	localModelCacheValidatorLogger.Info("validate update", "name", localModelCache.Name)
	localModelCacheWithSameStorageURI, err := v.validateUniqueStorageURI(ctx, localModelCache)
	if err != nil {
		localModelCacheValidatorLogger.Error(err, "Unable to check LocalModelCache with the same storage URI")
		return nil, err
	}
	if localModelCacheWithSameStorageURI != nil {
		return admission.Warnings{}, fmt.Errorf("LocalModelCache %s has the same StorageURI %s", localModelCacheWithSameStorageURI.Name, localModelCacheWithSameStorageURI.Spec.SourceModelUri)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *LocalModelCacheValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	localModelCache, err := utils.Convert[*v1alpha1.LocalModelCache](obj)
	if err != nil {
		localModelCacheValidatorLogger.Error(err, "Unable to convert object to LocalModelCache")
		return nil, err
	}
	localModelCacheValidatorLogger.Info("validate delete", "name", localModelCache.Name)

	// Check if current LocalModelCache is being used
	for _, isvcMeta := range localModelCache.Status.InferenceServices {
		isvc := v1beta1.InferenceService{}
		if err := v.Client.Get(ctx, client.ObjectKey(isvcMeta), &isvc); err != nil {
			localModelCacheValidatorLogger.Error(err, "Error getting InferenceService", "name", isvcMeta.Name)
			return nil, err
		}
		modelName, ok := isvc.Labels[constants.LocalModelLabel]
		if !ok {
			continue
		}
		if modelName == localModelCache.Name {
			return admission.Warnings{}, fmt.Errorf("LocalModelCache %s is being used by InferenceService %s", localModelCache.Name, isvcMeta.Name)
		}
	}
	return nil, nil
}

// Checks if there are other LocalModelCache with the same storage URI
func (v *LocalModelCacheValidator) validateUniqueStorageURI(ctx context.Context, currentLocalModelCache *v1alpha1.LocalModelCache) (*v1alpha1.LocalModelCache, error) {
	// Get all LocalModelCache CR
	localModelCacheList := &v1alpha1.LocalModelCacheList{}
	if err := v.Client.List(ctx, localModelCacheList); err != nil {
		localModelCacheValidatorLogger.Error(err, "Unable to list LocalModelCaches")
		return nil, err
	}
	// Check the storage URI of each LocalModelCache CR except for the current one
	for _, localModelCache := range localModelCacheList.Items {
		// Skip the current one
		if localModelCache.Name == currentLocalModelCache.Name {
			continue
		}
		if localModelCache.Spec.SourceModelUri == currentLocalModelCache.Spec.SourceModelUri {
			return &localModelCache, nil
		}
	}
	return nil, nil
}
