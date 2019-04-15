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
	err = c.Watch(&source.Kind{Type: &kfservingv1alpha1.KFService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Knative Service
	err = c.Watch(&source.Kind{Type: &knservingv1alpha1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kfservingv1alpha1.KFService{},
	})
	if err != nil {
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
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kfserving.kubeflow.org,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kfserving.kubeflow.org,resources=services/status,verbs=get;update;patch
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the KFService instance
	kfsvc := &kfservingv1alpha1.KFService{}
	err := r.Get(context.TODO(), request.NamespacedName, kfsvc)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	desiredService, err := resources.CreateKnativeService(kfsvc)
	if err != nil {
		log.Error(err, "Failed to create desired ksvc", "name", desiredService.Name)
		r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}
	if err := controllerutil.SetControllerReference(kfsvc, desiredService, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	serviceReconciler := ksvc.NewServiceReconciler(r.Client)

	ksvc, err := serviceReconciler.Reconcile(context.TODO(), desiredService)
	if err != nil {
		log.Error(err, "Failed to reconcile ksvc", "name", desiredService.Name)
		r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}

	if err = r.updateStatus(kfsvc, ksvc); err != nil {
		r.Recorder.Eventf(kfsvc, v1.EventTypeWarning, "InternalError", err.Error())
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) updateStatus(before *kfservingv1alpha1.KFService, ksvc *knservingv1alpha1.Service) error {
	after := before.DeepCopy()
	if ksvc.Status.Address != nil {
		after.Status.URI.Internal = ksvc.Status.Address.Hostname
	}
	if before.Spec.Canary == nil ||
		(before.Spec.Canary.TrafficPercent == 0 && before.Spec.Canary != nil) {
		after.Status.Default.Name = ksvc.Status.LatestCreatedRevisionName
		after.Status.Canary.Name = ""
	} else {
		after.Status.Canary.Name = ksvc.Status.LatestCreatedRevisionName
	}
	for _, traffic := range ksvc.Status.Traffic {
		switch traffic.RevisionName {
		case after.Status.Default.Name:
			after.Status.Default.Traffic = traffic.Percent
		case after.Status.Canary.Name:
			after.Status.Canary.Traffic = traffic.Percent
		default:
		}
	}
	if equality.Semantic.DeepEqual(before.Status, after.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update                                                                                               rwrite a prior update
		// to status with this stale state.

	} else if err := r.Update(context.TODO(), after); err != nil {
		log.Error(err, "Failed to update kfsvc status")
		r.Recorder.Eventf(after, v1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Service %q: %v", after.Name, err)
		return err
	} else if err == nil {
		// If there was a difference and there was no error.
		r.Recorder.Eventf(after, v1.EventTypeNormal, "Updated", "Updated Service %q", after.GetName())
	}

	return nil
}
