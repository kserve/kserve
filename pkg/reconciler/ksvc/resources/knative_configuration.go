package resources

import (
	"fmt"

	"github.com/knative/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	fwk "github.com/kubeflow/kfserving/pkg/frameworks"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateModelServingContainer(modelName string, modelSpec *v1alpha1.ModelSpec) *v1.Container {
	// ignoring error response since we assume validation ensured the modelSpec is valid
	fwkHandler, _ := fwk.MakeHandler(modelSpec)
	return fwkHandler.CreateModelServingContainer(modelName)
}

func CreateKnativeConfiguration(kfsvc *v1alpha1.KFService) (*knservingv1alpha1.Configuration, *knservingv1alpha1.Configuration) {
	var canaryContainer *v1.Container
	defaultContainer := CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Default)
	if kfsvc.Spec.Canary != nil {
		canaryContainer = CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Canary.ModelSpec)
	}
	annotations := make(map[string]string)

	if kfsvc.Spec.MinReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(kfsvc.Spec.MinReplicas)
	}
	if kfsvc.Spec.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(kfsvc.Spec.MaxReplicas)
	}
	for k, v := range kfsvc.Annotations {
		annotations[k] = v
	}
	defaultConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultConfigurationName(kfsvc.Name),
			Namespace: kfsvc.Namespace,
			Labels:    kfsvc.Labels,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
				Spec: knservingv1alpha1.RevisionSpec{
					Container: defaultContainer,
				},
			},
		},
	}
	if len(annotations) > 0 {
		defaultConfiguration.Annotations = annotations
	}
	if canaryContainer != nil {
		canaryConfiguration := &knservingv1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.CanaryConfigurationName(kfsvc.Name),
				Namespace: kfsvc.Namespace,
				Labels:    kfsvc.Labels,
			},
			Spec: knservingv1alpha1.ConfigurationSpec{
				RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
					Spec: knservingv1alpha1.RevisionSpec{
						Container: canaryContainer,
					},
				},
			},
		}
		if len(annotations) > 0 {
			canaryConfiguration.Annotations = annotations
		}
		return defaultConfiguration, canaryConfiguration
	} else {
		return defaultConfiguration, nil
	}
}
