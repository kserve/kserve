/*
Copyright 2020 kubeflow.org.

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

package modelconfig

import (
	"context"
	"fmt"
	v1alpha1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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

func (c *ModelConfigReconciler) Reconcile(req ctrl.Request, tm *v1alpha1api.TrainedModel) error {
	log.Info("Reconciling TrainedModel", "apiVersion", tm.APIVersion, "trainedmodel", tm.Spec)
	shardStrategy := memory.MemoryStrategy{}
	shardId := shardStrategy.GetOrAssignShard(tm)
	// Use tm's parent InferenceService field to get the model modelConfig
	modelConfigName := constants.ModelConfigName(tm.Spec.InferenceService, shardId)
	desiredModelConfig := &corev1.ConfigMap{}
	log.Info("Reconciling modelConfig", "modelConfigName", modelConfigName)
	if err := c.client.Get(context.TODO(), types.NamespacedName{Name: modelConfigName, Namespace: req.Namespace}, desiredModelConfig); err != nil {
		log.Error(err, "Failed to find model ConfigMap to reconcile for InferenceService", "name", tm.Spec.Model, "namespace", req.Namespace)
		// Error reading the object - requeue the request.
		return err
	}
	if tm.DeletionTimestamp != nil {
		//A TrainedModel is being deleted, remove the model from the model configmap
		deletedConfigs := []string{tm.Name}
		configDelta := modelconfig.NewConfigsDelta([]modelconfig.ModelConfig{}, deletedConfigs)
		err := configDelta.Process(desiredModelConfig)
		if err != nil {
			return fmt.Errorf("Can not remove model %v from config because of error %v", tm.Name, err)
		}
		// Update the model Config created by the InferenceService controller
		err = c.client.Update(context.TODO(), desiredModelConfig)
		if err != nil {
			return err
		}
	} else {
		// A TrainedModel is created or updated, add or update the model from the model configmap
		modelConfig := modelconfig.ModelConfig{Name: tm.Name, Spec: tm.Spec.Model}
		updatedConfigs := []modelconfig.ModelConfig{modelConfig}
		configDelta := modelconfig.NewConfigsDelta(updatedConfigs, nil)
		err := configDelta.Process(desiredModelConfig)
		if err != nil {
			return fmt.Errorf("Can not add or update a model %v from config because of error %v", tm.Name, err)
		}
		// Update the model Config created by the InferenceService controller
		err = c.client.Update(context.TODO(), desiredModelConfig)
		if err != nil {
			return err
		}
	}
	return nil
}
