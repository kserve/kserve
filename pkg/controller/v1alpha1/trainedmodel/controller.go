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

// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=trainedmodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=trainedmodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
package trainedmodel

import (
	"context"

	"github.com/go-logr/logr"
	v1alpha1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1alpha1/trainedmodel/reconcilers/modelconfig"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName("TrainedModel controller")

// TrainedModelReconciler reconciles a TrainedModel object
type TrainedModelReconciler struct {
	client.Client
	Log                   logr.Logger
	Scheme                *runtime.Scheme
	Recorder              record.EventRecorder
	ModelConfigReconciler *modelconfig.ModelConfigReconciler
}

func (r *TrainedModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the TrainedModel instance
	tm := &v1alpha1api.TrainedModel{}
	if err := r.Get(ctx, req.NamespacedName, tm); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// If the parent InferenceService does not exists, delete the trainedmodel
	isvc := &v1beta1api.InferenceService{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: req.Namespace, Name: tm.Spec.InferenceService}, isvc); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Parent InferenceService does not exists, deleting TrainedModel", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
			r.Delete(context.TODO(), tm)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	// Add parent InferenceService's name to TrainedModel's label
	if tm.Labels == nil {
		tm.Labels = make(map[string]string)
	}
	tm.Labels[constants.ParentInferenceServiceLabel] = isvc.Name

	// Use finalizer to handle TrainedModel deletion properly
	// When a TrainedModel object is being deleted it should
	// 1) Get its parent InferenceService
	// 2) Find its parent InferenceService model configmap
	// 3) Remove itself from the model configmap
	tmFinalizerName := "trainedmodel.finalizer"

	// examine DeletionTimestamp to determine if object is under deletion
	if tm.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !utils.ContainsString(tm.GetFinalizers(), tmFinalizerName) {
			tm.SetFinalizers(append(tm.GetFinalizers(), tmFinalizerName))
			if err := r.Update(context.Background(), tm); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if utils.ContainsString(tm.GetFinalizers(), tmFinalizerName) {
			//reconcile configmap to remove the model
			if err := r.ModelConfigReconciler.Reconcile(req, tm); err != nil {
				return reconcile.Result{}, err
			}
			// remove our finalizer from the list and update it.
			tm.SetFinalizers(utils.RemoveString(tm.GetFinalizers(), tmFinalizerName))
			if err := r.Update(context.Background(), tm); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// update URL and Address fo TrainedModel
	if err := r.updateStatus(req, tm); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile modelconfig to add this TrainedModel to its parent InferenceService's configmap
	if err := r.ModelConfigReconciler.Reconcile(req, tm); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TrainedModelReconciler) updateStatus(req ctrl.Request, desiredModel *v1alpha1api.TrainedModel) error {
	// Get the parent inference service
	isvc := &v1beta1api.InferenceService{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: req.Namespace, Name: desiredModel.Spec.InferenceService}, isvc); err != nil {
		return err
	}

	// Check if parent inference service has the status URL
	if isvc.Status.URL != nil {
		// Update status to contain the isvc URL with /v1/models/trained-model-name:predict appened
		url := isvc.Status.URL.String() + constants.PredictPath(desiredModel.Name, isvc.Spec.Predictor.GetImplementation().GetProtocol())
		externURL, err := apis.ParseURL(url)
		if err != nil {
			return err
		}
		desiredModel.Status.URL = externURL
	}

	// Check if parent inference service has the address URL
	if isvc.Status.Address != nil {
		if isvc.Status.Address.URL != nil {
			////Update status to contain the isvc address with /v1/models/trained-model-name:predict appened
			url := isvc.Status.Address.URL.String() + constants.PredictPath(desiredModel.Name, isvc.Spec.Predictor.GetImplementation().GetProtocol())
			clusterURL, err := apis.ParseURL(url)
			if err != nil {
				return err
			}
			desiredModel.Status.Address = &duckv1.Addressable{
				URL: clusterURL,
			}
		}
	}

	// Get the current model
	existingModel := &v1alpha1api.TrainedModel{}
	if err := r.Get(context.TODO(), req.NamespacedName, existingModel); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if equality.Semantic.DeepEqual(existingModel.Status, desiredModel.Status) {
		// We did not update anything
	} else {
		// Try to update model
		if err := r.Status().Update(context.TODO(), desiredModel); err != nil {
			r.Recorder.Eventf(desiredModel, v1.EventTypeWarning, "UpdateFailed",
				"Failed to update status for TrainedModel %q: %v", desiredModel.Name, err)
		}
	}

	return nil
}

func (r *TrainedModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.TrainedModel{}).
		Complete(r)
}
