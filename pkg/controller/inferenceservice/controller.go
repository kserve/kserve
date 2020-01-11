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

	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/reconcilers/istio"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/reconcilers/knative"
	"k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	kfserving "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
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

// Add creates a new InferenceService Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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

	// Watch for changes to InferenceService
	if err = c.Watch(&source.Kind{Type: &kfserving.InferenceService{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	kfservingController := &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kfserving.InferenceService{},
	}

	// Watch for changes to Knative Service
	if err = c.Watch(&source.Kind{Type: &knservingv1alpha1.Service{}}, kfservingController); err != nil {
		return err
	}

	// Watch for changes to Virtual Service
	if err = c.Watch(&source.Kind{Type: &istiov1alpha3.VirtualService{}}, kfservingController); err != nil {
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

// Reconciler is implemented by all subresources
type Reconciler interface {
	Reconcile(isvc *v1alpha2.InferenceService) error
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=inferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=,resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the InferenceService instance
	isvc := &kfserving.InferenceService{}
	if err := r.Get(context.TODO(), request.NamespacedName, isvc); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	configMap := &v1.ConfigMap{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		log.Error(err, "Failed to find ConfigMap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KFServingNamespace)
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	reconcilers := []Reconciler{
		knative.NewServiceReconciler(r.Client, r.scheme, configMap),
		istio.NewVirtualServiceReconciler(r.Client, r.scheme, configMap),
	}

	for _, reconciler := range reconcilers {
		if err := reconciler.Reconcile(isvc); err != nil {
			log.Error(err, "Failed to reconcile")
			r.Recorder.Eventf(isvc, v1.EventTypeWarning, "InternalError", err.Error())
			return reconcile.Result{}, err
		}
	}

	if err = r.updateStatus(isvc); err != nil {
		r.Recorder.Eventf(isvc, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) updateStatus(desiredService *kfserving.InferenceService) error {
	existing := &kfserving.InferenceService{}
	namespacedName := types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}
	if err := r.Get(context.TODO(), namespacedName, existing); err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(existing.Status, desiredService.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if err := r.Update(context.TODO(), desiredService); err != nil {
		log.Error(err, "Failed to update InferenceService status")
		r.Recorder.Eventf(desiredService, v1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for InferenceService %q: %v", desiredService.Name, err)
		return err
	} else {
		// If there was a difference and there was no error.
		r.Recorder.Eventf(desiredService, v1.EventTypeNormal, "Updated", "Updated InferenceService %q", desiredService.GetName())
	}

	return nil
}
