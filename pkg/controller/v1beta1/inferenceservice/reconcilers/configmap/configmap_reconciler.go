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

package configmap

import (
	"context"
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

//TODO: Check https://github.com/yuzisun/kfserving/blob/multi-model/pkg/controller/inferenceservice/resources/configmap/multi_model_service_config_map.go

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

func (c *ConfigMapReconciler) Reconcile(isvc *v1beta1api.InferenceService, req ctrl.Request) error {

	// Find the InferenceService's configmap. If its configmap does not exist, create an empty configmap.
	multiModelConfigMap := corev1.ConfigMap{}
	multiModelConfigMapName := types.NamespacedName{Name: constants.DefaultMultiModelConfigMapName(isvc.Name), Namespace: req.Namespace}
	if err := c.client.Get(context.TODO(), multiModelConfigMapName, &multiModelConfigMap); err != nil {
		if errors.IsNotFound(err) {
			storageUri := isvc.Spec.Predictor.GetStorageUri()
			if storageUri == nil {
				// If the InferenceService's storageUri is not set, create an empty multiModelConfigMap
				log.Info("Creating multimodel configmap", "configmap", multiModelConfigMapName, "inferenceservice", isvc.Name, "namespace", isvc.Namespace)
				newConfigMap, err := CreateEmptyMultiModelConfigMap(isvc)
				if err != nil {
					return err
				}
				if err := controllerutil.SetControllerReference(isvc, newConfigMap, c.scheme); err != nil {
					return err
				}
				err = c.client.Create(context.TODO(), newConfigMap)
				if err != nil {
					return err
				}
			}
		} else {
			return err
		}
	}
	return nil
}

func CreateEmptyMultiModelConfigMap(isvc *v1beta1api.InferenceService) (*corev1.ConfigMap, error) {
	multiModelConfigMapName := constants.DefaultMultiModelConfigMapName(isvc.Name)
	// Create a Multi-Model ConfigMap without any models in it
	multiModelConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      multiModelConfigMapName,
			Namespace: isvc.Namespace,
			Labels:    isvc.Labels,
		},
	}
	return multiModelConfigMap, nil
}
