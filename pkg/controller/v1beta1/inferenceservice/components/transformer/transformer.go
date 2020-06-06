package transformer

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components"
)

var _ components.Component = &Transformer{}

// Transformer reconciles resources for this component.
type Transformer struct {
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (t *Transformer) Reconcile(isvc *v1beta1.InferenceService) error {
	return nil
}
