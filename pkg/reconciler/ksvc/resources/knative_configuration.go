package resources

import (
	"fmt"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"

	"github.com/knative/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/frameworks/custom"
	"github.com/kubeflow/kfserving/pkg/frameworks/tensorflow"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			Name:      constants.DefaultConfigurationName(kfsvc.Name),
			Namespace: kfsvc.Namespace,
			Labels:    kfsvc.Labels,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: union(kfsvc.Labels, map[string]string{
						constants.KFServicePodLabelKey: kfsvc.Name,
					}),
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds: &constants.DefaultTimeout,
					},
					Container: CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Default),
				},
			},
		},
	}

	if kfsvc.Spec.Canary != nil {
		return defaultConfiguration, &knservingv1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.CanaryConfigurationName(kfsvc.Name),
				Namespace: kfsvc.Namespace,
				Labels:    kfsvc.Labels,
			},
			Spec: knservingv1alpha1.ConfigurationSpec{
				RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: union(kfsvc.Labels, map[string]string{
							constants.KFServicePodLabelKey: kfsvc.Name,
						}),
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							// Defaulting here since this always shows a diff with nil vs 300s(knative default)
							// we may need to expose this field in future
							TimeoutSeconds: &constants.DefaultTimeout,
						},
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
