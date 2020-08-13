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
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("Reconciler")

type ConfigMapReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewConfigMapReconciler(client client.Client, scheme *runtime.Scheme) *ConfigMapReconciler {
	return &ConfigMapReconciler{
		client: client,
		scheme: scheme,
	}
}

func (c *ConfigMapReconciler) Reconcile(desired *corev1.ConfigMap, tm *v1beta1api.TrainedModel) error {
	if tm.DeletionTimestamp != nil {
		//A Trainedmodel is being deleted, remove the model from multi-model configmap
		deletedConfigs := []string{tm.Name}
		mConfig := modelconfig.NewConfigsDelta(nil, deletedConfigs)
		err := mConfig.Process(desired)
		if err != nil {
			return err
		}
	} else {
		//A Trainedmodel is created or updated, add or update the model from multi-model configmap
		modelConfig := modelconfig.ModelConfig{Name: tm.Name, Spec: tm.Spec.Inference}
		updatedConfigs := []modelconfig.ModelConfig{modelConfig}
		mConfig := modelconfig.NewConfigsDelta(updatedConfigs, nil)
		err := mConfig.Process(desired)
		if err != nil {
			return err
		}
	}
	err := c.client.Create(context.TODO(), desired)
	if err != nil {
		return err
	}
	return nil
}
