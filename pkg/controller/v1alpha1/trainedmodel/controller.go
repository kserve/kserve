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
	"fmt"
	"github.com/go-logr/logr"
	v1alpha1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1alpha1/trainedmodel/reconcilers/modelconfig"
	v1beta1utils "github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/utils"
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
		if !utils.Includes(tm.GetFinalizers(), tmFinalizerName) {
			tm.SetFinalizers(append(tm.GetFinalizers(), tmFinalizerName))
			if err := r.Update(context.Background(), tm); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if utils.Includes(tm.GetFinalizers(), tmFinalizerName) {
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

	// Check inferenceserviceready, frameworksupported, and memoryavailability
	if err := r.updateConditions(req, tm); err != nil {
		return reconcile.Result{}, err
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

func (r *TrainedModelReconciler) updateConditions(req ctrl.Request, tm *v1alpha1api.TrainedModel) error {
	// Get the parent inference service
	isvc := &v1beta1api.InferenceService{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: req.Namespace, Name: tm.Spec.InferenceService}, isvc); err != nil {
		return err
	}

	var conditionErr error = nil
	// Update Inference Service Ready condition
	if isvc.Status.IsReady() {
		log.Info("Parent InferenceService is ready", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		tm.Status.SetCondition(v1alpha1api.InferenceServiceReady, &apis.Condition{
			Status: v1.ConditionTrue,
		})
	} else {
		log.Info("Parent InferenceService is not ready", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		tm.Status.SetCondition(v1alpha1api.InferenceServiceReady, &apis.Condition{
			Type:    v1alpha1api.InferenceServiceReady,
			Status:  v1.ConditionFalse,
			Reason:  "InferenceServiceNotReady",
			Message: "Inference Service needs to be ready before Trained Model can be ready",
		})

		conditionErr = fmt.Errorf(InferenceServiceNotReady, isvc.Name, tm.Name)
	}

	isvcConfig, err := v1beta1api.NewInferenceServicesConfig(r.Client)
	if err != nil {
		return err
	}

	// Update Is MMS Predictor condition
	implementations := isvc.Spec.Predictor.GetImplementations()
	if len(implementations) > 0 && v1beta1utils.IsMMSPredictor(&isvc.Spec.Predictor, isvcConfig) {
		tm.Status.SetCondition(v1alpha1api.IsMMSPredictor, &apis.Condition{
			Status: v1.ConditionTrue,
		})
	} else {
		tm.Status.SetCondition(v1alpha1api.IsMMSPredictor, &apis.Condition{
			Type:    v1alpha1api.IsMMSPredictor,
			Status:  v1.ConditionFalse,
			Reason:  "IsNotMMSPredictor",
			Message: "Inference Service predictor is not configured for multi-model serving",
		})

		conditionErr = fmt.Errorf(IsNotMMSPredictor, isvc.Name, tm.Name)
	}

	// Update Framework Supported condition
	predictor := isvc.Spec.Predictor.GetPredictorImplementation()
	if predictor != nil && (*predictor).IsFrameworkSupported(tm.Spec.Model.Framework, isvcConfig) {
		log.Info("Framework is supported", "TrainedModel", tm.Name, "InferenceService", isvc.Name, "Framework", tm.Spec.Model.Framework)
		tm.Status.SetCondition(v1alpha1api.FrameworkSupported, &apis.Condition{
			Status: v1.ConditionTrue,
		})
	} else {
		log.Info("Framework is not supported", "TrainedModel", tm.Name, "InferenceService", isvc.Name, "Framework", tm.Spec.Model.Framework)
		tm.Status.SetCondition(v1alpha1api.FrameworkSupported, &apis.Condition{
			Type:    v1alpha1api.FrameworkSupported,
			Status:  v1.ConditionFalse,
			Reason:  "FrameworkNotSupported",
			Message: "Inference Service does not support the Trained Model framework",
		})

		conditionErr = fmt.Errorf(FrameworkNotSupported, isvc.Name, tm.Name, tm.Spec.Model.Framework)
	}

	// Get trained models with same inference service
	var trainedModels v1alpha1api.TrainedModelList
	if err := r.List(context.TODO(), &trainedModels, client.InNamespace(tm.Namespace), client.MatchingLabels{constants.ParentInferenceServiceLabel: isvc.Name, constants.TrainedModelAllocated: isvc.Name}); err != nil {
		return err
	}

	if _, ok := tm.Labels[constants.TrainedModelAllocated]; !ok {
		trainedModels.Items = append(trainedModels.Items, *tm)
	}

	totalReqMemory := trainedModels.TotalRequestedMemory()
	// Update Inference Service Resource Available condition
	if v1beta1utils.IsMemoryResourceAvailable(isvc, totalReqMemory, isvcConfig) {
		log.Info("Parent InferenceService memory resources are available", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		if _, ok := tm.Labels[constants.TrainedModelAllocated]; !ok {
			tm.Labels[constants.TrainedModelAllocated] = isvc.Name
			if updateErr := r.Update(context.Background(), tm); updateErr != nil {
				r.Log.Error(updateErr, "Failed to update TrainedModel label", "TrainedModel", tm.Name)
				return updateErr
			}
		}

		tm.Status.SetCondition(v1alpha1api.MemoryResourceAvailable, &apis.Condition{
			Status: v1.ConditionTrue,
		})
	} else {
		log.Info("Parent InferenceService memory resources are not available", "TrainedModel", tm.Name, "InferenceService", isvc.Name)
		tm.Status.SetCondition(v1alpha1api.MemoryResourceAvailable, &apis.Condition{
			Type:    v1alpha1api.MemoryResourceAvailable,
			Status:  v1.ConditionFalse,
			Reason:  "MemoryResourceNotAvailable",
			Message: "Inference Service does not have enough memory resources for Trained Model",
		})

		conditionErr = fmt.Errorf(MemoryResourceNotAvailable, isvc.Name, tm.Name)
	}

	if statusErr := r.Status().Update(context.TODO(), tm); statusErr != nil {
		r.Log.Error(statusErr, "Failed to update TrainedModel condition", "TrainedModel", tm.Name)
		r.Recorder.Eventf(tm, v1.EventTypeWarning, "UpdateFailed",
			"Failed to update conditions for TrainedModel: %v", statusErr)
		return statusErr
	}

	return conditionErr
}

func (r *TrainedModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.TrainedModel{}).
		Complete(r)
}
