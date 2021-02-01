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

package components

import (
	"strconv"

	"github.com/go-logr/logr"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	modelconfig "github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/modelconfig"
	v1beta1utils "github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kubeflow/kfserving/pkg/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
)

var _ Component = &Predictor{}

// Predictor reconciles resources for this component.
type Predictor struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
	Log                    logr.Logger
}

func NewPredictor(client client.Client, scheme *runtime.Scheme, inferenceServiceConfig *v1beta1.InferenceServicesConfig) Component {
	return &Predictor{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		Log:                    ctrl.Log.WithName("PredictorReconciler"),
	}
}

// Reconcile observes the predictor and attempts to drive the status towards the desired state.
func (p *Predictor) Reconcile(isvc *v1beta1.InferenceService) error {
	p.Log.Info("Reconciling Predictor", "PredictorSpec", isvc.Spec.Predictor)
	predictor := isvc.Spec.Predictor.GetImplementation()
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := predictor.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}
	hasInferenceLogging := addLoggerAnnotations(isvc.Spec.Predictor.Logger, annotations)
	hasInferenceBatcher := addBatcherAnnotations(isvc.Spec.Predictor.Batcher, annotations)
	// Add agent annotations so mutator will mount model agent to multi-model InferenceService's predictor
	addAgentAnnotations(isvc, annotations, p.client)

	objectMeta := metav1.ObjectMeta{
		Name:      constants.DefaultPredictorServiceName(isvc.Name),
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
		}),
		Annotations: annotations,
	}
	container := predictor.GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), p.inferenceServiceConfig)
	if len(isvc.Spec.Predictor.PodSpec.Containers) == 0 {
		isvc.Spec.Predictor.PodSpec.Containers = []v1.Container{
			*container,
		}
	} else {
		isvc.Spec.Predictor.PodSpec.Containers[0] = *container
	}
	//TODO now knative supports multi containers, consolidate logger/batcher/puller to the sidecar container
	//https://github.com/kubeflow/kfserving/issues/973
	if hasInferenceLogging {
		addLoggerContainerPort(&isvc.Spec.Predictor.PodSpec.Containers[0])
	}

	if hasInferenceBatcher {
		addBatcherContainerPort(&isvc.Spec.Predictor.PodSpec.Containers[0])
	}

	podSpec := v1.PodSpec(isvc.Spec.Predictor.PodSpec)

	// Reconcile modelConfig
	configMapReconciler := modelconfig.NewModelConfigReconciler(p.client, p.scheme)
	if err := configMapReconciler.Reconcile(isvc); err != nil {
		return err
	}

	// Here we allow switch between knative and vanilla deployment
	r := knative.NewKsvcReconciler(p.client, p.scheme, objectMeta, &isvc.Spec.Predictor.ComponentExtensionSpec,
		&podSpec, isvc.Status.Components[v1beta1.PredictorComponent])

	if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
		return errors.Wrapf(err, "fails to set owner reference for predictor")
	}
	status, err := r.Reconcile()
	if err != nil {
		return errors.Wrapf(err, "fails to reconcile predictor")
	}
	isvc.Status.PropagateStatus(v1beta1.PredictorComponent, status)
	return nil
}

func addLoggerAnnotations(logger *v1beta1.LoggerSpec, annotations map[string]string) bool {
	if logger != nil {
		annotations[constants.LoggerInternalAnnotationKey] = "true"
		if logger.URL != nil {
			annotations[constants.LoggerSinkUrlInternalAnnotationKey] = *logger.URL
		}
		annotations[constants.LoggerModeInternalAnnotationKey] = string(logger.Mode)
		return true
	}
	return false
}

func addLoggerContainerPort(container *v1.Container) {
	if container != nil {
		if container.Ports == nil || len(container.Ports) == 0 {
			port, _ := strconv.Atoi(constants.InferenceServiceDefaultAgentPort)
			container.Ports = []v1.ContainerPort{
				{
					ContainerPort: int32(port),
				},
			}
		}
	}
}

func addBatcherAnnotations(batcher *v1beta1.Batcher, annotations map[string]string) bool {
	if batcher != nil {
		annotations[constants.BatcherInternalAnnotationKey] = "true"

		if batcher.MaxBatchSize != nil {
			s := strconv.Itoa(*batcher.MaxBatchSize)
			annotations[constants.BatcherMaxBatchSizeInternalAnnotationKey] = s
		}
		if batcher.MaxLatency != nil {
			s := strconv.Itoa(*batcher.MaxLatency)
			annotations[constants.BatcherMaxLatencyInternalAnnotationKey] = s
		}
		if batcher.Timeout != nil {
			s := strconv.Itoa(*batcher.Timeout)
			annotations[constants.BatcherTimeoutInternalAnnotationKey] = s
		}
		return true
	}
	return false
}

func addBatcherContainerPort(container *v1.Container) {
	if container != nil {
		if container.Ports == nil || len(container.Ports) == 0 {
			port, _ := strconv.Atoi(constants.InferenceServiceDefaultAgentPort)
			container.Ports = []v1.ContainerPort{
				{
					ContainerPort: int32(port),
				},
			}
		}
	}
}

func addAgentAnnotations(isvc *v1beta1.InferenceService, annotations map[string]string, client client.Client) bool {
	if v1beta1utils.IsMMSPredictor(&isvc.Spec.Predictor, client) {
		annotations[constants.AgentShouldInjectAnnotationKey] = "true"
		shardStrategy := memory.MemoryStrategy{}
		for _, id := range shardStrategy.GetShard(isvc) {
			multiModelConfigMapName := constants.ModelConfigName(isvc.Name, id)
			annotations[constants.AgentModelConfigVolumeNameAnnotationKey] = multiModelConfigMapName
			annotations[constants.AgentModelConfigMountPathAnnotationKey] = constants.ModelConfigDir
			annotations[constants.AgentModelDirAnnotationKey] = constants.ModelDir
		}
		return true
	}
	return false
}
