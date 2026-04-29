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

package llminferenceservice

import (
	"context"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var defaulterLogger = logf.Log.WithName("llminferenceservice-defaulter")

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false

// +kubebuilder:webhook:path=/mutate-serving-kserve-io-v1alpha1-llminferenceservice,mutating=true,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=llminferenceservices,verbs=create;update,versions=v1alpha1,name=llminferenceservice.kserve-webhook-server.v1alpha1.defaulter,admissionReviewVersions=v1

// LLMInferenceServiceDefaulterV1Alpha1 sets local model cache labels on v1alpha1 LLMInferenceService resources.
type LLMInferenceServiceDefaulterV1Alpha1 struct {
	Client    client.Client
	Clientset kubernetes.Interface
}

var _ webhook.CustomDefaulter = &LLMInferenceServiceDefaulterV1Alpha1{}

func (d *LLMInferenceServiceDefaulterV1Alpha1) Default(ctx context.Context, obj runtime.Object) error {
	typedObj, ok := obj.(*v1alpha1.LLMInferenceService)
	if !ok {
		err := fmt.Errorf("unsupported object type %T for LLMInferenceService v1alpha1 defaulter", obj)
		defaulterLogger.Error(err, "Unable to convert object to LLMInferenceService v1alpha1")
		return err
	}

	llmSvcV2 := &v1alpha2.LLMInferenceService{}
	if err := typedObj.ConvertTo(llmSvcV2); err != nil {
		defaulterLogger.Error(err, "Unable to convert v1alpha1 object to LLMInferenceService v1alpha2")
		return err
	}
	if err := d.applyDefaults(ctx, llmSvcV2); err != nil {
		return err
	}
	if err := typedObj.ConvertFrom(llmSvcV2); err != nil {
		defaulterLogger.Error(err, "Unable to convert LLMInferenceService v1alpha2 object back to v1alpha1")
		return err
	}
	return nil
}

// +kubebuilder:webhook:path=/mutate-serving-kserve-io-v1alpha2-llminferenceservice,mutating=true,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=llminferenceservices,verbs=create;update,versions=v1alpha2,name=llminferenceservice.kserve-webhook-server.v1alpha2.defaulter,admissionReviewVersions=v1

// LLMInferenceServiceDefaulterV1Alpha2 sets local model cache labels on v1alpha2 LLMInferenceService resources.
type LLMInferenceServiceDefaulterV1Alpha2 struct {
	Client    client.Client
	Clientset kubernetes.Interface
}

var _ webhook.CustomDefaulter = &LLMInferenceServiceDefaulterV1Alpha2{}

func (d *LLMInferenceServiceDefaulterV1Alpha2) Default(ctx context.Context, obj runtime.Object) error {
	typedObj, ok := obj.(*v1alpha2.LLMInferenceService)
	if !ok {
		err := fmt.Errorf("unsupported object type %T for LLMInferenceService v1alpha2 defaulter", obj)
		defaulterLogger.Error(err, "Unable to convert object to LLMInferenceService v1alpha2")
		return err
	}
	return d.applyDefaults(ctx, typedObj)
}

// LLMInferenceServiceDefaulter is kept for shared tests and local usage.
// Production registration uses version-specific defaulters.
type LLMInferenceServiceDefaulter struct {
	Client    client.Client
	Clientset kubernetes.Interface
}

var _ webhook.CustomDefaulter = &LLMInferenceServiceDefaulter{}

func (d *LLMInferenceServiceDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	switch typedObj := obj.(type) {
	case *v1alpha2.LLMInferenceService:
		return applyDefaults(ctx, d.Client, d.Clientset, typedObj)
	case *v1alpha1.LLMInferenceService:
		llmSvcV2 := &v1alpha2.LLMInferenceService{}
		if err := typedObj.ConvertTo(llmSvcV2); err != nil {
			defaulterLogger.Error(err, "Unable to convert v1alpha1 object to LLMInferenceService v1alpha2")
			return err
		}
		if err := applyDefaults(ctx, d.Client, d.Clientset, llmSvcV2); err != nil {
			return err
		}
		if err := typedObj.ConvertFrom(llmSvcV2); err != nil {
			defaulterLogger.Error(err, "Unable to convert LLMInferenceService v1alpha2 object back to v1alpha1")
			return err
		}
		return nil
	default:
		err := fmt.Errorf("unsupported object type %T for LLMInferenceService defaulter", obj)
		defaulterLogger.Error(err, "Unable to convert object to LLMInferenceService")
		return err
	}
}

func (d *LLMInferenceServiceDefaulterV1Alpha1) applyDefaults(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	return applyDefaults(ctx, d.Client, d.Clientset, llmSvc)
}

func (d *LLMInferenceServiceDefaulterV1Alpha2) applyDefaults(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	return applyDefaults(ctx, d.Client, d.Clientset, llmSvc)
}

func applyDefaults(
	ctx context.Context,
	k8sClient client.Client,
	clientset kubernetes.Interface,
	llmSvc *v1alpha2.LLMInferenceService,
) error {
	llmSvc.SetDefaults(ctx)

	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, clientset)
	if err != nil {
		defaulterLogger.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return err
	}
	localModelConfig, err := v1beta1.NewLocalModelConfig(configMap)
	if err != nil {
		return err
	}

	_, localModelDisabled := llmSvc.Annotations[constants.DisableLocalModelKey]
	if !localModelDisabled && localModelConfig.Enabled {
		models := &v1alpha1.LocalModelCacheList{}
		if err := k8sClient.List(ctx, models); err != nil {
			defaulterLogger.Error(err, "Cannot list local models")
			return err
		}
		nsModels := &v1alpha1.LocalModelNamespaceCacheList{}
		if err := k8sClient.List(ctx, nsModels, client.InNamespace(llmSvc.Namespace)); err != nil {
			defaulterLogger.Error(err, "Cannot list namespace-scoped local models", "namespace", llmSvc.Namespace)
			return err
		}
		SetLocalModelLabel(llmSvc, models, nsModels)
	} else {
		DeleteLocalModelMetadata(llmSvc)
	}

	return nil
}

