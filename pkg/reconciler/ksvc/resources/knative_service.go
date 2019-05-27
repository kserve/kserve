package resources

import (
	"fmt"
	"github.com/knative/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateKnativeService(name string, metadata metav1.ObjectMeta, modelSpec *v1alpha1.ModelSpec) *knservingv1alpha1.Service {
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

	// User can pass down scaling target annotation to overwrite the target default 1
	if _, ok := metadata.Annotations[autoscaling.TargetAnnotationKey]; !ok {
		annotations[autoscaling.TargetAnnotationKey] = constants.DefaultScalingTarget
	}
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := metadata.Annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	kfsvcAnnotations := utils.Filter(metadata.Annotations, configurationAnnotationFilter)

	configuration := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			RunLatest: &knservingv1alpha1.RunLatestType{
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: utils.Union(metadata.Labels, map[string]string{
								constants.KFServicePodLabelKey: metadata.Name,
							}),
							Annotations: utils.Union(kfsvcAnnotations, annotations),
						},
						Spec: knservingv1alpha1.RevisionSpec{
							RevisionSpec: v1beta1.RevisionSpec{
								// Defaulting here since this always shows a diff with nil vs 300s(knative default)
								// we may need to expose this field in future
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1beta1.PodSpec{
									ServiceAccountName: modelSpec.ServiceAccountName,
								},
							},
							Container: modelSpec.CreateModelServingContainer(metadata.Name),
						},
					},
				},
			},
		},
	}
	return configuration
}
