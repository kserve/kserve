package resources

import (
	"fmt"
	"log"

	"github.com/knative/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/frameworks"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateModelServingContainer(modelName string, modelSpec *v1alpha1.ModelSpec) *v1.Container {
	fwkHandler, err := frameworks.MakeHandler(modelSpec)
	if err != nil {
		log.Fatal(err)
	}
	return fwkHandler.CreateModelServingContainer(modelName)
}

func CreateKnativeConfiguration(kfsvc *v1alpha1.KFService) (*knservingv1alpha1.Configuration, *knservingv1alpha1.Configuration) {
	annotations := make(map[string]string)
	if kfsvc.Spec.MinReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(kfsvc.Spec.MinReplicas)
	}
	if kfsvc.Spec.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(kfsvc.Spec.MaxReplicas)
	}

	defaultConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.DefaultConfigurationName(kfsvc.Name),
			Namespace:   kfsvc.Namespace,
			Labels:      kfsvc.Labels,
			Annotations: union(kfsvc.Annotations, annotations),
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			Template: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: union(kfsvc.Labels, map[string]string{
						constants.KFServicePodLabelKey: kfsvc.Name,
					}),
					Annotations: kfsvc.Annotations,
				},
				Spec: knservingv1alpha1.RevisionSpec{
					Container: CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Default),
				},
			},
		},
	}

	if kfsvc.Spec.Canary != nil {
		return defaultConfiguration, &knservingv1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name:        constants.CanaryConfigurationName(kfsvc.Name),
				Namespace:   kfsvc.Namespace,
				Labels:      kfsvc.Labels,
				Annotations: union(kfsvc.Annotations, annotations),
			},
			Spec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: union(kfsvc.Labels, map[string]string{
							constants.KFServicePodLabelKey: kfsvc.Name,
						}),
						Annotations: kfsvc.Annotations,
					},
					Spec: knservingv1alpha1.RevisionSpec{
						Container: CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Canary.ModelSpec),
					},
				},
			},
		}
	}
	return defaultConfiguration, nil
}

func union(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
