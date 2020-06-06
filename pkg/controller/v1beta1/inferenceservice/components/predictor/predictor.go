package predictor

import (
	"fmt"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

var _ components.Component = &Predictor{}

// Predictor reconciles resources for this component.
type Predictor struct {
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (p *Predictor) Reconcile(isvc *v1beta1.InferenceService) error {
	d := appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: isvc.Spec.Predictor.GetContainers(),
				},
			},
		},
	}

	fmt.Printf("%v", d)

	return nil
}
