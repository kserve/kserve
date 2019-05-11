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

func CreateKnativeConfiguration(kfsvc *v1alpha1.KFService, name string, modelSpec *v1alpha1.ModelSpec) *knservingv1alpha1.Configuration {
	if modelSpec == nil {
		return nil
	}
	annotations := make(map[string]string)
	if modelSpec.MinReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(modelSpec.MinReplicas)
	}
	if modelSpec.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(modelSpec.MaxReplicas)
	}

	//User can pass down scaling target annotation to overwrite the target default 1
	if _, ok := kfsvc.Annotations[autoscaling.TargetAnnotationKey]; !ok {
		annotations[autoscaling.TargetAnnotationKey] = constants.DefaultScalingTarget
	}
	//User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := kfsvc.Annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	kfsvcAnnotations := filter(kfsvc.Annotations, isFilteredConfigurationAnnotation)

	configuration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: kfsvc.Namespace,
			Labels:    kfsvc.Labels,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: union(kfsvc.Labels, map[string]string{
						constants.KFServicePodLabelKey: kfsvc.Name,
					}),
					Annotations: union(kfsvcAnnotations, annotations),
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds: &constants.DefaultTimeout,
					},
					Container: CreateModelServingContainer(kfsvc.Name, modelSpec),
				},
			},
		},
	}
	return configuration
}

func isFilteredConfigurationAnnotation(annotationKey string) bool {
	switch annotationKey {
	case autoscaling.TargetAnnotationKey:
		return true
	case autoscaling.ClassAnnotationKey:
		return true
	default:
		return false
	}
}
