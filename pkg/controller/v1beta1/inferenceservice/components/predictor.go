/*
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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	modelconfig "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/modelconfig"
	raw "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
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
	var container *v1.Container
	var podSpec v1.PodSpec

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})

	hasInferenceLogging := addLoggerAnnotations(isvc.Spec.Predictor.Logger, annotations)
	hasInferenceBatcher := addBatcherAnnotations(isvc.Spec.Predictor.Batcher, annotations)

	// Add agent annotations so mutator will mount model agent to multi-model InferenceService's predictor
	addAgentAnnotations(isvc, annotations, p.inferenceServiceConfig)

	// Reconcile modelConfig
	configMapReconciler := modelconfig.NewModelConfigReconciler(p.client, p.scheme)
	if err := configMapReconciler.Reconcile(isvc); err != nil {
		return err
	}

	// If Model is specified, prioritize using that. Otherwise, we will assume a framework object was specified.
	if isvc.Spec.Predictor.Model != nil {
		var sRuntime v1alpha1.ServingRuntimeSpec
		var err error

		if isvc.Spec.Predictor.Model.Runtime != nil {
			r, err := isvcutils.GetServingRuntime(p.client, *isvc.Spec.Predictor.Model.Runtime, isvc.Namespace)
			if err != nil {
				return err
			}

			if r.IsDisabled() {
				return fmt.Errorf("specified runtime %s is disabled", *isvc.Spec.Predictor.Model.Runtime)
			}

			// Verify that the selected runtime supports the specified framework.
			if !isvc.Spec.Predictor.Model.RuntimeSupportsModel(*isvc.Spec.Predictor.Model.Runtime, r) {
				return fmt.Errorf("specified runtime %s does not support specified framework/version", *isvc.Spec.Predictor.Model.Runtime)
			}

			sRuntime = *r
		} else {
			runtimes, err := isvc.Spec.Predictor.Model.GetSupportingRuntimes(p.client, isvc.Namespace, false)
			if err != nil {
				return err
			}
			if len(runtimes) == 0 {
				return fmt.Errorf("no runtime found to support predictor with model type: %v", isvc.Spec.Predictor.Model.Framework)
			}
			// Get first supporting runtime.
			sRuntime = runtimes[0]
		}
		if len(sRuntime.Containers) == 0 {
			return errors.New("no container configuration found in selected serving runtime")
		}
		// Assume only one container is specified in runtime spec.
		container, err = isvcutils.MergeRuntimeContainers(&sRuntime.Containers[0], &isvc.Spec.Predictor.Model.Container)
		if err != nil {
			return errors.Wrapf(err, "failed to get runtime container")
		}
		if sourceURI := isvc.Spec.Predictor.Model.GetStorageUri(); sourceURI != nil {
			annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
		}

		mergedPodSpec, err := isvcutils.MergePodSpec(&sRuntime.ServingRuntimePodSpec, &isvc.Spec.Predictor.PodSpec)
		if err != nil {
			return errors.Wrapf(err, "failed to consolidate serving runtime PodSpecs")
		}

		// Other dependencies rely on the container to be a specific name.
		container.Name = constants.InferenceServiceContainerName

		// Replace placeholders in runtime container by values from inferenceservice metadata
		err = isvcutils.ReplacePlaceholders(container, isvc.ObjectMeta)
		if err != nil {
			return errors.Wrapf(err, "failed to replace placeholders in serving runtime Container")
		}

		// Update image tag if GPU is enabled or runtime version is provided
		isvcutils.UpdateImageTag(container, isvc.Spec.Predictor.Model.RuntimeVersion, p.inferenceServiceConfig)

		podSpec = *mergedPodSpec
		podSpec.Containers = []v1.Container{
			*container,
		}

	} else {
		predictor := isvc.Spec.Predictor.GetImplementation()
		container = predictor.GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), p.inferenceServiceConfig)
		// Knative does not support INIT containers or mounting, so we add annotations that trigger the
		// StorageInitializer injector to mutate the underlying deployment to provision model data
		if sourceURI := predictor.GetStorageUri(); sourceURI != nil {
			annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
		}

		podSpec = v1.PodSpec(isvc.Spec.Predictor.PodSpec)
		if len(podSpec.Containers) == 0 {
			podSpec.Containers = []v1.Container{
				*container,
			}
		} else {
			podSpec.Containers[0] = *container
		}

	}

	objectMeta := metav1.ObjectMeta{
		Name:      constants.DefaultPredictorServiceName(isvc.Name),
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
		}),
		Annotations: annotations,
	}

	p.Log.Info("Resolved container", "container", container, "podSpec", podSpec)

	//TODO now knative supports multi containers, consolidate logger/batcher/puller to the sidecar container
	//https://github.com/kserve/kserve/issues/973
	if hasInferenceLogging || hasInferenceBatcher {
		addAgentContainerPort(&podSpec.Containers[0])
	}

	deployConfig, err := v1beta1.NewDeployConfig(p.client)
	if err != nil {
		return err
	}

	// Here we allow switch between knative and vanilla deployment
	if isvcutils.GetDeploymentMode(annotations, deployConfig) == constants.RawDeployment {
		r, err := raw.NewRawKubeReconciler(p.client, p.scheme, objectMeta, &isvc.Spec.Predictor.ComponentExtensionSpec,
			&podSpec)
		if err != nil {
			return errors.Wrapf(err, "fails to create NewRawKubeReconciler for predictor")
		}
		//set Deployment Controller
		if err := controllerutil.SetControllerReference(isvc, r.Deployment.Deployment, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set deployment owner reference for predictor")
		}
		//set Service Controller
		if err := controllerutil.SetControllerReference(isvc, r.Service.Service, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set service owner reference for predictor")
		}
		//set autoscaler Controller
		if r.Scaler.Autoscaler.AutoscalerClass == constants.AutoscalerClassHPA {
			if err := controllerutil.SetControllerReference(isvc, r.Scaler.Autoscaler.HPA.HPA, p.scheme); err != nil {
				return errors.Wrapf(err, "fails to set HPA owner reference for predictor")
			}
		}

		deployment, err := r.Reconcile()
		if err != nil {
			return errors.Wrapf(err, "fails to reconcile predictor")
		}
		isvc.Status.PropagateRawStatus(v1beta1.PredictorComponent, deployment, r.URL)
	} else {
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
	}

	return nil
}
