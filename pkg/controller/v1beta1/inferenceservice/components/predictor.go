/*
Copyright 2021 The KServe Authors.

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
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	modelconfig "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/modelconfig"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

var _ Component = &Predictor{}

const (
	ErrInvalidPlaceholder         = "failed to replace placeholders in serving runtime %s Container %s"
	ErrNoContainerFound           = "no container configuration found in selected serving runtime"
	ErrRayClusterInsufficientGPUs = "the total required number of GPUs(%d) is less than the number of GPUs assigned to the head node(%d) + worker node(%d)"
)

// Predictor reconciles resources for this component.
type Predictor struct {
	client                 client.Client
	clientset              kubernetes.Interface
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	deploymentMode         constants.DeploymentModeType
	Log                    logr.Logger
}

func NewPredictor(client client.Client, clientset kubernetes.Interface, scheme *runtime.Scheme,
	inferenceServiceConfig *v1beta1.InferenceServicesConfig, deploymentMode constants.DeploymentModeType,
) Component {
	return &Predictor{
		client:                 client,
		clientset:              clientset,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		deploymentMode:         deploymentMode,
		Log:                    ctrl.Log.WithName("PredictorReconciler"),
	}
}

// Reconcile observes the predictor and attempts to drive the status towards the desired state.
func (p *Predictor) Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) (ctrl.Result, error) {
	var predContainer *corev1.Container
	var podSpec corev1.PodSpec
	var workerPodSpec *corev1.PodSpec
	var workerObjectMeta metav1.ObjectMeta
	var sRuntime v1alpha1.ServingRuntimeSpec
	var sRuntimeLabels map[string]string
	var sRuntimeAnnotations map[string]string
	multiNodeEnabled := false
	isvcGeneration := strconv.FormatInt(isvc.Generation, 10)

	// Set default value for multi-node
	if isvc.Spec.Predictor.WorkerSpec != nil {
		multiNodeEnabled = true
	}
	var annotations map[string]string
	if p.deploymentMode == constants.RawDeployment {
		annotations = utils.Filter(isvc.Annotations, func(key string) bool {
			// https://issues.redhat.com/browse/RHOAIENG-20326
			// For RawDeployment, we allow the security.opendatahub.io/enable-auth annotation
			return !utils.Includes(isvcutils.FilterList(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, constants.ODHKserveRawAuth), key)
		})
	} else {
		annotations = utils.Filter(isvc.Annotations, func(key string) bool {
			return !utils.Includes(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, key)
		})
	}

	p.Log.V(1).Info("Predictor custom annotations", "annotations", p.inferenceServiceConfig.ServiceAnnotationDisallowedList)
	p.Log.V(1).Info("Predictor custom labels", "labels", p.inferenceServiceConfig.ServiceLabelDisallowedList)

	addLoggerAnnotations(isvc.Spec.Predictor.Logger, annotations)
	addBatcherAnnotations(isvc.Spec.Predictor.Batcher, annotations)
	// Add StorageSpec annotations so mutator will mount storage credentials to InferenceService's predictor
	addStorageSpecAnnotations(isvc.Spec.Predictor.GetImplementation().GetStorageSpec(), annotations)
	// Add agent annotations so mutator will mount model agent to multi-model InferenceService's predictor
	addAgentAnnotations(isvc, annotations)

	// Reconcile modelConfig
	if err := p.reconcileModelConfig(ctx, isvc); err != nil {
		return ctrl.Result{}, err
	}

	predictor := isvc.Spec.Predictor.GetImplementation()

	// Knative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if err := p.addStorageInitializerAnnotations(ctx, predictor, annotations); err != nil {
		return ctrl.Result{}, err
	}

	// If Model is specified, prioritize using that. Otherwise, we will assume a framework object was specified.
	if isvc.Spec.Predictor.Model != nil {
		var err error
		sRuntime, err = p.reconcileModel(ctx, isvc, multiNodeEnabled)
		if err != nil {
			return ctrl.Result{}, err
		}
		podSpec, err = p.buildPodSpec(isvc, sRuntime)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		predContainer = predictor.GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), p.inferenceServiceConfig)
		podSpec = corev1.PodSpec(isvc.Spec.Predictor.PodSpec)
		if len(podSpec.Containers) == 0 {
			podSpec.Containers = []corev1.Container{
				*predContainer,
			}
		} else {
			podSpec.Containers[0] = *predContainer
		}
	}

	predictorName := p.getPredictorName(ctx, isvc)

	// Labels and annotations from predictor component
	// Label filter will be handled in ksvc_reconciler and raw reconciler
	predictorLabels := isvc.Spec.Predictor.Labels
	var predictorAnnotations map[string]string
	if p.deploymentMode == constants.RawDeployment {
		predictorAnnotations = utils.Filter(isvc.Spec.Predictor.Annotations, func(key string) bool {
			// https://issues.redhat.com/browse/RHOAIENG-20326
			// For RawDeployment, we allow the security.opendatahub.io/enable-auth annotation
			return !utils.Includes(isvcutils.FilterList(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, constants.ODHKserveRawAuth), key)
		})
	} else {
		predictorAnnotations = utils.Filter(isvc.Spec.Predictor.Annotations, func(key string) bool {
			return !utils.Includes(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, key)
		})
	}

	// Label filter will be handled in ksvc_reconciler
	sRuntimeLabels = sRuntime.ServingRuntimePodSpec.Labels
	sRuntimeAnnotations = utils.Filter(sRuntime.ServingRuntimePodSpec.Annotations, func(key string) bool {
		return !utils.Includes(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, key)
	})
	objectMeta := p.buildObjectMeta(isvc, predictorName, sRuntimeLabels, predictorLabels, sRuntimeAnnotations, annotations, predictorAnnotations)

	// Autoscaler should be ignored when multiNodeEnabled is true
	if multiNodeEnabled {
		var err error
		workerObjectMeta, workerPodSpec, err = p.reconcileWorker(sRuntime, isvc, &podSpec, annotations, predictorAnnotations, isvcGeneration)
		if err != nil {
			isvc.Status.PropagateRawStatusWithMessages(v1beta1.PredictorComponent, v1beta1.InvalidGPUAllocation, err.Error(), corev1.ConditionFalse)
			return ctrl.Result{}, err
		}
		objectMeta.Labels[constants.InferenceServiceGenerationPodLabelKey] = isvcGeneration
	}

	p.Log.Info("Resolved container", "container", predContainer, "podSpec", podSpec)
	var rawDeployment bool
	var podLabelKey string
	var podLabelValue string

	// Here we allow switch between knative and vanilla deployment
	kstatus := &knservingv1.ServiceStatus{}
	if p.deploymentMode == constants.RawDeployment {
		rawDeployment = true
		podLabelKey = constants.RawDeploymentAppLabel
		// This is main RawKubeReconciler to create objects (deployment, svc, scaler)
		if err := p.reconcileRawDeployment(ctx, isvc, objectMeta, workerObjectMeta, &podSpec, workerPodSpec); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		var err error
		podLabelKey = constants.RevisionLabel

		if kstatus, err = p.reconcileKnativeDeployment(ctx, isvc, &objectMeta, &podSpec); err != nil {
			return ctrl.Result{}, err
		}
		if isvc.GetForceStopRuntime() {
			// Exit early if we have already set the status to stopped
			existing_stopped_condition := isvc.Status.GetCondition(v1beta1.Stopped)
			if existing_stopped_condition != nil && existing_stopped_condition.Status == corev1.ConditionTrue {
				return ctrl.Result{}, nil
			}

			deployMode := isvc.Status.DeploymentMode

			// Clear all statuses
			isvc.Status = v1beta1.InferenceServiceStatus{}

			// Preserve the deployment mode value
			isvc.Status.DeploymentMode = deployMode

			// Set the ready condition
			predictor_ready_condition := &apis.Condition{
				Type:   v1beta1.PredictorReady,
				Status: corev1.ConditionFalse,
			}
			isvc.Status.SetCondition(v1beta1.PredictorReady, predictor_ready_condition)

			// Add the stopped condition
			stopped_condition := &apis.Condition{
				Type:   v1beta1.Stopped,
				Status: corev1.ConditionTrue,
			}
			isvc.Status.SetCondition(v1beta1.Stopped, stopped_condition)

			return ctrl.Result{}, nil
		} else {
			resume_condition := &apis.Condition{
				Type:   v1beta1.Stopped,
				Status: corev1.ConditionFalse,
			}
			isvc.Status.SetCondition(v1beta1.Stopped, resume_condition)
		}
	}

	statusSpec := isvc.Status.Components[v1beta1.PredictorComponent]
	if rawDeployment {
		podLabelValue = constants.GetRawServiceLabel(predictorName)
	} else {
		podLabelValue = statusSpec.LatestCreatedRevision
	}
	predictorPods, err := isvcutils.ListPodsByLabel(ctx, p.client, isvc.ObjectMeta.Namespace, podLabelKey, podLabelValue)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to list inferenceservice pods by label")
	}

	if isvc.Status.PropagateModelStatus(statusSpec, predictorPods, rawDeployment, kstatus) {
		return ctrl.Result{}, nil
	} else {
		return ctrl.Result{Requeue: true}, nil
	}
}

func (p *Predictor) reconcileModelConfig(ctx context.Context, isvc *v1beta1.InferenceService) error {
	configMapReconciler := modelconfig.NewModelConfigReconciler(p.client, p.clientset, p.scheme)
	return configMapReconciler.Reconcile(ctx, isvc)
}

func (p *Predictor) addStorageInitializerAnnotations(ctx context.Context, predictor v1beta1.ComponentImplementation, annotations map[string]string) error {
	if sourceURI := predictor.GetStorageUri(); sourceURI != nil {
		if _, ok := annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]; ok {
			return errors.New("must provide only one of storageUri and storage.path")
		}
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
		err := isvcutils.ValidateStorageURI(ctx, sourceURI, p.client)
		if err != nil {
			return fmt.Errorf("StorageURI not supported: %w", err)
		}
	}
	return nil
}

func (p *Predictor) reconcileModel(ctx context.Context, isvc *v1beta1.InferenceService, multiNodeEnabled bool) (v1alpha1.ServingRuntimeSpec, error) {
	var sRuntime v1alpha1.ServingRuntimeSpec

	if isvc.Spec.Predictor.Model.Runtime != nil {
		// set runtime defaults
		isvc.SetRuntimeDefaults()
		r, err := isvcutils.GetServingRuntime(ctx, p.client, *isvc.Spec.Predictor.Model.Runtime, isvc.Namespace)
		if err != nil {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.RuntimeNotRecognized,
				Message: "Waiting for runtime to become available",
			})
			return sRuntime, err
		}

		if r.IsDisabled() {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.RuntimeDisabled,
				Message: "Specified runtime is disabled",
			})
			return sRuntime, fmt.Errorf("specified runtime %s is disabled", *isvc.Spec.Predictor.Model.Runtime)
		}

		if isvc.Spec.Predictor.Model.ProtocolVersion != nil &&
			!r.IsProtocolVersionSupported(*isvc.Spec.Predictor.Model.ProtocolVersion) {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.NoSupportingRuntime,
				Message: "Specified runtime does not support specified protocol version",
			})
			return sRuntime, fmt.Errorf("specified runtime %s does not support specified protocol version", *isvc.Spec.Predictor.Model.Runtime)
		}

		// Verify that the selected runtime supports the specified framework.
		if !isvc.Spec.Predictor.Model.RuntimeSupportsModel(r) {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.NoSupportingRuntime,
				Message: "Specified runtime does not support specified framework/version",
			})
			return sRuntime, fmt.Errorf("specified runtime %s does not support specified framework/version", *isvc.Spec.Predictor.Model.Runtime)
		}

		sRuntime = *r
	} else {
		runtimes, err := isvc.Spec.Predictor.Model.GetSupportingRuntimes(ctx, p.client, isvc.Namespace, false, multiNodeEnabled)
		if err != nil {
			return sRuntime, err
		}
		if len(runtimes) == 0 {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.NoSupportingRuntime,
				Message: "No runtime found to support specified framework/version",
			})
			return sRuntime, fmt.Errorf("no runtime found to support predictor with model type: %v", isvc.Spec.Predictor.Model.ModelFormat)
		}
		// Get first supporting runtime.
		sRuntime = runtimes[0].Spec
		isvc.Spec.Predictor.Model.Runtime = &runtimes[0].Name

		// set runtime defaults
		isvc.SetRuntimeDefaults()
	}
	// assign protocol version to inferenceservice based on runtime selected
	if isvc.Spec.Predictor.Model.ProtocolVersion == nil {
		protocolVersion := constants.GetProtocolVersionString(
			constants.ProtocolVersion(
				v1beta1.GetProtocolVersionPriority(sRuntime.ProtocolVersions),
			),
		)
		isvc.Spec.Predictor.Model.ProtocolVersion = &protocolVersion
	}

	return sRuntime, nil
}

func (p *Predictor) buildPodSpec(isvc *v1beta1.InferenceService, sRuntime v1alpha1.ServingRuntimeSpec) (corev1.PodSpec, error) {
	var podSpec corev1.PodSpec
	var predContainer *corev1.Container
	var err error

	if len(sRuntime.Containers) == 0 {
		isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
			Reason:  v1beta1.InvalidPredictorSpec,
			Message: ErrNoContainerFound,
		})
		return podSpec, errors.New(ErrNoContainerFound)
	}
	var mergedPodSpec *corev1.PodSpec
	_, predContainer, mergedPodSpec, err = isvcutils.MergeServingRuntimeAndInferenceServiceSpecs(sRuntime.Containers, isvc.Spec.Predictor.Model.Container, isvc, constants.InferenceServiceContainerName, sRuntime.ServingRuntimePodSpec, isvc.Spec.Predictor.PodSpec)
	if err != nil {
		return podSpec, err
	}

	// Replace placeholders in runtime container by values from inferenceservice metadata
	if err = isvcutils.ReplacePlaceholders(predContainer, isvc.ObjectMeta); err != nil {
		isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
			Reason:  v1beta1.InvalidPredictorSpec,
			Message: fmt.Sprintf(ErrInvalidPlaceholder, *isvc.Spec.Predictor.Model.Runtime, predContainer.Name),
		})
		return podSpec, errors.Wrapf(err, ErrInvalidPlaceholder, *isvc.Spec.Predictor.Model.Runtime, predContainer.Name)
	}

	// Update image tag if GPU is enabled or runtime version is provided
	isvcutils.UpdateImageTag(predContainer, isvc.Spec.Predictor.Model.RuntimeVersion, isvc.Spec.Predictor.Model.Runtime)

	podSpec = *mergedPodSpec
	podSpec.Containers = []corev1.Container{*predContainer}

	containerIndexInSR := isvcutils.GetContainerIndexByName(sRuntime.Containers, constants.TransformerContainerName)
	containerIndexInIS := isvcutils.GetContainerIndexByName(isvc.Spec.Predictor.Containers, constants.TransformerContainerName)

	var transformerContainer *corev1.Container
	switch {
	case containerIndexInSR != -1 && containerIndexInIS != -1:
		// Merge transformer container from ServingRuntime and InferenceService
		transformerContainer, err = isvcutils.MergeRuntimeContainers(&sRuntime.Containers[containerIndexInSR], &isvc.Spec.Predictor.Containers[containerIndexInIS])
		if err != nil {
			return podSpec, err
		}
	case containerIndexInSR != -1:
		transformerContainer = &sRuntime.Containers[containerIndexInSR]
	case containerIndexInIS != -1:
		transformerContainer = &isvc.Spec.Predictor.Containers[containerIndexInIS]
	}

	if transformerContainer != nil {
		// Replace placeholders in transformer container by values from inferenceservice metadata
		if err = isvcutils.ReplacePlaceholders(transformerContainer, isvc.ObjectMeta); err != nil {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.InvalidPredictorSpec,
				Message: fmt.Sprintf(ErrInvalidPlaceholder, *isvc.Spec.Predictor.Model.Runtime, transformerContainer.Name),
			})
			return podSpec, errors.Wrapf(err, ErrInvalidPlaceholder, *isvc.Spec.Predictor.Model.Runtime, transformerContainer.Name)
		}
		podSpec.Containers = append(podSpec.Containers, *transformerContainer)
	}

	// Append all containers except the predictor and transformer containers from ServingRuntime
	for _, container := range sRuntime.Containers {
		if container.Name != constants.InferenceServiceContainerName && container.Name != constants.TransformerContainerName {
			podSpec.Containers = append(podSpec.Containers, container)
		}
	}

	// Append all containers except the transformer container from InferenceService
	for _, container := range isvc.Spec.Predictor.Containers {
		if container.Name != constants.TransformerContainerName {
			podSpec.Containers = append(podSpec.Containers, container)
		}
	}

	return podSpec, nil
}

func (p *Predictor) getPredictorName(ctx context.Context, isvc *v1beta1.InferenceService) string {
	predictorName := constants.PredictorServiceName(isvc.Name)
	if p.deploymentMode == constants.RawDeployment {
		existing := &corev1.Service{}
		err := p.client.Get(ctx, types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		}
	} else {
		existing := &knservingv1.Service{}
		err := p.client.Get(ctx, types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		}
	}
	return predictorName
}

func (p *Predictor) buildObjectMeta(isvc *v1beta1.InferenceService, predictorName string, sRuntimeLabels, predictorLabels, sRuntimeAnnotations, annotations, predictorAnnotations map[string]string) metav1.ObjectMeta {
	// Labels and annotations priority: predictor component > isvc > ServingRuntimePodSpec
	// Labels and annotations from high priority will overwrite that from low priority
	return metav1.ObjectMeta{
		Name:      predictorName,
		Namespace: isvc.Namespace,
		Labels: utils.Union(
			sRuntimeLabels,
			isvc.Labels,
			predictorLabels,
			map[string]string{
				constants.InferenceServicePodLabelKey: isvc.Name,
				constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
			},
		),
		Annotations: utils.Union(
			sRuntimeAnnotations,
			annotations,
			predictorAnnotations,
		),
	}
}

func (p *Predictor) reconcileWorker(sRuntime v1alpha1.ServingRuntimeSpec, isvc *v1beta1.InferenceService, podSpec *corev1.PodSpec, annotations, predictorAnnotations map[string]string, isvcGeneration string) (metav1.ObjectMeta, *corev1.PodSpec, error) {
	var workerObjectMeta metav1.ObjectMeta
	var workerPodSpec *corev1.PodSpec
	var err error

	sRuntimeWorkerAnnotations := sRuntime.WorkerSpec.Annotations
	sRuntimeWorkerLabels := sRuntime.WorkerSpec.ServingRuntimePodSpec.Labels

	if workerPodSpec, err = multiNodeProcess(sRuntime, isvc, podSpec, annotations, isvcGeneration); err != nil {
		return workerObjectMeta, workerPodSpec, err
	}

	workerObjectMeta = metav1.ObjectMeta{
		Name:      constants.PredictorWorkerServiceName(isvc.Name),
		Namespace: isvc.Namespace,
		Labels: utils.Union(
			sRuntimeWorkerLabels,
			isvc.Labels,
			isvc.Spec.Predictor.Labels,
			map[string]string{
				constants.InferenceServiceGenerationPodLabelKey: isvcGeneration,
				constants.InferenceServicePodLabelKey:           isvc.Name,
				constants.KServiceComponentLabel:                string(v1beta1.PredictorComponent),
			},
		),
		Annotations: utils.Union(
			sRuntimeWorkerAnnotations,
			annotations,
			predictorAnnotations,
		),
	}

	return workerObjectMeta, workerPodSpec, nil
}

func multiNodeProcess(sRuntime v1alpha1.ServingRuntimeSpec, isvc *v1beta1.InferenceService, podSpec *corev1.PodSpec, annotations map[string]string, isvcGeneration string) (*corev1.PodSpec, error) {
	var workerContainer *corev1.Container
	var mergedWorkerPodSpec *corev1.PodSpec
	var err error

	// Initialize PipelineParallelSize and TensorParallelSize if not set
	if sRuntime.WorkerSpec.PipelineParallelSize == nil {
		sRuntime.WorkerSpec.PipelineParallelSize = ptr.To(constants.DefaultPipelineParallelSize)
	}
	if sRuntime.WorkerSpec.TensorParallelSize == nil {
		sRuntime.WorkerSpec.TensorParallelSize = ptr.To(constants.DefaultTensorParallelSize)
	}

	// Set the PipelineParallelSize from InferenceService to ServingRuntime workerSpec.PipelineParallelSize
	if isvc.Spec.Predictor.WorkerSpec.PipelineParallelSize != nil {
		sRuntime.WorkerSpec.PipelineParallelSize = isvc.Spec.Predictor.WorkerSpec.PipelineParallelSize
	}
	// Set the TensorParallelSize from InferenceService to ServingRuntime workerSpec.TensorParallelSize
	if isvc.Spec.Predictor.WorkerSpec.TensorParallelSize != nil {
		sRuntime.WorkerSpec.TensorParallelSize = isvc.Spec.Predictor.WorkerSpec.TensorParallelSize
	}

	if sRuntime.WorkerSpec == nil {
		errMsg := "you cannot set WorkerSpec in the InferenceService if the ServingRuntime does not have a WorkerSpec"
		isvc.Status.PropagateRawStatusWithMessages(v1beta1.PredictorComponent, v1beta1.InvalidWorkerSpecNotSet, errMsg, corev1.ConditionFalse)
		return nil, errors.New(errMsg)
	}
	// Check if workerSpec in ServingRuntime does not have worker containers information, it should return errors
	if len(sRuntime.WorkerSpec.Containers) == 0 {
		errMsg := "No workerSpec container configuration found in selected serving runtime"
		isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
			Reason:  v1beta1.InvalidPredictorSpec,
			Message: errMsg,
		})
		return nil, errors.New(errMsg)
	}

	targetisvcContainer := corev1.Container{}
	if isvc.Spec.Predictor.WorkerSpec.Containers != nil {
		targetisvcContainer = isvc.Spec.Predictor.WorkerSpec.Containers[0]
	}
	_, workerContainer, mergedWorkerPodSpec, err = isvcutils.MergeServingRuntimeAndInferenceServiceSpecs(sRuntime.WorkerSpec.Containers, targetisvcContainer, isvc, constants.WorkerContainerName, sRuntime.WorkerSpec.ServingRuntimePodSpec, isvc.Spec.Predictor.WorkerSpec.PodSpec)
	if err != nil {
		return nil, err
	}

	mergedWorkerPodSpec.Containers = []corev1.Container{
		*workerContainer,
	}

	// Calculate the total number of GPUs required for the request based on the tensor parallel size and pipeline parallel size specified in the worker spec.
	// totalRequestGPUCount is the product of TensorParallelSize and PipelineParallelSize,
	// which represents the total number of GPUs needed for distributed computation.

	totalRequestGPUCount := *sRuntime.WorkerSpec.TensorParallelSize * *sRuntime.WorkerSpec.PipelineParallelSize

	rayNodeCount, workerNodeGPUCount, headNodeGPUCount, err := computeRayNodeAndGPUs(mergedWorkerPodSpec, totalRequestGPUCount, podSpec)
	if err != nil {
		return nil, err
	}

	// Add required environment variables: PipelineParallelSize, TensorParallelSize
	// Deployment node deployement
	if err := isvcutils.AddEnvVarToPodSpec(podSpec, constants.InferenceServiceContainerName, constants.PipelineParallelSizeEnvName, strconv.Itoa(*sRuntime.WorkerSpec.PipelineParallelSize)); err != nil {
		return nil, errors.Wrapf(err, "failed to add %s environment to the container(%s)", constants.PipelineParallelSizeEnvName, constants.InferenceServiceContainerName)
	}
	if err := isvcutils.AddEnvVarToPodSpec(podSpec, constants.InferenceServiceContainerName, constants.TensorParallelSizeEnvName, strconv.Itoa(*sRuntime.WorkerSpec.TensorParallelSize)); err != nil {
		return nil, errors.Wrapf(err, "failed to add %s environment to the container(%s)", constants.TensorParallelSizeEnvName, constants.InferenceServiceContainerName)
	}
	if err := isvcutils.AddEnvVarToPodSpec(podSpec, constants.InferenceServiceContainerName, constants.RayNodeCountEnvName, strconv.Itoa(rayNodeCount)); err != nil {
		return nil, errors.Wrapf(err, "failed to add %s environment to the container(%s)", constants.RayNodeCountEnvName, constants.InferenceServiceContainerName)
	}
	if err := isvcutils.AddEnvVarToPodSpec(podSpec, constants.InferenceServiceContainerName, constants.RequestGPUCountEnvName, strconv.Itoa(headNodeGPUCount)); err != nil {
		return nil, errors.Wrapf(err, "failed to add %s environment to the container(%s)", constants.RequestGPUCountEnvName, constants.InferenceServiceContainerName)
	}

	// Set the environment variable for "isvc name" to the MODEL_NAME when multiNodeEnabled is true.
	if err := isvcutils.AddEnvVarToPodSpec(podSpec, constants.InferenceServiceContainerName, "MODEL_NAME", isvc.Name); err != nil {
		return nil, errors.Wrapf(err, "failed to add MODEL_NAME environment to the container(%s)", constants.InferenceServiceContainerName)
	}

	deploymentAnnotations := annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	storageProtocol := strings.Split(deploymentAnnotations, "://")[0]
	if storageProtocol == "pvc" {
		// Set the environment variable for "/mnt/models" to the MODEL_DIR when multiNodeEnabled is true.
		if err := isvcutils.AddEnvVarToPodSpec(podSpec, constants.InferenceServiceContainerName, "MODEL_DIR", constants.DefaultModelLocalMountPath); err != nil {
			return nil, errors.Wrapf(err, "failed to add MODEL_DIR environment to the container(%s)", constants.DefaultModelLocalMountPath)
		}
	}
	// Worker node deployement
	if err := isvcutils.AddEnvVarToPodSpec(mergedWorkerPodSpec, constants.WorkerContainerName, constants.RayNodeCountEnvName, strconv.Itoa(rayNodeCount)); err != nil {
		return nil, errors.Wrapf(err, "failed to add %s environment to the container(%s)", constants.RayNodeCountEnvName, constants.WorkerContainerName)
	}
	if err := isvcutils.AddEnvVarToPodSpec(mergedWorkerPodSpec, constants.WorkerContainerName, constants.RequestGPUCountEnvName, strconv.Itoa(workerNodeGPUCount)); err != nil {
		return nil, errors.Wrapf(err, "failed to add %s environment to the container(%s)", constants.RequestGPUCountEnvName, constants.WorkerContainerName)
	}
	// Set the environment variable for "isvc name" to the ISVC_NAME when multiNodeEnabled is true.
	if err := isvcutils.AddEnvVarToPodSpec(mergedWorkerPodSpec, constants.WorkerContainerName, "ISVC_NAME", isvc.Name); err != nil {
		return nil, errors.Wrapf(err, "failed to add ISVC_NAME environment to the container(%s)", constants.WorkerContainerName)
	}
	// Set the environment variable for "isvc name" to the HEAD_SVC when multiNodeEnabled is true.
	if err := isvcutils.AddEnvVarToPodSpec(mergedWorkerPodSpec, constants.WorkerContainerName, "HEAD_SVC", constants.GetHeadServiceName(isvc.Name, isvcGeneration)); err != nil {
		return nil, errors.Wrapf(err, "failed to add HEAD_SVC environment to the container(%s)", constants.WorkerContainerName)
	}
	return mergedWorkerPodSpec, nil
}

// The `rayNodeCount` is determined based on the requested GPU count.
// We use the GPU resource defined in `workerSpec` to calculate the required GPU count.
// The `rayNodeCount` is set to the ceiling value of (total requested GPU count / GPUs per worker node).
// The head node GPU count is determined by subtracting (workerSpec GPU resource * (rayNodeCount - 1)) from the total GPU count.
// If the head node has a predefined GPU count, it takes precedence.
// However, if the total required GPU count is less than the sum of (head node GPU count + workerSpec GPU resource * rayNodeCount), an error is raised.
func computeRayNodeAndGPUs(mergedWorkerPodSpec *corev1.PodSpec, totalRequestGPUCount int, podSpec *corev1.PodSpec) (int, int, int, error) {
	getGPUResourceQty := func(spec *corev1.PodSpec) int {
		if _, gpuResourceQuantity, exist := utils.GetGPUResourceQtyByType(&spec.Containers[0].Resources, "Request"); exist {
			return int(gpuResourceQuantity.Value())
		}
		return 0
	}

	computeRayNodes := func(totalGPUs, workerGPUs int) int {
		if workerGPUs == 0 {
			return 1
		}
		return int(math.Ceil(float64(totalGPUs) / float64(workerGPUs)))
	}

	validateGPUAllocation := func(rayNodeCount, headNodeGPUCount, workerNodeGPUCount int) error {
		if totalRequestGPUCount > headNodeGPUCount+(workerNodeGPUCount*(rayNodeCount-1)) {
			return fmt.Errorf(ErrRayClusterInsufficientGPUs, totalRequestGPUCount, headNodeGPUCount, workerNodeGPUCount*(rayNodeCount-1))
		}
		return nil
	}

	var rayNodeCount int
	headNodeGPUCount := getGPUResourceQty(podSpec)
	workerNodeGPUCount := getGPUResourceQty(mergedWorkerPodSpec)

	// Case 1 & 2: At least worker GPUs are set
	if workerNodeGPUCount > 0 {
		rayNodeCount = computeRayNodes(totalRequestGPUCount, workerNodeGPUCount)
		newHeadNodeGPUCount := totalRequestGPUCount - workerNodeGPUCount*(rayNodeCount-1)

		if headNodeGPUCount > 0 {
			rayNodeCountByHeadGpuCount := computeRayNodes(totalRequestGPUCount, headNodeGPUCount)
			if rayNodeCountByHeadGpuCount == 1 {
				rayNodeCount = 1
			}
		} else if headNodeGPUCount == 0 {
			headNodeGPUCount = newHeadNodeGPUCount
		}

		// Use only head node with total request gpu count if it can satisfy the GPU requirement
		if rayNodeCount == 1 {
			workerNodeGPUCount = 0
			headNodeGPUCount = totalRequestGPUCount
		}

		if err := validateGPUAllocation(rayNodeCount, headNodeGPUCount, workerNodeGPUCount); err != nil {
			return 0, 0, 0, err
		}
		return rayNodeCount, workerNodeGPUCount, headNodeGPUCount, nil
	}

	// Case 3: Only head GPU is set
	if headNodeGPUCount > 0 {
		remainingGPUs := totalRequestGPUCount - headNodeGPUCount
		rayNodeCount = 1
		workerNodeGPUCount = 1

		if remainingGPUs > 0 {
			rayNodeCount = computeRayNodes(remainingGPUs, workerNodeGPUCount) + 1 // Single GPU worker nodes
		}

		if err := validateGPUAllocation(rayNodeCount, headNodeGPUCount, workerNodeGPUCount); err != nil {
			return 0, 0, 0, err
		}
		return rayNodeCount, workerNodeGPUCount, headNodeGPUCount, nil
	}

	// Case 4: No GPUs found → Default values
	return totalRequestGPUCount, 1, 1, nil
}

func (p *Predictor) reconcileRawDeployment(ctx context.Context, isvc *v1beta1.InferenceService, objectMeta, workerObjectMeta metav1.ObjectMeta, podSpec, workerPodSpec *corev1.PodSpec) error {
	r, err := raw.NewRawKubeReconciler(ctx, p.client, p.clientset, p.scheme, constants.InferenceServiceResource, objectMeta, workerObjectMeta, &isvc.Spec.Predictor.ComponentExtensionSpec,
		podSpec, workerPodSpec)
	if err != nil {
		return errors.Wrapf(err, "fails to create NewRawKubeReconciler for predictor")
	}

	// set Deployment Controller
	for _, deployment := range r.Deployment.DeploymentList {
		if err := controllerutil.SetControllerReference(isvc, deployment, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set deployment owner reference for predictor")
		}
	}
	for _, svc := range r.Service.ServiceList {
		// set Service Controller
		if err := controllerutil.SetControllerReference(isvc, svc, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set service owner reference for predictor")
		}
	}
	// set Otel Controller
	if r.OtelCollector != nil {
		if err := r.OtelCollector.SetControllerReferences(isvc, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set otel owner references for predictor")
		}
	}
	// set autoscaler Controller
	if err := r.Scaler.Autoscaler.SetControllerReferences(isvc, p.scheme); err != nil {
		return errors.Wrapf(err, "fails to set autoscaler owner references for predictor")
	}

	deploymentList, err := r.Reconcile(ctx)
	if err != nil {
		return errors.Wrapf(err, "fails to reconcile predictor")
	}

	isvc.Status.PropagateRawStatus(v1beta1.PredictorComponent, deploymentList, r.URL)
	return nil
}

func (p *Predictor) reconcileKnativeDeployment(ctx context.Context, isvc *v1beta1.InferenceService, objectMeta *metav1.ObjectMeta, podSpec *corev1.PodSpec) (*knservingv1.ServiceStatus, error) {
	r, err := knative.NewKsvcReconciler(ctx, p.client, p.clientset, p.scheme, *objectMeta, &isvc.Spec.Predictor.ComponentExtensionSpec,
		podSpec, isvc.Status.Components[v1beta1.PredictorComponent], p.inferenceServiceConfig.ServiceLabelDisallowedList)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to create new knative service reconciler for predictor")
	}
	if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
		return nil, errors.Wrapf(err, "fails to set owner reference for predictor")
	}
	kstatus, err := r.Reconcile(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to reconcile predictor")
	}
	if !isvc.GetForceStopRuntime() {
		isvc.Status.PropagateStatus(v1beta1.PredictorComponent, kstatus)
	}
	return kstatus, nil
}
