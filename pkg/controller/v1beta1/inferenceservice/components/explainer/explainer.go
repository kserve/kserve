package explainer

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components"
)

var _ components.Component = &Explainer{}

// Explainer reconciles resources for this component.
type Explainer struct {
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (e *Explainer) Reconcile(isvc *v1beta1.InferenceService) error {
	return nil
}
