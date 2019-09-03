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

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"knative.dev/serving/pkg/apis/serving/v1beta1"
)

const (
	FrameworkConfigKeyName = "frameworks"
)

var serviceAnnotationDisallowedList = []string{
	autoscaling.MinScaleAnnotationKey,
	autoscaling.MaxScaleAnnotationKey,
	constants.ModelInitializerSourceUriInternalAnnotationKey,
	"kubectl.kubernetes.io/last-applied-configuration",
}

type ServiceBuilder struct {
	frameworksConfig  *v1alpha2.FrameworksConfig
	credentialBuilder *credentials.CredentialBuilder
}

func NewServiceBuilder(client client.Client, config *v1.ConfigMap) *ServiceBuilder {
	frameworkConfig := &v1alpha2.FrameworksConfig{}
	if fmks, ok := config.Data[FrameworkConfigKeyName]; ok {
		err := json.Unmarshal([]byte(fmks), &frameworkConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall json string due to %v ", err))
		}
	}

	return &ServiceBuilder{
		frameworksConfig:  frameworkConfig,
		credentialBuilder: credentials.NewCredentialBulder(client, config),
	}
}

func (c *ServiceBuilder) CreateEndpointService(kfsvc *v1alpha2.KFService, endpoint constants.KFServiceEndpoint, isCanary bool) (*knservingv1alpha1.Service, error) {
	serviceName := constants.DefaultServiceName(kfsvc.Name, endpoint)
	if isCanary {
		serviceName = constants.CanaryServiceName(kfsvc.Name, endpoint)
	}
	switch endpoint {
	case constants.Predictor:
		predictorSpec := &kfsvc.Spec.Default.Predictor
		if isCanary {
			predictorSpec = &kfsvc.Spec.Canary.Predictor
		}
		return c.CreatePredictorService(serviceName, kfsvc.ObjectMeta, predictorSpec)
	case constants.Transformer:
		transformerSpec := kfsvc.Spec.Default.Transformer
		if isCanary {
			transformerSpec = kfsvc.Spec.Canary.Transformer
		}
		if transformerSpec == nil {
			return nil, nil
		}
		return c.CreateTransformerService(serviceName, kfsvc.ObjectMeta, transformerSpec, isCanary)
	case constants.Explainer:
		explainerSpec := &kfsvc.Spec.Default.Explainer
		if isCanary {
			explainerSpec = &kfsvc.Spec.Canary.Explainer
		}
		if explainerSpec == nil {
			return nil, nil
		}
		//TODO create explainer
		return nil, nil
	}
	return nil, fmt.Errorf("Invalid endpoint")
}

func (c *ServiceBuilder) CreatePredictorService(name string, metadata metav1.ObjectMeta, predictorSpec *v1alpha2.PredictorSpec) (*knservingv1alpha1.Service, error) {
	annotations := utils.Filter(metadata.Annotations, func(key string) bool {
		return !utils.Includes(serviceAnnotationDisallowedList, key)
	})

	if predictorSpec.MinReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(predictorSpec.MinReplicas)
	}
	if predictorSpec.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(predictorSpec.MaxReplicas)
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
	if sourceURI := predictorSpec.GetModelSourceUri(); sourceURI != "" {
		annotations[constants.ModelInitializerSourceUriInternalAnnotationKey] = sourceURI
	}

	service := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
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
								ServiceAccountName: predictorSpec.ServiceAccountName,
								Containers: []v1.Container{
									*predictorSpec.CreateModelServingContainer(metadata.Name, c.frameworksConfig),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := c.credentialBuilder.CreateSecretVolumeAndEnv(
		metadata.Namespace,
		predictorSpec.ServiceAccountName,
		&service.Spec.Template.Spec.Containers[0],
		&service.Spec.Template.Spec.Volumes,
	); err != nil {
		return nil, err
	}

	return service, nil
}

func (c *ServiceBuilder) CreateTransformerService(name string, metadata metav1.ObjectMeta, transformerSpec *v1alpha2.TransformerSpec, isCanary bool) (*knservingv1alpha1.Service, error) {
	annotations := utils.Filter(metadata.Annotations, func(key string) bool {
		return !utils.Includes(serviceAnnotationDisallowedList, key)
	})

	if transformerSpec.MinReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(transformerSpec.MinReplicas)
	}
	if transformerSpec.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(transformerSpec.MaxReplicas)
	}

	// User can pass down scaling target annotation to overwrite the target default 1
	if _, ok := annotations[autoscaling.TargetAnnotationKey]; !ok {
		annotations[autoscaling.TargetAnnotationKey] = constants.DefaultScalingTarget
	}
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	predict_url := constants.DefaultServiceName(metadata.Name, constants.Predictor) + "." + metadata.Namespace
	if isCanary {
		predict_url = constants.CanaryServiceName(metadata.Name, constants.Predictor) + "." + metadata.Namespace
	}
	service := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
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
								ServiceAccountName: transformerSpec.ServiceAccountName,
								Containers: []v1.Container{
									{
										Image: transformerSpec.Custom.Container.Image,
										Args: []string{
											"--model_name",
											metadata.Name,
											"--predict_url",
											"http://" + predict_url + "/v1/models/" + metadata.Name + ":predict",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := c.credentialBuilder.CreateSecretVolumeAndEnv(
		metadata.Namespace,
		transformerSpec.ServiceAccountName,
		&service.Spec.Template.Spec.Containers[0],
		&service.Spec.Template.Spec.Volumes,
	); err != nil {
		return nil, err
	}

	return service, nil
}
