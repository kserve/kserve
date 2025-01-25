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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
	v1beta1utils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
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

func (c *ModelConfigReconciler) Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) error {
	if v1beta1utils.IsMMSPredictor(&isvc.Spec.Predictor) {
		// Create an empty modelConfig for every InferenceService shard
		// An InferenceService without storageUri is an empty model server with for multi-model serving so a modelConfig configmap should be created
		// An InferenceService with storageUri is considered as multi-model InferenceService with only one model, a modelConfig configmap should be created as well
		shardStrategy := memory.MemoryStrategy{}
		for _, id := range shardStrategy.GetShard(isvc) {
			modelConfigName := constants.ModelConfigName(isvc.Name, id)
			_, err := c.clientset.CoreV1().ConfigMaps(isvc.Namespace).Get(ctx, modelConfigName, metav1.GetOptions{})
			if err != nil {
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
					err = c.client.Create(ctx, newModelConfig)
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
