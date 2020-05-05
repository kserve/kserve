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
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

var serviceAnnotationDisallowedList = []string{
	autoscaling.MinScaleAnnotationKey,
	autoscaling.MaxScaleAnnotationKey,
	constants.StorageInitializerSourceUriInternalAnnotationKey,
	"kubectl.kubernetes.io/last-applied-configuration",
}

const (
	// Set to 20% of the resource for main container, InferenceService defaults to 1CPU which is 200m for queue-proxy
	// https://github.com/knative/serving/blob/1d263950f9f2fea85a4dd394948a029c328af9d9/pkg/reconciler/revision/resources/resourceboundary.go#L30
	DefaultQueueSideCarResourcePercentage = "20"
)

type ServiceBuilder struct {
	inferenceServiceConfig *v1alpha2.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
}

func NewServiceBuilder(client client.Client, config *v1.ConfigMap) *ServiceBuilder {
	inferenceServiceConfig, err := v1alpha2.NewInferenceServicesConfig(config)
	if err != nil {
		fmt.Printf("Failed to get inference service config %s", err)
		panic("Failed to get inference service config")

	}
	return &ServiceBuilder{
		inferenceServiceConfig: inferenceServiceConfig,
		credentialBuilder:      credentials.NewCredentialBulder(client, config),
	}
}

func (c *ServiceBuilder) CreateInferenceServiceComponent(isvc *v1alpha2.InferenceService, component constants.InferenceServiceComponent, isCanary bool) (*knservingv1.Service, error) {
	serviceName := constants.DefaultServiceName(isvc.Name, component)
	if isCanary {
		serviceName = constants.CanaryServiceName(isvc.Name, component)
	}
	switch component {
	case constants.Predictor:
		predictorSpec := &isvc.Spec.Default.Predictor
		if isCanary {
			predictorSpec = &isvc.Spec.Canary.Predictor
		}
		return c.CreatePredictorService(serviceName, isvc.ObjectMeta, predictorSpec, isCanary)
	case constants.Transformer:
		transformerSpec := isvc.Spec.Default.Transformer
		if isCanary {
			transformerSpec = isvc.Spec.Canary.Transformer
		}
		if transformerSpec == nil {
			return nil, nil
		}
		return c.CreateTransformerService(serviceName, isvc.ObjectMeta, transformerSpec, isCanary)
	case constants.Explainer:
		explainerSpec := isvc.Spec.Default.Explainer
		predictorService := constants.PredictorURL(isvc.ObjectMeta, isCanary)
		if isvc.Spec.Default.Transformer != nil {
			predictorService = constants.TransformerURL(isvc.ObjectMeta, isCanary)
		}
		if explainerSpec == nil {
			return nil, nil
		}
		return c.CreateExplainerService(serviceName, isvc.ObjectMeta, explainerSpec, predictorService)
	}
	return nil, fmt.Errorf("Invalid Component")
}

func addLoggerAnnotations(logger *v1alpha2.Logger, annotations map[string]string) bool {
	if logger != nil {
		annotations[constants.LoggerInternalAnnotationKey] = "true"
		if logger.Url != nil {
			annotations[constants.LoggerSinkUrlInternalAnnotationKey] = *logger.Url
		}
		annotations[constants.LoggerModeInternalAnnotationKey] = string(logger.Mode)
		return true
	}
	return false
}

func addLoggerContainerPort(container *v1.Container) {
	if container != nil {
		if container.Ports == nil {
			port, _ := strconv.Atoi(constants.InferenceServiceDefaultLoggerPort)
			container.Ports = []v1.ContainerPort{
				v1.ContainerPort{
					ContainerPort: int32(port),
				},
			}
		}
	}
}

