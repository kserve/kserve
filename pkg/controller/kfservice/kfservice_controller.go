/*
Copyright 2019 kubeflow.org.

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

package service

import (
	"context"

	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubeflow/kfserving/pkg/reconciler/ksvc"
	"github.com/kubeflow/kfserving/pkg/reconciler/ksvc/resources"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"

	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	kfservingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	ControllerName = "kfserving-controller"
)

var log = logf.Log.WithName(ControllerName)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KFService Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	eventBroadcaster := record.NewBroadcaster()
	return &ReconcileService{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		Recorder: eventBroadcaster.NewRecorder(
			mgr.GetScheme(), v1.EventSource{Component: ControllerName}),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to KFService
	if err = c.Watch(&source.Kind{Type: &kfservingv1alpha1.KFService{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	kfservingController := &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kfservingv1alpha1.KFService{},
	}

	// Watch for changes to Knative Configuration
	if err = c.Watch(&source.Kind{Type: &knservingv1alpha1.Configuration{}}, kfservingController); err != nil {
		return err
	}

	// Watch for changes to Knative Route
	if err = c.Watch(&source.Kind{Type: &knservingv1alpha1.Route{}}, kfservingController); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Service object
type ReconcileService struct {
	client.Client
	scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
// +kubebuilder:rbac:groups=serving.knative.dev,resources=configurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=configurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=kfservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=kfservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=,resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the KFService instance
	kfsvc := &kfservingv1alpha1.KFService{}
	if err := r.Get(context.TODO(), request.NamespacedName, kfsvc); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	configMap := &v1.ConfigMap{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: constants.KFServingConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		log.Error(err, "Failed to find config map", "name", constants.KFServingConfigMapName)
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	credentialBuilder := ksvc.NewCredentialBulder(r.Client, configMap)

	serviceReconciler := ksvc.NewServiceReconciler(r.Client)
	// Reconcile configurations

	desiredDefault := resources.CreateKnativeConfiguration(constants.DefaultConfigurationName(kfsvc.Name),
		kfsvc.ObjectMeta, &kfsvc.Spec.Default, configMap.Data)

	if err := controllerutil.SetControllerReference(kfsvc, desiredDefault, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	if err := credentialBuilder.CreateSecretVolumeAndEnv(context.TODO(), request.Namespace, kfsvc.Spec.Default.ServiceAccountName,
		desiredDefault); err != nil {
		log.Error(err, "Failed to create credential volume or envs", "ServiceAccount", kfsvc.Spec.Default.ServiceAccountName)
	}

	defaultConfiguration, err := serviceReconciler.ReconcileConfiguration(context.TODO(), desiredDefault)
	if err != nil {
		log.Error(err, "Failed to reconcile default model spec", "name", desiredDefault.Name)
		r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}
	kfsvc.Status.PropagateDefaultConfigurationStatus(&defaultConfiguration.Status)

	if kfsvc.Spec.Canary != nil {
		desiredCanary := resources.CreateKnativeConfiguration(constants.CanaryConfigurationName(kfsvc.Name),
			kfsvc.ObjectMeta, kfsvc.Spec.Canary, configMap.Data)

		if err := controllerutil.SetControllerReference(kfsvc, desiredCanary, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		if err := credentialBuilder.CreateSecretVolumeAndEnv(context.TODO(), request.Namespace, kfsvc.Spec.Canary.ServiceAccountName,
			desiredCanary); err != nil {
			log.Error(err, "Failed to create credential volume or envs", "ServiceAccount", kfsvc.Spec.Canary.ServiceAccountName)
		}

		canaryConfiguration, err := serviceReconciler.ReconcileConfiguration(context.TODO(), desiredCanary)
		if err != nil {
			log.Error(err, "Failed to reconcile canary model spec", "name", desiredCanary.Name)
			r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
			return reconcile.Result{}, err
		}
		kfsvc.Status.PropagateCanaryConfigurationStatus(&canaryConfiguration.Status)
	}

	// Reconcile route
	desiredRoute := resources.CreateKnativeRoute(kfsvc)
	if err := controllerutil.SetControllerReference(kfsvc, desiredRoute, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	route, err := serviceReconciler.ReconcileRoute(context.TODO(), desiredRoute)
	if err != nil {
		log.Error(err, "Failed to reconcile route", "name", desiredRoute.Name)
		r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}
	kfsvc.Status.PropagateRouteStatus(&route.Status)

	if err = r.updateStatus(kfsvc); err != nil {
		r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) updateStatus(desiredService *kfservingv1alpha1.KFService) error {
	existing := &kfservingv1alpha1.KFService{}
	namespacedName := types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}
	if err := r.Get(context.TODO(), namespacedName, existing); err != nil {
		if errors.IsNotFound(err) {
			return err
		}
		return err
	}
	if equality.Semantic.DeepEqual(existing.Status, desiredService.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if err := r.Update(context.TODO(), desiredService); err != nil {
		log.Error(err, "Failed to update KFService status")
		r.Recorder.Eventf(desiredService, v1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for KFService %q: %v", desiredService.Name, err)
		return err
	} else if err == nil {
		// If there was a difference and there was no error.
		r.Recorder.Eventf(desiredService, v1.EventTypeNormal, "Updated", "Updated Service %q", desiredService.GetName())
	}

	return nil
}
