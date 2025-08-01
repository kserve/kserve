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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=trainedmodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=trainedmodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
package trainedmodel

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/reconcilers/modelconfig"
	v1beta1utils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	InferenceServiceNotReady   = "Inference Service \"%s\" is not ready. Trained Model \"%s\" cannot deploy"
	FrameworkNotSupported      = "Inference Service \"%s\" does not support the Trained Model \"%s\" framework \"%s\""
	MemoryResourceNotAvailable = "Inference Service \"%s\" memory resources are not available. Trained Model \"%s\" cannot deploy"
	IsNotMMSPredictor          = "Inference Service \"%s\" predictor is not configured for multi-model serving. Trained Model \"%s\" cannot deploy"
)

var log = logf.Log.WithName("TrainedModel controller")

// TrainedModelReconciler reconciles a TrainedModel object
type TrainedModelReconciler struct {
	client.Client
	Clientset             kubernetes.Interface
	Log                   logr.Logger
	Scheme                *runtime.Scheme
	Recorder              record.EventRecorder
	ModelConfigReconciler *modelconfig.ModelConfigReconciler
}

func (r *TrainedModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the TrainedModel instance
	tm := &v1alpha1.TrainedModel{}
	if err := r.Get(ctx, req.NamespacedName, tm); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// If the parent InferenceService does not exists, delete the trainedmodel
	isvc := &v1beta1.InferenceService{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: tm.Spec.InferenceService}, isvc); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Parent InferenceService does not exists, deleting TrainedModel", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
			if err := r.Delete(ctx, tm); err != nil {
				log.Error(err, "Error deleting resource")
				return reconcile.Result{}, err
			}
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
		if !utils.Includes(tm.GetFinalizers(), tmFinalizerName) {
			tm.SetFinalizers(append(tm.GetFinalizers(), tmFinalizerName))
			if err := r.Update(ctx, tm); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if utils.Includes(tm.GetFinalizers(), tmFinalizerName) {
			// reconcile configmap to remove the model
			if err := r.ModelConfigReconciler.Reconcile(ctx, req, tm); err != nil {
				return reconcile.Result{}, err
			}
			// remove our finalizer from the list and update it.
			tm.SetFinalizers(utils.RemoveString(tm.GetFinalizers(), tmFinalizerName))
			if err := r.Update(ctx, tm); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Check inferenceserviceready, and memoryavailability
	if err := r.updateConditions(ctx, req, tm); err != nil {
		return reconcile.Result{}, err
	}

	// update URL and Address fo TrainedModel
	if err := r.updateStatus(ctx, req, tm); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile modelconfig to add this TrainedModel to its parent InferenceService's configmap
	if err := r.ModelConfigReconciler.Reconcile(ctx, req, tm); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TrainedModelReconciler) updateStatus(ctx context.Context, req ctrl.Request, desiredModel *v1alpha1.TrainedModel) error {
	// Get the parent inference service
	isvc := &v1beta1.InferenceService{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: desiredModel.Spec.InferenceService}, isvc); err != nil {
		return err
	}

	// Check if parent inference service has the status URL
	if isvc.Status.URL != nil {
		// Update status to contain the isvc URL with /v1/models/trained-model-name:predict appended
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
			////Update status to contain the isvc address with /v1/models/trained-model-name:predict appended
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
	existingModel := &v1alpha1.TrainedModel{}
	if err := r.Get(ctx, req.NamespacedName, existingModel); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if equality.Semantic.DeepEqual(existingModel.Status, desiredModel.Status) {
		// We did not update anything
	} else {
		// Try to update model
		if err := r.Status().Update(ctx, desiredModel); err != nil {
			r.Recorder.Eventf(desiredModel, corev1.EventTypeWarning, "UpdateFailed",
				"Failed to update status for TrainedModel %q: %v", desiredModel.Name, err)
		}
	}

	return nil
}

func (r *TrainedModelReconciler) updateConditions(ctx context.Context, req ctrl.Request, tm *v1alpha1.TrainedModel) error {
	// Get the parent inference service
	isvc := &v1beta1.InferenceService{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: tm.Spec.InferenceService}, isvc); err != nil {
		return err
	}

	var conditionErr error = nil
	// Update Inference Service Ready condition
	if isvc.Status.IsReady() {
		log.Info("Parent InferenceService is ready", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		tm.Status.SetCondition(v1alpha1.InferenceServiceReady, &apis.Condition{
			Status: corev1.ConditionTrue,
		})
	} else {
		log.Info("Parent InferenceService is not ready", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		tm.Status.SetCondition(v1alpha1.InferenceServiceReady, &apis.Condition{
			Type:    v1alpha1.InferenceServiceReady,
			Status:  corev1.ConditionFalse,
			Reason:  "InferenceServiceNotReady",
			Message: "Inference Service needs to be ready before Trained Model can be ready",
		})

		conditionErr = fmt.Errorf(InferenceServiceNotReady, isvc.Name, tm.Name)
	}

	// Update Is MMS Predictor condition
	implementations := isvc.Spec.Predictor.GetImplementations()
	if len(implementations) > 0 && v1beta1utils.IsMMSPredictor(&isvc.Spec.Predictor) {
		tm.Status.SetCondition(v1alpha1.IsMMSPredictor, &apis.Condition{
			Status: corev1.ConditionTrue,
		})
	} else {
		tm.Status.SetCondition(v1alpha1.IsMMSPredictor, &apis.Condition{
			Type:    v1alpha1.IsMMSPredictor,
			Status:  corev1.ConditionFalse,
			Reason:  "IsNotMMSPredictor",
			Message: "Inference Service predictor is not configured for multi-model serving",
		})

		conditionErr = fmt.Errorf(IsNotMMSPredictor, isvc.Name, tm.Name)
	}

	// Get trained models with same inference service
	var trainedModels v1alpha1.TrainedModelList
	if err := r.List(ctx, &trainedModels, client.InNamespace(tm.Namespace), client.MatchingLabels{constants.ParentInferenceServiceLabel: isvc.Name, constants.TrainedModelAllocated: isvc.Name}); err != nil {
		return err
	}

	if _, ok := tm.Labels[constants.TrainedModelAllocated]; !ok {
		trainedModels.Items = append(trainedModels.Items, *tm)
	}

	totalReqMemory := trainedModels.TotalRequestedMemory()
	// Update Inference Service Resource Available condition
	if v1beta1utils.IsMemoryResourceAvailable(isvc, totalReqMemory) {
		log.Info("Parent InferenceService memory resources are available", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		if _, ok := tm.Labels[constants.TrainedModelAllocated]; !ok {
			tm.Labels[constants.TrainedModelAllocated] = isvc.Name
			if updateErr := r.Update(ctx, tm); updateErr != nil {
				r.Log.Error(updateErr, "Failed to update TrainedModel label", "TrainedModel", tm.Name)
				return updateErr
			}
		}

		tm.Status.SetCondition(v1alpha1.MemoryResourceAvailable, &apis.Condition{
			Status: corev1.ConditionTrue,
		})
	} else {
		log.Info("Parent InferenceService memory resources are not available", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		tm.Status.SetCondition(v1alpha1.MemoryResourceAvailable, &apis.Condition{
			Type:    v1alpha1.MemoryResourceAvailable,
			Status:  corev1.ConditionFalse,
			Reason:  "MemoryResourceNotAvailable",
			Message: "Inference Service does not have enough memory resources for Trained Model",
		})

		conditionErr = fmt.Errorf(MemoryResourceNotAvailable, isvc.Name, tm.Name)
	}

	if statusErr := r.Status().Update(ctx, tm); statusErr != nil {
		r.Log.Error(statusErr, "Failed to update TrainedModel condition", "TrainedModel", tm.Name)
		r.Recorder.Eventf(tm, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update conditions for TrainedModel: %v", statusErr)
		return statusErr
	}

	return conditionErr
}

func (r *TrainedModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.TrainedModel{}).
		Complete(r)
}
