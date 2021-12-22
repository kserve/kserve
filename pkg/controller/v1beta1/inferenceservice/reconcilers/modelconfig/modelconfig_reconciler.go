/*
Copyright 2021 The KServe Authors.

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

package multimodelconfig

import (
	"context"

	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
	v1beta1utils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/modelconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("Reconciler")

type ModelConfigReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewModelConfigReconciler(client client.Client, scheme *runtime.Scheme) *ModelConfigReconciler {
	return &ModelConfigReconciler{
		client: client,
		scheme: scheme,
	}
}

func (c *ModelConfigReconciler) Reconcile(isvc *v1beta1api.InferenceService) error {
	isvcConfig, err := v1beta1api.NewInferenceServicesConfig(c.client)
	if err != nil {
		return err
	}
	if v1beta1utils.IsMMSPredictor(&isvc.Spec.Predictor, isvcConfig) {
		// Create an empty modelConfig for every InferenceService shard
		// An InferenceService without storageUri is an empty model server with for multi-model serving so a modelConfig configmap should be created
		// An InferenceService with storageUri is considered as multi-model InferenceService with only one model, a modelConfig configmap should be created as well
		shardStrategy := memory.MemoryStrategy{}
		for _, id := range shardStrategy.GetShard(isvc) {
			modelConfig := corev1.ConfigMap{}
			modelConfigName := types.NamespacedName{Name: constants.ModelConfigName(isvc.Name, id), Namespace: isvc.Namespace}
			if err := c.client.Get(context.TODO(), modelConfigName, &modelConfig); err != nil {
				if errors.IsNotFound(err) {
					// If the modelConfig does not exist for an InferenceService without storageUri, create an empty modelConfig
					log.Info("Creating modelConfig", "configmap", modelConfigName, "inferenceservice", isvc.Name, "namespace", isvc.Namespace)
					newModelConfig, err := modelconfig.CreateEmptyModelConfig(isvc, id)
					if err != nil {
						return err
					}
					if err := controllerutil.SetControllerReference(isvc, newModelConfig, c.scheme); err != nil {
						return err
					}
					err = c.client.Create(context.TODO(), newModelConfig)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
	}
	return nil
}
