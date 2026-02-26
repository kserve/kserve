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
	"slices"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var llmMutatorLogger = logf.Log.WithName("llminferenceservice-v1alpha2-mutating-webhook")

// +kubebuilder:object:generate=false
type LLMInferenceServiceDefaulter struct {
	Scheme *runtime.Scheme
}

var _ webhook.CustomDefaulter = &LLMInferenceServiceDefaulter{}

func (d *LLMInferenceServiceDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	llmSvc, ok := obj.(*v1alpha2.LLMInferenceService)
	if !ok {
		return nil
	}
	llmMutatorLogger.Info("Defaulting LLMInferenceService", "namespace", llmSvc.Namespace, "name", llmSvc.Name)

	llmSvc.SetDefaults(ctx)

	cfg, err := config.GetConfig()
	if err != nil {
		llmMutatorLogger.Error(err, "unable to set up client config")
		return err
	}
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		llmMutatorLogger.Error(err, "unable to create clientSet")
		return err
	}
	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, clientSet)
	if err != nil {
		llmMutatorLogger.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return err
	}
	localModelConfig, err := v1beta1.NewLocalModelConfig(configMap)
	if err != nil {
		return err
	}

	_, localModelDisabled := llmSvc.Annotations[constants.DisableLocalModelKey]
	var models *v1alpha1.LocalModelCacheList
	var nsModels *v1alpha1.LocalModelNamespaceCacheList
	if !localModelDisabled && localModelConfig.Enabled {
		var c client.Client
		if c, err = client.New(cfg, client.Options{Scheme: d.Scheme}); err != nil {
			llmMutatorLogger.Error(err, "Failed to start client")
			return err
		}
		models = &v1alpha1.LocalModelCacheList{}
		if err := c.List(ctx, models); err != nil {
			llmMutatorLogger.Error(err, "Cannot List local models")
			return err
		}
		nsModels = &v1alpha1.LocalModelNamespaceCacheList{}
		if err := c.List(ctx, nsModels, client.InNamespace(llmSvc.Namespace)); err != nil {
			llmMutatorLogger.Error(err, "Cannot List namespace-scoped local models", "namespace", llmSvc.Namespace)
			return err
		}
	}

	SetLocalModelLabel(llmSvc, models, nsModels)
	return nil
}

// SetLocalModelLabel sets local model labels on the LLMInferenceService if a matching cache exists.
// Namespace-scoped LocalModelNamespaceCache takes precedence over cluster-scoped LocalModelCache.
func SetLocalModelLabel(llmSvc *v1alpha2.LLMInferenceService, models *v1alpha1.LocalModelCacheList, nsModels *v1alpha1.LocalModelNamespaceCacheList) {
	storageUri := llmSvc.Spec.Model.URI.String()
	if storageUri == "" {
		return
	}
	nodeGroup, nodeGroupExists := llmSvc.Annotations[constants.NodeGroupAnnotationKey]

	if nsModels != nil {
		for i, nsModel := range nsModels.Items {
			if nsModel.Spec.MatchStorageURI(storageUri) {
				var localModelPVCName string
				if nodeGroupExists {
					if slices.Contains(nsModel.Spec.NodeGroups, nodeGroup) {
						localModelPVCName = nsModel.Name + "-" + nodeGroup
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

				llmMutatorLogger.Info("LocalModelNamespaceCache found", "model", nsModels.Items[i].Name, "modelNamespace", nsModels.Items[i].Namespace, "namespace", llmSvc.Namespace, "llmisvc", llmSvc.Name)
				return
			}
		}
	}

	if models == nil {
		deleteLocalModelMetadata(llmSvc)
		return
	}
	var localModel *v1alpha1.LocalModelCache
	var localModelPVCName string
	for i, model := range models.Items {
		if model.Spec.MatchStorageURI(storageUri) {
			if nodeGroupExists {
				if slices.Contains(model.Spec.NodeGroups, nodeGroup) {
					localModelPVCName = model.Name + "-" + nodeGroup
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
		deleteLocalModelMetadata(llmSvc)
		return
	}
	if llmSvc.Labels == nil {
		llmSvc.Labels = make(map[string]string)
	}
	if llmSvc.Annotations == nil {
		llmSvc.Annotations = make(map[string]string)
	}
	llmSvc.Labels[constants.LocalModelLabel] = localModel.Name
	delete(llmSvc.Labels, constants.LocalModelNamespaceLabel)
	llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey] = localModel.Spec.SourceModelUri
	llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey] = localModelPVCName

	llmMutatorLogger.Info("LocalModelCache found", "model", localModel.Name, "namespace", llmSvc.Namespace, "llmisvc", llmSvc.Name)
}

func deleteLocalModelMetadata(llmSvc *v1alpha2.LLMInferenceService) {
	if llmSvc.Labels != nil {
		delete(llmSvc.Labels, constants.LocalModelLabel)
		delete(llmSvc.Labels, constants.LocalModelNamespaceLabel)
	}
	if llmSvc.Annotations != nil {
		delete(llmSvc.Annotations, constants.LocalModelSourceUriAnnotationKey)
		delete(llmSvc.Annotations, constants.LocalModelPVCNameAnnotationKey)
	}
}
