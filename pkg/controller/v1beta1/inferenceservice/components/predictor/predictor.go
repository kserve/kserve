/*
Copyright 2020 kubeflow.org.

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

package predictor

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/multimodelconfig"
	knativeres "github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/resources/knative"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/trainedmodel/shard"
	"github.com/kubeflow/kfserving/pkg/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

var _ components.Component = &Predictor{}

// Predictor reconciles resources for this component.
type Predictor struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
	Log                    logr.Logger
}

func NewPredictor(client client.Client, scheme *runtime.Scheme, config *v1.ConfigMap) components.Component {
	inferenceServiceConfig, err := v1beta1.NewInferenceServicesConfig(config)
	if err != nil {
		fmt.Printf("Failed to get inference service config %s", err)
		panic("Failed to get inference service config")

	}
	return &Predictor{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		credentialBuilder:      credentials.NewCredentialBulder(client, config),
		Log:                    ctrl.Log.WithName("v1beta1Controllers").WithName("Predictor"),
	}
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (p *Predictor) Reconcile(isvc *v1beta1.InferenceService) error {
	propagateStatusFn := isvc.Status.PropagateStatus

	//Reconcile knative service
	var service *knservingv1.Service
	var err error
	service, err = p.CreatePredictorService(isvc)
	if err != nil {
		return err
	}

	r := knative.NewServiceReconciler(p.client, p.scheme)
	if err := controllerutil.SetControllerReference(isvc, service, p.scheme); err != nil {
		return err
	}
	if status, err := r.Reconcile(service); err != nil {
		return err
	} else {
		propagateStatusFn(v1beta1.PredictorComponent, status)
		return nil
	}
}

func (p *Predictor) CreatePredictorService(isvc *v1beta1.InferenceService) (*knservingv1.Service, error) {
	log := p.Log.WithValues("Predictor", isvc.Name)

	predictor, err := isvc.GetPredictor()
	if err != nil {
		return nil, err
	}
	log.Info("Reconciling Predictor", "PredictorSpec", isvc.Spec)
	serviceName := constants.DefaultServiceName(isvc.Name, constants.Predictor)

	annotations, err := knativeres.BuildAnnotations(isvc.ObjectMeta, isvc.Spec.Predictor.MinReplicas,
		isvc.Spec.Predictor.MaxReplicas, isvc.Spec.Predictor.ContainerConcurrency)
	if err != nil {
		return nil, err
	}

	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := predictor.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}
	container := predictor.GetContainer(isvc.Name, p.inferenceServiceConfig)

	// Knative does not support multiple containers so we add an annotation that triggers pod
	// mutator to add it
	/*hasInferenceLogging := knativeres.AddLoggerAnnotations(predictorSpec.Logger, annotations)

	if hasInferenceLogging {
		knativeres.AddLoggerContainerPort(container)
	}

	hasInferenceBatcher := knativeres.AddBatcherAnnotations(predictorSpec.Batcher, annotations)
	if hasInferenceBatcher {
		knativeres.AddBatcherContainerPort(container)
	}*/

	endpoint := constants.InferenceServiceDefault
	if isvc.Spec.Predictor.CustomPredictor == nil {
		isvc.Spec.Predictor.CustomPredictor = &v1beta1.CustomPredictor{
			PodTemplateSpec: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						*container,
					},
				},
			},
		}
	}
	log.Info("create predictor", "spec", isvc.Spec.Predictor.CustomPredictor)

	concurrency := int64(isvc.Spec.Predictor.ContainerConcurrency)
	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: isvc.Namespace,
			Labels:    isvc.Labels,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: utils.Union(isvc.Labels, map[string]string{
							constants.InferenceServicePodLabelKey: isvc.Name,
							constants.KServiceComponentLabel:      constants.Predictor.String(),
							constants.KServiceModelLabel:          isvc.Name,
							constants.KServiceEndpointLabel:       endpoint,
						}),
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						// Defaulting here since this always shows a diff with nil vs 300s(knative default)
						// we may need to expose this field in future
						TimeoutSeconds:       &constants.DefaultPredictorTimeout,
						ContainerConcurrency: &concurrency,
						PodSpec:              isvc.Spec.Predictor.CustomPredictor.Spec,
					},
				},
			},
			RouteSpec: knservingv1.RouteSpec{
				Traffic: []knservingv1.TrafficTarget{
					{
						Tag:     "default",
						Percent: proto.Int64(100),
					},
				},
			},
		},
	}

	//If InferenceService's storageUri is empty, create multi-model service configMap and
	//mount it into this predictor's knative service
	storageUri := isvc.Spec.Predictor.GetStorageUri()
	if storageUri == nil || len(*storageUri) == 0 {
		shardManager := shard.ShardManager{Strategy: shard.Memory}
		for _, id := range shardManager.GetShardIdsForInferenceService(isvc) {
			multiModelConfigMap, err := multimodelconfig.CreateEmptyMultiModelConfigMap(isvc, id)
			if err == nil {

				multiModelConfigVolume := v1.Volume{
					Name: constants.MultiModelConfigVolumeName,
					VolumeSource: v1.VolumeSource{
						ConfigMap: &v1.ConfigMapVolumeSource{},
					},
				}
				multiModelConfigVolume.ConfigMap.Name = multiModelConfigMap.Name
				// Mount the multi-model configmap to ksvc if not already mounted
				service.Spec.Template.Spec.Volumes = mountVolumeIfNotExist(service.Spec.Template.Spec.Volumes, multiModelConfigVolume)
				//TODO mount multi-model configmap in the the knative user container
			}
		}
	}
	if err := p.credentialBuilder.CreateSecretVolumeAndEnv(
		isvc.Namespace,
		isvc.Spec.Predictor.CustomPredictor.Spec.ServiceAccountName,
		&service.Spec.Template.Spec.Containers[0],
		&service.Spec.Template.Spec.Volumes,
	); err != nil {
		return nil, err
	}

	return service, nil
}

func mountVolumeIfNotExist(existingVolumes []v1.Volume, additionalVolume v1.Volume) []v1.Volume {
	if existingVolumes == nil {
		existingVolumes = []v1.Volume{}
	}
	for _, volume := range existingVolumes {
		if volume.Name == additionalVolume.Name {
			//additionalVolume exists
			return existingVolumes
		}
	}
	updatedVolumes := append(existingVolumes, additionalVolume)
	return updatedVolumes
}
