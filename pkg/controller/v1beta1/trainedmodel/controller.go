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

package trainedmodel

import (
	"context"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/scheduler"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/trainedmodel/reconcilers/multimodelconfig"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"github.com/go-logr/logr"
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TrainedModelReconciler reconciles a TrainedModel object
type TrainedModelReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=trainedmodel,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=trainedmodel/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
func (r *TrainedModelReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := r.Log.WithValues("trainedmodel", req.NamespacedName)

	// Fetch the TrainedModel instance
	trainedModel := &v1beta1api.TrainedModel{}
	if err := r.Get(context.TODO(), req.NamespacedName, trainedModel); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	log.Info("Reconciling TrainedModel", "apiVersion", trainedModel.APIVersion, "trainedModel", trainedModel.Spec)

	// Fetch the InferenceService
	isvc := &v1beta1api.InferenceService{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: trainedModel.Spec.InferenceService, Namespace: req.Namespace}, isvc); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	shardManager := scheduler.ShardManager{Strategy: scheduler.Memory}
	for _, id := range shardManager.GetShardId(isvc) {
		// Use trainedModel's parent InferenceService field to get the multi-model configMap
		multiModelConfigMapName := constants.DefaultMultiModelConfigMapName(trainedModel.Spec.InferenceService, id)
		configMap := &v1.ConfigMap{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: multiModelConfigMapName, Namespace: req.Namespace}, configMap)
		if err != nil {
			log.Error(err, "Failed to find Multi-model ConfigMap", "name", multiModelConfigMapName, "namespace", req.Namespace)
			// Error reading the object - requeue the request.
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}

		//Reconcile multi-model configmap to add/update/remove model files
		configMapReconciler := multimodelconfig.NewConfigMapReconciler(r.Client, r.Scheme)
		if err := configMapReconciler.Reconcile(configMap, trainedModel); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *TrainedModelReconciler) updateStatus(desiredService *v1beta1api.TrainedModel) error {
	//TODO update TrainedModel status object, this will be done in a separate PR
	return nil
}

func (r *TrainedModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1api.TrainedModel{}).
		Complete(r)
}
