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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/utils"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

var _ Component = &Explainer{}

// Explainer reconciles resources for this component.
type Explainer struct {
	client                 client.Client
	clientset              kubernetes.Interface
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder //nolint: unused
	deploymentMode         constants.DeploymentModeType
	Log                    logr.Logger
}

func NewExplainer(client client.Client, clientset kubernetes.Interface, scheme *runtime.Scheme,
	inferenceServiceConfig *v1beta1.InferenceServicesConfig, deploymentMode constants.DeploymentModeType) Component {
	return &Explainer{
		client:                 client,
		clientset:              clientset,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		deploymentMode:         deploymentMode,
		Log:                    ctrl.Log.WithName("ExplainerReconciler"),
	}
}

// Reconcile observes the explainer and attempts to drive the status towards the desired state.
func (e *Explainer) Reconcile(isvc *v1beta1.InferenceService) (ctrl.Result, error) {
	var container *v1.Container
	var podSpec v1.PodSpec
	var sRuntimeLabels map[string]string
	var sRuntimeAnnotations map[string]string

	e.Log.Info("Reconciling Explainer", "ExplainerSpec", isvc.Spec.Explainer)
	explainer := isvc.Spec.Explainer.GetImplementation()
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := explainer.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
		err := isvcutils.ValidateStorageURI(sourceURI, e.client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("StorageURI not supported: %w", err)
		}
	}
	addLoggerAnnotations(isvc.Spec.Explainer.Logger, annotations)

	explainerName := constants.ExplainerServiceName(isvc.Name)
	predictorName := constants.PredictorServiceName(isvc.Name)

	//If Model is specified, prioritize using that. Otherwise, we will assume a framework object was specified.
	if isvc.Spec.Explainer.Model != nil {
		var sRuntime v1alpha1.ServingRuntimeSpec
		var err error

		if isvc.Spec.Explainer.Model.Runtime != nil {
			e.Log.Info("Reconciling Explainer", "ExplainerRuntime", isvc.Spec.Explainer.Model.Runtime)
			// set runtime defaults
			isvc.SetRuntimeDefaults()
			r, err := isvcutils.GetServingRuntime(e.client, *isvc.Spec.Explainer.Model.Runtime, isvc.Namespace)
			if err != nil {
				isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
					Reason:  v1beta1.RuntimeNotRecognized,
					Message: "Waiting for runtime to become available",
				})
				return ctrl.Result{}, err
			}

			if isvc.Spec.Explainer.Model.ProtocolVersion != nil &&
			!r.IsProtocolVersionSupported(*isvc.Spec.Explainer.Model.ProtocolVersion) {
				isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
					Reason:  v1beta1.NoSupportingRuntime,
					Message: "Specified runtime does not support specified protocol version",
				})
			return ctrl.Result{}, fmt.Errorf("specified runtime %s does not support specified protocol version", *isvc.Spec.Explainer.Model.Runtime)
			}

			// Verify that the selected runtime supports the specified framework.
			if !isvc.Spec.Explainer.Model.RuntimeSupportsModel(r) {
				isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
					Reason:  v1beta1.NoSupportingRuntime,
					Message: "Specified runtime does not support specified framework/version",
				})
				return ctrl.Result{}, fmt.Errorf("specified runtime %s does not support specified framework/version", *isvc.Spec.Explainer.Model.Runtime)
			}

			sRuntime = *r
		} else {
			e.Log.Info("Reconciling Explainer", "ExplainerRuntime", "Runtime not specified")
			runtimes, err := isvc.Spec.Explainer.Model.GetSupportingRuntimes(e.client, isvc.Namespace, false)
			if err != nil {
				return ctrl.Result{}, err
			}
			if len(runtimes) == 0 {
				isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
					Reason:  v1beta1.NoSupportingRuntime,
					Message: "No runtime found to support specified framework/version",
				})
				return ctrl.Result{}, fmt.Errorf("no runtime found to support explainer with model type: %v", isvc.Spec.Explainer.Model.ModelFormat)
			}
			// Get first supporting runtime
			sRuntime = runtimes[0].Spec
			isvc.Spec.Explainer.Model.Runtime = &runtimes[0].Name

			// set runtime defaults
			isvc.SetRuntimeDefaults()
		}
		// assign protocol version to inferenceservice based on runtime selected
		if isvc.Spec.Explainer.Model.ProtocolVersion == nil {
			protocolVersion := constants.GetProtocolVersionString(
				constants.ProtocolVersion(
					v1beta1.GetProtocolVersionPriority(sRuntime.ProtocolVersions),
				),
			)
			isvc.Spec.Explainer.Model.ProtocolVersion = &protocolVersion
		}

		if len(sRuntime.Containers) == 0 {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.InvalidExplainerSpec,
				Message: "No container configuration found in selected serving runtime",
			})
			return ctrl.Result{}, errors.New("no container configuration found in selected serving runtime")
		}

		kserveContainerIdx := -1
		for i := range sRuntime.Containers {
			if sRuntime.Containers[i].Name == constants.InferenceServiceContainerName {
				kserveContainerIdx = i
				break
			}
		}
		if kserveContainerIdx == -1 {
			return ctrl.Result{}, errors.New("failed to find kserve-container in ServingRuntime containers")
		}

		container, err = isvcutils.MergeRuntimeContainers(&sRuntime.Containers[kserveContainerIdx], &isvc.Spec.Explainer.Model.Container)
		if err != nil {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.InvalidExplainerSpec,
				Message: "Failed to get runtime container",
			})
			return ctrl.Result{}, errors.Wrapf(err, "failed to get runtime container")
		}

		mergedPodSpec, err := isvcutils.MergePodSpec(&sRuntime.ServingRuntimePodSpec, &isvc.Spec.Explainer.PodSpec)
		if err != nil {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.InvalidExplainerSpec,
				Message: "Failed to consolidate serving runtime PodSpecs",
			})
			return ctrl.Result{}, errors.Wrapf(err, "failed to consolidate serving runtime PodSpecs")
		}

		// Replace placeholders in runtime container by values from inferenceservice metadata
		if err = isvcutils.ReplacePlaceholders(container, isvc.ObjectMeta); err != nil {
			isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
				Reason:  v1beta1.InvalidExplainerSpec,
				Message: "Failed to replace placeholders in serving runtime Container",
			})
			return ctrl.Result{}, errors.Wrapf(err, "failed to replace placeholders in serving runtime Container")
		}

		// Update image tag if GPU is enabled or runtime version is provided
		isvcutils.UpdateImageTag(container, isvc.Spec.Explainer.Model.RuntimeVersion, isvc.Spec.Explainer.Model.Runtime)

		podSpec = *mergedPodSpec
		podSpec.Containers = []v1.Container{
			*container,
		}
		podSpec.Containers = append(podSpec.Containers, sRuntime.Containers[:kserveContainerIdx]...)
		podSpec.Containers = append(podSpec.Containers, sRuntime.Containers[kserveContainerIdx+1:]...)

		// Label filter will be handled in ksvc_reconciler
		sRuntimeLabels = sRuntime.ServingRuntimePodSpec.Labels
		sRuntimeAnnotations = utils.Filter(sRuntime.ServingRuntimePodSpec.Annotations, func(key string) bool {
			return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
		})

	} else {
		container = explainer.GetContainer(isvc.ObjectMeta, isvc.Spec.Explainer.GetExtensions(), e.inferenceServiceConfig, predictorName)

		podSpec = v1.PodSpec(isvc.Spec.Explainer.PodSpec)
		if len(podSpec.Containers) == 0 {
			podSpec.Containers = []v1.Container{
				*container,
			}
		} else {
			podSpec.Containers[0] = *container
		}

	}

	if e.deploymentMode == constants.RawDeployment {
		existing := &v1.Service{}
		err := e.client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultExplainerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			explainerName = constants.DefaultExplainerServiceName(isvc.Name)
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		}
	} else {
		existing := &knservingv1.Service{}
		err := e.client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultExplainerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			explainerName = constants.DefaultExplainerServiceName(isvc.Name)
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		}
	}

	// Labels and annotations from explainer component
	// Label filter will be handled in ksvc_reconciler
	explainerLabels := isvc.Spec.Explainer.Labels
	explainerAnnotations := utils.Filter(isvc.Spec.Explainer.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})

	// Labels and annotations priority: explainer component > isvc
	// Labels and annotations from high priority will overwrite that from low priority
	objectMeta := metav1.ObjectMeta{
		Name:      explainerName,
		Namespace: isvc.Namespace,
		Labels: utils.Union(
			sRuntimeLabels,
			isvc.Labels,
			explainerLabels,
			map[string]string{
				constants.InferenceServicePodLabelKey: isvc.Name,
				constants.KServiceComponentLabel:      string(v1beta1.ExplainerComponent),
			},
		),
		Annotations: utils.Union(
			sRuntimeAnnotations,
			annotations,
			explainerAnnotations,
		),
	}

	e.Log.Info("Resolved container", "Container", container.String(), "podSpec", podSpec.String())
	var rawDeployment bool
	var podLabelKey string
	var podLabelValue string

	// Here we allow switch between knative and vanilla deployment
	if e.deploymentMode == constants.RawDeployment {
		rawDeployment = true
		podLabelKey = constants.RawDeploymentAppLabel
		r, err := raw.NewRawKubeReconciler(e.client, e.clientset, e.scheme, objectMeta,
			&isvc.Spec.Explainer.ComponentExtensionSpec, &podSpec)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to create NewRawKubeReconciler for explainer")
		}
		// set Deployment Controller
		if err := controllerutil.SetControllerReference(isvc, r.Deployment.Deployment, e.scheme); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to set deployment owner reference for explainer")
		}
		// set Service Controller
		if err := controllerutil.SetControllerReference(isvc, r.Service.Service, e.scheme); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to set service owner reference for explainer")
		}
		// set autoscaler Controller
		if err := r.Scaler.Autoscaler.SetControllerReferences(isvc, e.scheme); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to set autoscaler owner references for explainer")
		}

		deployment, err := r.Reconcile()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile explainer")
		}
		isvc.Status.PropagateRawStatus(v1beta1.ExplainerComponent, deployment, r.URL)
	} else {
		podLabelKey = constants.RevisionLabel
		r := knative.NewKsvcReconciler(e.client, e.scheme, objectMeta, &isvc.Spec.Explainer.ComponentExtensionSpec,
			&podSpec, isvc.Status.Components[v1beta1.ExplainerComponent])

		if err := controllerutil.SetControllerReference(isvc, r.Service, e.scheme); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to set owner reference for explainer")
		}
		status, err := r.Reconcile()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile explainer")
		}
		isvc.Status.PropagateStatus(v1beta1.ExplainerComponent, status)
	}
	statusSpec := isvc.Status.Components[v1beta1.ExplainerComponent]
	if rawDeployment {
		podLabelValue = constants.GetRawServiceLabel(explainerName)
	} else {
		podLabelValue = statusSpec.LatestCreatedRevision
	}
	explainerPods, err := isvcutils.ListPodsByLabel(e.client, isvc.ObjectMeta.Namespace, podLabelKey, podLabelValue)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to list inferenceservice pods by label")
	}
	isvc.Status.PropagateModelStatus(statusSpec, explainerPods, rawDeployment)
	return ctrl.Result{}, nil
}
