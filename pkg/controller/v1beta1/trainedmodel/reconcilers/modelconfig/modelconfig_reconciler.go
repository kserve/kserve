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
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func (c *ModelConfigReconciler) Reconcile(desired *corev1.ConfigMap, tm *v1beta1api.TrainedModel) error {
	if tm.DeletionTimestamp != nil {
		//A TrainedModel is being deleted, remove the model from the model configmap
		deletedConfigs := []string{tm.Name}
		configDelta := modelconfig.NewConfigsDelta([]modelconfig.ModelConfig{}, deletedConfigs)
		err := configDelta.Process(desired)
		if err != nil {
			return fmt.Errorf("Can not remove model %v from config because of error %v", tm.Name, err)
		}
		// Update the model Config created by the InferenceService controller
		err = c.client.Update(context.TODO(), desired)
		if err != nil {
			return fmt.Errorf("Can not delete model config %v", err)
		}
	} else {
		// A TrainedModel is created or updated, add or update the model from the model configmap
		modelConfig := modelconfig.ModelConfig{Name: tm.Name, Spec: tm.Spec.Model}
		updatedConfigs := []modelconfig.ModelConfig{modelConfig}
		configDelta := modelconfig.NewConfigsDelta(updatedConfigs, nil)
		err := configDelta.Process(desired)
		if err != nil {
			return fmt.Errorf("Can not add or update a model %v from config because of error %v", tm.Name, err)
		}
		// Update the model Config created by the InferenceService controller
		err = c.client.Update(context.TODO(), desired)
		if err != nil {
			return err
		}
	}
	return nil
}
