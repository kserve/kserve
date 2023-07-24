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

package inferenceservice

import (
	"context"
	"fmt"
	"k8s.io/client-go/util/retry"
	"reflect"

	"github.com/go-logr/logr"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/components"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/ingress"
	modelconfig "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/modelconfig"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices;inferenceservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes;servingruntimes/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterservingruntimes;clusterservingruntimes/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterservingruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// InferenceState describes the Readiness of the InferenceService
type InferenceServiceState string

// Different InferenceServiceState an InferenceService may have.
const (
	InferenceServiceReadyState    InferenceServiceState = "InferenceServiceReady"
	InferenceServiceNotReadyState InferenceServiceState = "InferenceServiceNotReady"
)

// InferenceServiceReconciler reconciles a InferenceService object
type InferenceServiceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (r *InferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()

	// Fetch the InferenceService instance
	isvc := &v1beta1api.InferenceService{}
	if err := r.Get(ctx, req.NamespacedName, isvc); err != nil {
		if apierr.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	//get annotations from isvc
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})

	deployConfig, err := v1beta1api.NewDeployConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create DeployConfig")
	}

	deploymentMode := isvcutils.GetDeploymentMode(annotations, deployConfig)
	r.Log.Info("Inference service deployment mode ", "deployment mode ", deploymentMode)

	if deploymentMode == constants.ModelMeshDeployment {
		if isvc.Spec.Transformer == nil {
			// Skip if no transformers
			r.Log.Info("Skipping reconciliation for InferenceService", constants.DeploymentMode, deploymentMode,
				"apiVersion", isvc.APIVersion, "isvc", isvc.Name)
			return ctrl.Result{}, nil
		}
		// Continue to reconcile when there is a transformer
		r.Log.Info("Continue reconciliation for InferenceService", constants.DeploymentMode, deploymentMode,
			"apiVersion", isvc.APIVersion, "isvc", isvc.Name)
	}
	// name of our custom finalizer
	finalizerName := "inferenceservice.finalizers"

	// examine DeletionTimestamp to determine if object is under deletion
	if isvc.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !utils.Includes(isvc.ObjectMeta.Finalizers, finalizerName) {
			isvc.ObjectMeta.Finalizers = append(isvc.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), isvc); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if utils.Includes(isvc.ObjectMeta.Finalizers, finalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(isvc); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			isvc.ObjectMeta.Finalizers = utils.RemoveString(isvc.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), isvc); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	r.Log.Info("Reconciling inference service", "apiVersion", isvc.APIVersion, "isvc", isvc.Name)
	isvcConfig, err := v1beta1api.NewInferenceServicesConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create InferenceServicesConfig")
	}
	reconcilers := []components.Component{}
	if deploymentMode != constants.ModelMeshDeployment {
		reconcilers = append(reconcilers, components.NewPredictor(r.Client, r.Scheme, isvcConfig, deploymentMode))
	}
	if isvc.Spec.Transformer != nil {
		reconcilers = append(reconcilers, components.NewTransformer(r.Client, r.Scheme, isvcConfig, deploymentMode))
	}
	if isvc.Spec.Explainer != nil {
		reconcilers = append(reconcilers, components.NewExplainer(r.Client, r.Scheme, isvcConfig, deploymentMode))
	}
	for _, reconciler := range reconcilers {
		result, err := reconciler.Reconcile(isvc)
		if err != nil {
			r.Log.Error(err, "Failed to reconcile", "reconciler", reflect.ValueOf(reconciler), "Name", isvc.Name)
			r.Recorder.Eventf(isvc, v1.EventTypeWarning, "InternalError", err.Error())
			r.updateStatus(isvc, deploymentMode)
			return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile component")
		}
		if result.Requeue || result.RequeueAfter > 0 {
			return result, nil
		}
	}
	// reconcile RoutesReady and LatestDeploymentReady conditions for serverless deployment
	if deploymentMode == constants.Serverless {
		componentList := []v1beta1api.ComponentType{v1beta1api.PredictorComponent}
		if isvc.Spec.Transformer != nil {
			componentList = append(componentList, v1beta1api.TransformerComponent)
		}
		if isvc.Spec.Explainer != nil {
			componentList = append(componentList, v1beta1api.ExplainerComponent)
		}
		isvc.Status.PropagateCrossComponentStatus(componentList, v1beta1api.RoutesReady)
		isvc.Status.PropagateCrossComponentStatus(componentList, v1beta1api.LatestDeploymentReady)
	}
	//Reconcile ingress
	ingressConfig, err := v1beta1api.NewIngressConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create IngressConfig")
	}

	//check raw deployment
	if deploymentMode == constants.RawDeployment {
		reconciler, err := ingress.NewRawIngressReconciler(r.Client, r.Scheme, ingressConfig)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
		}
		if err := reconciler.Reconcile(isvc); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
		}
	} else {
		reconciler := ingress.NewIngressReconciler(r.Client, r.Scheme, ingressConfig)
		r.Log.Info("Reconciling ingress for inference service", "isvc", isvc.Name)
		if err := reconciler.Reconcile(isvc); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
		}
	}

	// Reconcile modelConfig
	configMapReconciler := modelconfig.NewModelConfigReconciler(r.Client, r.Scheme)
	if err := configMapReconciler.Reconcile(isvc); err != nil {
		return reconcile.Result{}, err
	}

	if err = r.updateStatus(isvc, deploymentMode); err != nil {
		r.Recorder.Eventf(isvc, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *InferenceServiceReconciler) updateStatus(desiredService *v1beta1api.InferenceService, deploymentMode constants.DeploymentModeType) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existingService := &v1beta1api.InferenceService{}
		namespacedName := types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}
		if err := r.Get(context.TODO(), namespacedName, existingService); err != nil {
			return err
		}
		wasReady := inferenceServiceReadiness(existingService.Status)
		if inferenceServiceStatusEqual(existingService.Status, desiredService.Status, deploymentMode) {
			// If we didn't change anything then don't call updateStatus.
			// This is important because the copy we loaded from the informer's
			// cache may be stale and we don't want to overwrite a prior update
			// to status with this stale state.
		} else if err := r.Status().Update(context.TODO(), desiredService); err != nil {
			if apierr.IsConflict(err) {
				return err
			}
			r.Log.Error(err, "Failed to update InferenceService status", "InferenceService", desiredService.Name)
			r.Recorder.Eventf(desiredService, v1.EventTypeWarning, "UpdateFailed",
				"Failed to update status for InferenceService %q: %v", desiredService.Name, err)
			return errors.Wrapf(err, "fails to update InferenceService status")
		} else {
			// If there was a difference and there was no error.
			isReady := inferenceServiceReadiness(desiredService.Status)
			if wasReady && !isReady { // Moved to NotReady State
				r.Recorder.Eventf(desiredService, v1.EventTypeWarning, string(InferenceServiceNotReadyState),
					fmt.Sprintf("InferenceService [%v] is no longer Ready", desiredService.GetName()))
			} else if !wasReady && isReady { // Moved to Ready State
				r.Recorder.Eventf(desiredService, v1.EventTypeNormal, string(InferenceServiceReadyState),
					fmt.Sprintf("InferenceService [%v] is Ready", desiredService.GetName()))
			}
		}
		return nil
	})
	return err
}

