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

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	// logger for the validation webhook.
	clusterLocalModelValidatorLogger = logf.Log.WithName("clusterlocalmodel-v1alpha1-validation-webhook")
)

// +kubebuilder:object:generate=false
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
// ClusterLocalModelValidator is responsible for validating the ClusterLocalModelValidator resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false and +k8s:deepcopy-gen=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ClusterLocalModelValidator struct {
	client.Client
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-clusterlocalmodels,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=clusterlocalmodels,versions=v1alpha1,name=clusterlocalmodel.kserve-webhook-server.validator
var _ webhook.CustomValidator = &ClusterLocalModelValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *ClusterLocalModelValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	clusterLocalModel, err := convertToClusterLocalModel(obj)
	if err != nil {
		clusterLocalModelValidatorLogger.Error(err, "Unable to convert object to ClusterLocalModel")
		return nil, err
	}
	clusterLocalModelValidatorLogger.Info("validate create", "name", clusterLocalModel.Name)
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *ClusterLocalModelValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	clusterLocalModel, err := convertToClusterLocalModel(newObj)
	if err != nil {
		clusterLocalModelValidatorLogger.Error(err, "Unable to convert object to ClusterLocalModel")
		return nil, err
	}
	clusterLocalModelValidatorLogger.Info("validate update", "name", clusterLocalModel.Name)
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *ClusterLocalModelValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	clusterLocalModel, err := convertToClusterLocalModel(obj)
	if err != nil {
		clusterLocalModelValidatorLogger.Error(err, "Unable to convert object to ClusterLocalModel")
		return nil, err
	}
	clusterLocalModelValidatorLogger.Info("validate delete", "name", clusterLocalModel.Name)

	// Check if current ClusterLocalModel is being used
	for _, isvcMeta := range clusterLocalModel.Status.InferenceServices {
		isvc := v1beta1.InferenceService{}
		if err := v.Get(ctx, client.ObjectKey(isvcMeta), &isvc); err != nil {
			clusterLocalModelValidatorLogger.Error(err, "Error getting InferenceService", "name", isvcMeta.Name)
			return nil, err
		}
		modelName, ok := isvc.Labels[constants.LocalModelLabel]
		if !ok {
			continue
		}
		if modelName == clusterLocalModel.Name {
			return admission.Warnings{}, fmt.Errorf("ClusterLocalModel %s is being used by InferenceService %s", clusterLocalModel.Name, isvcMeta.Name)
		}
	}
	return nil, nil
}

// Convert runtime.Object into ClusterLocalModel
func convertToClusterLocalModel(obj runtime.Object) (*ClusterLocalModel, error) {
	clusterLocalModel, ok := obj.(*ClusterLocalModel)
	if !ok {
		return nil, fmt.Errorf("expected an ClusterLocalModel object but got %T", obj)
	}
	return clusterLocalModel, nil
}
