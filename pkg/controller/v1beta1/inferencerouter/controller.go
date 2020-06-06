package inferencerouter

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &Controller{}

// Controller for the InferenceRouter resource
type Controller struct {
	client.Client
	scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (c *Controller) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
