package inferenceservice

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components/explainer"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components/predictor"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components/transformer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &Controller{}

// Controller for the InferenceService resource
type Controller struct {
	client.Client
	scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (c *Controller) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// 1. Read state of world.
	isvc := &v1beta1.InferenceService{}

	// 2. Reconcile underlying resources.
	for _, component := range []components.Component{
		&predictor.Predictor{},
		&explainer.Explainer{},
		&transformer.Transformer{},
	} {
		component.Reconcile(isvc)
	}

	// 3. Persist status.
	// TODO

	return reconcile.Result{}, nil
}