func (c *ServiceBuilder) CreatePredictorService(name string, metadata metav1.ObjectMeta, predictorSpec *v1alpha2.PredictorSpec, isCanary bool) (*knservingv1.Service, error) {
	annotations, err := c.buildAnnotations(metadata, predictorSpec.MinReplicas, predictorSpec.MaxReplicas, predictorSpec.Parallelism)
	if err != nil {
		return nil, err
	}

	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := predictorSpec.GetStorageUri(); sourceURI != "" {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = sourceURI
	}

	// Knative does not support multiple containers so we add an annotation that triggers pod
	// mutator to add it
	hasInferenceLogging := addLoggerAnnotations(predictorSpec.Logger, annotations)
	container := predictorSpec.GetContainer(metadata.Name, predictorSpec.Parallelism, c.inferenceServiceConfig)
	if hasInferenceLogging {
		addLoggerContainerPort(container)
	}

	endpoint := constants.InferenceServiceDefault
	if isCanary {
		endpoint = constants.InferenceServiceCanary
	}
	concurrency := int64(predictorSpec.Parallelism)
	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: utils.Union(metadata.Labels, map[string]string{
							constants.InferenceServicePodLabelKey: metadata.Name,
							constants.KServiceComponentLabel:      constants.Predictor.String(),
							constants.KServiceModelLabel:          metadata.Name,
							constants.KServiceEndpointLabel:       endpoint,
						}),
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds:       &constants.DefaultPredictorTimeout,
						ContainerConcurrency: &concurrency,
						PodSpec: v1.PodSpec{
							ServiceAccountName: predictorSpec.ServiceAccountName,
							Containers: []v1.Container{
								*container,
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

func (c *ServiceBuilder) CreateTransformerService(name string, metadata metav1.ObjectMeta, transformerSpec *v1alpha2.TransformerSpec, isCanary bool) (*knservingv1.Service, error) {
	annotations, err := c.buildAnnotations(metadata, transformerSpec.MinReplicas, transformerSpec.MaxReplicas, transformerSpec.Parallelism)
	if err != nil {
		return nil, err
	}

	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := transformerSpec.GetStorageUri(); sourceURI != "" {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = sourceURI
	}
	// Knative does not support multiple containers so we add an annotation that triggers pod
	// mutator to add it
	hasInferenceLogging := addLoggerAnnotations(transformerSpec.Logger, annotations)
	container := transformerSpec.GetContainerSpec(metadata, isCanary)
	if hasInferenceLogging {
		addLoggerContainerPort(container)
	}

	endpoint := constants.InferenceServiceDefault
	if isCanary {
		endpoint = constants.InferenceServiceCanary
	}

	concurrency := int64(transformerSpec.Parallelism)
	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: utils.Union(metadata.Labels, map[string]string{
							constants.InferenceServicePodLabelKey: metadata.Name,
							constants.KServiceComponentLabel:      constants.Transformer.String(),
							constants.KServiceModelLabel:          metadata.Name,
							constants.KServiceEndpointLabel:       endpoint,
						}),
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds:       &constants.DefaultTransformerTimeout,
						ContainerConcurrency: &concurrency,
						PodSpec: v1.PodSpec{
							ServiceAccountName: transformerSpec.ServiceAccountName,
							Containers: []v1.Container{
								*container,
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

func (c *ServiceBuilder) CreateExplainerService(name string, metadata metav1.ObjectMeta, explainerSpec *v1alpha2.ExplainerSpec, predictorService string) (*knservingv1.Service, error) {
	annotations, err := c.buildAnnotations(metadata, explainerSpec.MinReplicas, explainerSpec.MaxReplicas, explainerSpec.Parallelism)
	if err != nil {
		return nil, err
	}

	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// ModelInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := explainerSpec.GetStorageUri(); sourceURI != "" {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = sourceURI
	}

	// Knative does not support multiple containers so we add an annotation that triggers pod
	// mutator to add it
	hasInferenceLogging := addLoggerAnnotations(explainerSpec.Logger, annotations)
	container := explainerSpec.CreateExplainerContainer(metadata.Name, explainerSpec.Parallelism, predictorService, c.inferenceServiceConfig)
	if hasInferenceLogging {
		addLoggerContainerPort(container)
	}

	concurrency := int64(explainerSpec.Parallelism)
	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metadata.Namespace,
			Labels:    metadata.Labels,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: utils.Union(metadata.Labels, map[string]string{
							constants.InferenceServicePodLabelKey: metadata.Name,
							constants.KServiceComponentLabel:      constants.Explainer.String(),
							constants.KServiceModelLabel:          metadata.Name,
						}),
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds: &constants.DefaultExplainerTimeout,
						ContainerConcurrency: &concurrency,
						PodSpec: v1.PodSpec{
							ServiceAccountName: explainerSpec.ServiceAccountName,
							Containers: []v1.Container{
								*container,
							},
						},
					},
				},
			},
		},
	}

	if err := c.credentialBuilder.CreateSecretVolumeAndEnv(
		metadata.Namespace,
		explainerSpec.ServiceAccountName,
		&service.Spec.Template.Spec.Containers[0],
		&service.Spec.Template.Spec.Volumes,
	); err != nil {
		return nil, err
	}

	return service, nil
}

func (c *ServiceBuilder) buildAnnotations(metadata metav1.ObjectMeta, minReplicas *int, maxReplicas int, parallelism int) (map[string]string, error) {
	annotations := utils.Filter(metadata.Annotations, func(key string) bool {
		return !utils.Includes(serviceAnnotationDisallowedList, key)
	})

	if minReplicas == nil {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(constants.DefaultMinReplicas)
	} else if *minReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(*minReplicas)
	}

	if maxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(maxReplicas)
	}

	if _, ok := annotations[serving.QueueSideCarResourcePercentageAnnotation]; !ok {
		annotations[serving.QueueSideCarResourcePercentageAnnotation] = DefaultQueueSideCarResourcePercentage
	}
	// User can pass down scaling target annotation to overwrite the target default 1
	if _, ok := annotations[autoscaling.TargetAnnotationKey]; !ok {
		if parallelism == 0 {
			annotations[autoscaling.TargetAnnotationKey] = constants.DefaultScalingTarget
		} else {
			annotations[autoscaling.TargetAnnotationKey] = strconv.Itoa(parallelism)
		}
	}
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}
	return annotations, nil
}
