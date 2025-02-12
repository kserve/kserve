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

package modelconfig

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
	"github.com/kserve/kserve/pkg/modelconfig"
)

var log = logf.Log.WithName("Reconciler")

type ModelConfigReconciler struct {
	client    client.Client
	clientset kubernetes.Interface
	scheme    *runtime.Scheme
}

func NewModelConfigReconciler(client client.Client, clientset kubernetes.Interface, scheme *runtime.Scheme) *ModelConfigReconciler {
	return &ModelConfigReconciler{
		client:    client,
		clientset: clientset,
		scheme:    scheme,
	}
}

func (c *ModelConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request, tm *v1alpha1.TrainedModel) error {
	log.Info("Reconciling TrainedModel", "apiVersion", tm.APIVersion, "trainedmodel", tm.Spec)
	shardStrategy := memory.MemoryStrategy{}
	shardId := shardStrategy.GetOrAssignShard(tm)
	// Use tm's parent InferenceService field to get the model modelConfig
	modelConfigName := constants.ModelConfigName(tm.Spec.InferenceService, shardId)
	log.Info("Reconciling modelConfig", "modelConfigName", modelConfigName, "namespace", req.Namespace)
	desiredModelConfig, err := c.clientset.CoreV1().ConfigMaps(req.Namespace).Get(ctx, modelConfigName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to find model ConfigMap to reconcile for InferenceService", "name", tm.Spec.Model, "namespace", req.Namespace)
		// Error reading the object - requeue the request.
		return err
	}
	if tm.DeletionTimestamp != nil {
		// A TrainedModel is being deleted, remove the model from the model configmap
		deletedConfigs := []string{tm.Name}
		configDelta := modelconfig.NewConfigsDelta([]modelconfig.ModelConfig{}, deletedConfigs)
		err := configDelta.Process(desiredModelConfig)
		if err != nil {
			return fmt.Errorf("Can not remove model %v from config because of error %w", tm.Name, err)
		}
		// Update the model Config created by the InferenceService controller
		err = c.client.Update(ctx, desiredModelConfig)
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
			return fmt.Errorf("Can not add or update a model %v from config because of error %w", tm.Name, err)
		}
		// Update the model Config created by the InferenceService controller
		err = c.client.Update(ctx, desiredModelConfig)
		if err != nil {
			return err
		}
	}
	return nil
}
