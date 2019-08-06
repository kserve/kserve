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

package knative

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/knative/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FrameworkConfigKeyName = "frameworks"
)

var configurationAnnotationDisallowedList = []string{
	autoscaling.MinScaleAnnotationKey,
	autoscaling.MaxScaleAnnotationKey,
	constants.ModelInitializerSourceUriInternalAnnotationKey,
	"kubectl.kubernetes.io/last-applied-configuration",
}

type ConfigurationBuilder struct {
	frameworksConfig  *v1alpha1.FrameworksConfig
	credentialBuilder *credentials.CredentialBuilder
}

func NewConfigurationBuilder(client client.Client, config *v1.ConfigMap) *ConfigurationBuilder {
	frameworkConfig := &v1alpha1.FrameworksConfig{}
	if fmks, ok := config.Data[FrameworkConfigKeyName]; ok {
		err := json.Unmarshal([]byte(fmks), &frameworkConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall json string due to %v ", err))
		}
	}

	return &ConfigurationBuilder{
		frameworksConfig:  frameworkConfig,
		credentialBuilder: credentials.NewCredentialBulder(client, config),
	}
}

func (c *ConfigurationBuilder) CreateKnativeConfiguration(name string, metadata metav1.ObjectMeta, modelSpec *v1alpha1.ModelSpec) (*knservingv1alpha1.Configuration, error) {
	annotations := utils.Filter(metadata.Annotations, func(key string) bool {
		return !utils.Includes(configurationAnnotationDisallowedList, key)
	})

	if modelSpec.MinReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(modelSpec.MinReplicas)
	}
	if modelSpec.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(modelSpec.MaxReplicas)
	}

	// User can pass down scaling target annotation to overwrite the target default 1
	if _, ok := annotations[autoscaling.TargetAnnotationKey]; !ok {
		annotations[autoscaling.TargetAnnotationKey] = constants.DefaultScalingTarget
	}
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// ModelInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := modelSpec.GetModelSourceUri(); sourceURI != "" {
		annotations[constants.ModelInitializerSourceUriInternalAnnotationKey] = sourceURI
	}

	configuration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			Template: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: utils.Union(metadata.Labels, map[string]string{
						constants.KFServicePodLabelKey: metadata.Name,
					}),
					Annotations: annotations,
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							ServiceAccountName: modelSpec.ServiceAccountName,
							Containers: []v1.Container{
								*modelSpec.CreateModelServingContainer(metadata.Name, c.frameworksConfig),
							},
						},
					},
				},
			},
		},
	}

	if err := c.credentialBuilder.CreateSecretVolumeAndEnv(
		metadata.Namespace,
		modelSpec.ServiceAccountName,
		&configuration.Spec.Template.Spec.Containers[0],
		&configuration.Spec.Template.Spec.Volumes,
	); err != nil {
		return nil, err
	}

	return configuration, nil
}
