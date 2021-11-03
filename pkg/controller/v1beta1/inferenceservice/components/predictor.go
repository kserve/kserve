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
	"encoding/json"

	"github.com/go-logr/logr"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	modelconfig "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/modelconfig"
	raw "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/servingruntimes"
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
		runtimes, err := servingruntimes.GetRuntimesSupportingModelType(p.client, isvc.Namespace, isvc.Spec.Predictor)
		if err != nil {
			return err
		}
		if len(runtimes) == 0 {
			return errors.New("No runtime found to support predictor")
		}
		// Get first supporting runtime.
		sRuntime := runtimes[0]
		if len(sRuntime.Containers) == 0 {
			return errors.New("No container configuration found in selected serving runtime")
		}
		// Assume only one container is specified in runtime spec.
		container, err = mergeRuntimeContainers(&sRuntime.Containers[0], &isvc.Spec.Predictor.Model.Container)
		if err != nil {
			return errors.Wrapf(err, "failed to get runtime container")
		}
		if sourceURI := isvc.Spec.Predictor.Model.GetStorageUri(); sourceURI != nil {
			annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
		}

		mergedPodSpec, err := mergePodSpec(&sRuntime.ServingRuntimePodSpec, &isvc.Spec.Predictor.PodSpec)
		if err != nil {
			return errors.Wrapf(err, "failed to consolidate serving runtime PodSpecs")
		}

		// Other dependencies rely on the container to be a specific name.
		container.Name = constants.InferenceServiceContainerName

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

		if len(isvc.Spec.Predictor.PodSpec.Containers) == 0 {
			isvc.Spec.Predictor.PodSpec.Containers = []v1.Container{
				*container,
			}
		} else {
			isvc.Spec.Predictor.PodSpec.Containers[0] = *container
		}

		podSpec = v1.PodSpec(isvc.Spec.Predictor.PodSpec)

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
		addAgentContainerPort(&isvc.Spec.Predictor.PodSpec.Containers[0])
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

// Merge the predictor Container struct with the runtime Container struct, allowing users
// to override runtime container settings from the predictor spec.
func mergeRuntimeContainers(runtimeContainer *v1alpha1.Container, predictorContainer *v1.Container) (*v1.Container, error) {
	// Default container configuration from the runtime.
	coreContainer := v1.Container{
		Args:            runtimeContainer.Args,
		Command:         runtimeContainer.Command,
		Env:             runtimeContainer.Env,
		Image:           runtimeContainer.Image,
		Name:            runtimeContainer.Name,
		Resources:       runtimeContainer.Resources,
		ImagePullPolicy: runtimeContainer.ImagePullPolicy,
		WorkingDir:      runtimeContainer.WorkingDir,
		LivenessProbe:   runtimeContainer.LivenessProbe,
	}
	// Save runtime container name, as the name can be overridden as empty string during the Unmarshal below
	// since the Name field does not have the 'omitempty' struct tag.
	runtimeContainerName := runtimeContainer.Name

	// Args and Env will be combined instead of overridden.
	argCopy := make([]string, len(coreContainer.Args))
	copy(argCopy, coreContainer.Args)

	envCopy := make([]v1.EnvVar, len(coreContainer.Env))
	copy(envCopy, coreContainer.Env)

	// Use JSON Marshal/Unmarshal to merge Container structs.
	overrides, err := json.Marshal(predictorContainer)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(overrides, &coreContainer); err != nil {
		return nil, err
	}

	if coreContainer.Name == "" {
		coreContainer.Name = runtimeContainerName
	}

	argCopy = append(argCopy, predictorContainer.Args...)
	envCopy = append(envCopy, predictorContainer.Env...)

	coreContainer.Args = argCopy
	coreContainer.Env = envCopy

	return &coreContainer, nil

}

// Merge the predictor PodSpec struct with the runtime PodSpec struct, allowing users
// to override runtime PodSpec settings from the predictor spec.
func mergePodSpec(runtimePodSpec *v1alpha1.ServingRuntimePodSpec, predictorPodSpec *v1beta1.PodSpec) (*v1.PodSpec, error) {

	corePodSpec := v1.PodSpec{
		NodeSelector: runtimePodSpec.NodeSelector,
		Affinity:     runtimePodSpec.Affinity,
		Tolerations:  runtimePodSpec.Tolerations,
	}

	// Use JSON Marshal/Unmarshal to merge PodSpec structs.
	overrides, err := json.Marshal(predictorPodSpec)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(overrides, &corePodSpec); err != nil {
		return nil, err
	}

	return &corePodSpec, nil

}
