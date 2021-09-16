/*

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

// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices;inferenceservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

package service

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/inferenceservice/reconcilers/istio"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/inferenceservice/reconcilers/knative"
	"k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler is implemented by all subresources
type Reconciler interface {
	Reconcile(isvc *v1alpha2.InferenceService) error
}

var _ reconcile.Reconciler = &InferenceServiceReconciler{}

// ReconcileService reconciles a Service object
type InferenceServiceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (r *InferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.InferenceService{}).
		Owns(&knservingv1.Service{}).
		Owns(&v1alpha3.VirtualService{}).
		Complete(r)
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
func (r *InferenceServiceReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the InferenceService instance
	isvc := &v1alpha2.InferenceService{}
	if err := r.Get(ctx, request.NamespacedName, isvc); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	r.Log.Info("Reconciling inference service", "apiVersion", isvc.APIVersion, "isvc", isvc.Name)
	configMap := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
	if err != nil {
		r.Log.Error(err, "Failed to find ConfigMap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	reconcilers := []Reconciler{
		knative.NewServiceReconciler(r.Client, r.Scheme, configMap),
		istio.NewVirtualServiceReconciler(r.Client, r.Scheme, configMap),
	}

	for _, reconciler := range reconcilers {
		if err := reconciler.Reconcile(isvc); err != nil {
			r.Log.Error(err, "Failed to reconcile")
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

func InferenceServiceReadiness(status v1alpha2.InferenceServiceStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == v1.ConditionTrue
}

func (r *InferenceServiceReconciler) updateStatus(desiredService *v1alpha2.InferenceService) error {
	existing := &v1alpha2.InferenceService{}
	namespacedName := types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}
	if err := r.Get(context.TODO(), namespacedName, existing); err != nil {
		return err
	}
	wasReady := InferenceServiceReadiness(existing.Status)
	if equality.Semantic.DeepEqual(existing.Status, desiredService.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if err := r.Status().Update(context.TODO(), desiredService); err != nil {
		r.Log.Error(err, "Failed to update InferenceService status")
		r.Recorder.Eventf(desiredService, v1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for InferenceService %q: %v", desiredService.Name, err)
		return err
	} else {
		// If there was a difference and there was no error.
		isReady := InferenceServiceReadiness(desiredService.Status)
		if wasReady && !isReady { // Moved to NotReady State
			r.Recorder.Eventf(desiredService, v1.EventTypeWarning, string(v1alpha2.InferenceServiceNotReadyState),
				fmt.Sprintf("InferenceService [%v] is no longer Ready", desiredService.GetName()))
		} else if !wasReady && isReady { // Moved to Ready State
			r.Recorder.Eventf(desiredService, v1.EventTypeNormal, string(v1alpha2.InferenceServiceReadyState),
				fmt.Sprintf("InferenceService [%v] is Ready", desiredService.GetName()))
		}
		r.Recorder.Eventf(desiredService, v1.EventTypeNormal, "Updated", "Updated InferenceService %q", desiredService.GetName())
	}
	return nil
}
