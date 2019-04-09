package resources

import (
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/containers/tensorflow"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getModelServingContainer(modelSpec *v1alpha1.ModelSpec, modelName string) *v1.Container {
	if modelSpec.Tensorflow != nil {
		return tensorflow.CreateTensorflowContainer(modelSpec.Tensorflow, modelName)
	} else {
		//TODO(@yuzisun) handle other model types
		return &v1.Container{}
	}
}

func CreateKnService(desiredService *v1alpha1.KFService) (*knservingv1alpha1.Service, error) {
	var revisions []string
	container := &v1.Container{}
	routingPercent := int32(0)
	if desiredService.Spec.Canary == nil ||
		(desiredService.Spec.Canary.TrafficPercent == 0 && desiredService.Spec.Canary != nil) {
		//TODO(@yuzisun) should we add model name to the spec, can be different than service name?
		container = getModelServingContainer(&desiredService.Spec.Default, desiredService.Name)
		revisions = []string{knservingv1alpha1.ReleaseLatestRevisionKeyword}
		routingPercent = 0
	} else if desiredService.Spec.Canary != nil {
		container = getModelServingContainer(&desiredService.Spec.Canary.ModelSpec, desiredService.Name)
		revisions = []string{desiredService.Status.Default.Name, knservingv1alpha1.ReleaseLatestRevisionKeyword}
		routingPercent = desiredService.Spec.Canary.TrafficPercent
	}
	return &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desiredService.Name,
			Namespace: desiredService.Namespace,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			Release: &knservingv1alpha1.ReleaseType{
				Revisions:      revisions,
				RolloutPercent: int(routingPercent),
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: *container,
						},
					},
				},
			},
		},
	}, nil
}