// SetLocalModelLabel sets local model labels on the LLMInferenceService if a matching cache exists.
// Namespace-scoped LocalModelNamespaceCache takes precedence over cluster-scoped LocalModelCache.
func SetLocalModelLabel(llmSvc *v1alpha2.LLMInferenceService, models *v1alpha1.LocalModelCacheList, nsModels *v1alpha1.LocalModelNamespaceCacheList) {
	modelUri := llmSvc.Spec.Model.URI.String()
	if modelUri == "" {
		return
	}

	isvcNodeGroup, isvcNodeGroupExists := llmSvc.Annotations[constants.NodeGroupAnnotationKey]

	// Check namespace-scoped LocalModelNamespaceCache first (higher priority)
	if nsModels != nil {
		for i, nsModel := range nsModels.Items {
			if nsModel.Spec.MatchStorageURI(modelUri) {
				var localModelPVCName string
				if isvcNodeGroupExists {
					if slices.Contains(nsModel.Spec.NodeGroups, isvcNodeGroup) {
						localModelPVCName = nsModel.Name + "-" + isvcNodeGroup
					} else {
						continue
					}
				} else {
					localModelPVCName = nsModel.Name + "-" + nsModel.Spec.NodeGroups[0]
				}
				if llmSvc.Labels == nil {
					llmSvc.Labels = make(map[string]string)
				}
				if llmSvc.Annotations == nil {
					llmSvc.Annotations = make(map[string]string)
				}
				llmSvc.Labels[constants.LocalModelLabel] = nsModels.Items[i].Name
				llmSvc.Labels[constants.LocalModelNamespaceLabel] = nsModels.Items[i].Namespace
				llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey] = nsModels.Items[i].Spec.SourceModelUri
				llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey] = localModelPVCName

				defaulterLogger.Info("LocalModelNamespaceCache found", "model", nsModels.Items[i].Name,
					"modelNamespace", nsModels.Items[i].Namespace, "llmSvcNamespace", llmSvc.Namespace, "llmSvc", llmSvc.Name)
				return
			}
		}
	}

	// Fall back to cluster-scoped LocalModelCache
	if models == nil {
		DeleteLocalModelMetadata(llmSvc)
		return
	}
	var localModel *v1alpha1.LocalModelCache
	var localModelPVCName string
	for i, model := range models.Items {
		if model.Spec.MatchStorageURI(modelUri) {
			if isvcNodeGroupExists {
				if slices.Contains(model.Spec.NodeGroups, isvcNodeGroup) {
					localModelPVCName = model.Name + "-" + isvcNodeGroup
				} else {
					continue
				}
			} else {
				localModelPVCName = model.Name + "-" + model.Spec.NodeGroups[0]
			}
			localModel = &models.Items[i]
			break
		}
	}
	if localModel == nil {
		DeleteLocalModelMetadata(llmSvc)
		return
	}
	if llmSvc.Labels == nil {
		llmSvc.Labels = make(map[string]string)
	}
	if llmSvc.Annotations == nil {
		llmSvc.Annotations = make(map[string]string)
	}
	llmSvc.Labels[constants.LocalModelLabel] = localModel.Name
	// Remove namespace label for cluster-scoped model (in case it was previously set)
	delete(llmSvc.Labels, constants.LocalModelNamespaceLabel)
	llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey] = localModel.Spec.SourceModelUri
	llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey] = localModelPVCName

	defaulterLogger.Info("LocalModelCache found", "model", localModel.Name, "namespace", llmSvc.Namespace, "llmSvc", llmSvc.Name)
}

// DeleteLocalModelMetadata removes local model cache internal labels and annotations
func DeleteLocalModelMetadata(llmSvc *v1alpha2.LLMInferenceService) {
	if llmSvc.Labels != nil {
		delete(llmSvc.Labels, constants.LocalModelLabel)
		delete(llmSvc.Labels, constants.LocalModelNamespaceLabel)
	}
	if llmSvc.Annotations != nil {
		delete(llmSvc.Annotations, constants.LocalModelSourceUriAnnotationKey)
		delete(llmSvc.Annotations, constants.LocalModelPVCNameAnnotationKey)
	}
}