func inferenceServiceReadiness(status v1beta1api.InferenceServiceStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == v1.ConditionTrue
}

func inferenceServiceStatusEqual(s1, s2 v1beta1api.InferenceServiceStatus, deploymentMode constants.DeploymentModeType) bool {
	if deploymentMode == constants.ModelMeshDeployment {
		// If the deployment mode is ModelMesh, reduce the status scope to compare.
		// Exclude Predictor and ModelStatus which are mananged by ModelMesh controllers
		return equality.Semantic.DeepEqual(s1.Address, s2.Address) &&
			equality.Semantic.DeepEqual(s1.URL, s2.URL) &&
			equality.Semantic.DeepEqual(s1.Status, s2.Status) &&
			equality.Semantic.DeepEqual(s1.Components[v1beta1api.TransformerComponent], s2.Components[v1beta1api.TransformerComponent]) &&
			equality.Semantic.DeepEqual(s1.Components[v1beta1api.ExplainerComponent], s2.Components[v1beta1api.ExplainerComponent])
	}
	return equality.Semantic.DeepEqual(s1, s2)
}

func (r *InferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager, deployConfig *v1beta1api.DeployConfig, ingressConfig *v1beta1api.IngressConfig) error {
	if deployConfig.DefaultDeploymentMode == string(constants.RawDeployment) {
		return ctrl.NewControllerManagedBy(mgr).
			For(&v1beta1api.InferenceService{}).
			Owns(&appsv1.Deployment{}).
			Complete(r)
	} else if ingressConfig.DisableIstioVirtualHost == false {
		return ctrl.NewControllerManagedBy(mgr).
			For(&v1beta1api.InferenceService{}).
			Owns(&knservingv1.Service{}).
			Owns(&v1alpha3.VirtualService{}).
			Owns(&appsv1.Deployment{}).
			Complete(r)
	} else {
		return ctrl.NewControllerManagedBy(mgr).
			For(&v1beta1api.InferenceService{}).
			Owns(&knservingv1.Service{}).
			Owns(&appsv1.Deployment{}).
			Complete(r)
	}

}

func (r *InferenceServiceReconciler) deleteExternalResources(isvc *v1beta1api.InferenceService) error {
	// Delete all the TrainedModel that uses this InferenceService as parent
	r.Log.Info("Deleting external resources", "InferenceService", isvc.Name)
	var trainedModels v1alpha1api.TrainedModelList
	if err := r.List(context.TODO(),
		&trainedModels,
		client.MatchingLabels{constants.ParentInferenceServiceLabel: isvc.Name},
		client.InNamespace(isvc.Namespace),
	); err != nil {
		r.Log.Error(err, "unable to list trained models", "inferenceservice", isvc.Name)
		return err
	}

	for _, v := range trainedModels.Items {
		if err := r.Delete(context.TODO(), &v, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			r.Log.Error(err, "unable to delete trainedmodel", "trainedmodel", v)
		}
	}
	return nil
}
