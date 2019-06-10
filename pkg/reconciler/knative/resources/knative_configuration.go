/*
Copyright 2019 kubeflow.org.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"encoding/json"
	"fmt"
	"github.com/knative/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FRAMEWORK_CONFIG_KEY_NAME = "frameworks"
)

type ConfigurationBuilder struct {
	frameworksConfig *v1alpha1.FrameworksConfig
}

func NewConfigurationBuilder(config *v1.ConfigMap) *ConfigurationBuilder {
	frameworkConfig := &v1alpha1.FrameworksConfig{}
	if fmks, ok := config.Data[FRAMEWORK_CONFIG_KEY_NAME]; ok {
		err := json.Unmarshal([]byte(fmks), &frameworkConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall json string due to %v ", err))
		}
	}
	return &ConfigurationBuilder{
		frameworksConfig: frameworkConfig,
	}
}

func (c *ConfigurationBuilder) CreateKnativeConfiguration(name string, metadata metav1.ObjectMeta, modelSpec *v1alpha1.ModelSpec) *knservingv1alpha1.Configuration {
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

	configuration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
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
					Container: modelSpec.CreateModelServingContainer(metadata.Name, c.frameworksConfig),
				},
			},
		},
	}
	return configuration
}

func configurationAnnotationFilter(annotationKey string) bool {
	switch annotationKey {
	case autoscaling.TargetAnnotationKey:
		return true
	case autoscaling.ClassAnnotationKey:
		return true
	case constants.PvcNameAnnotation:
		return true
	case constants.PvcMountPathAnnotation:
		return true
	default:
		return false
	}
}
