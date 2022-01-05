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
	"github.com/go-logr/logr"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
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

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

var _ Component = &Explainer{}

// Explainer reconciles resources for this component.
type Explainer struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
	Log                    logr.Logger
}

func NewExplainer(client client.Client, scheme *runtime.Scheme, inferenceServiceConfig *v1beta1.InferenceServicesConfig) Component {
	return &Explainer{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		Log:                    ctrl.Log.WithName("ExplainerReconciler"),
	}
}

// Reconcile observes the explainer and attempts to drive the status towards the desired state.
func (e *Explainer) Reconcile(isvc *v1beta1.InferenceService) error {
	e.Log.Info("Reconciling Explainer", "ExplainerSpec", isvc.Spec.Explainer)
	explainer := isvc.Spec.Explainer.GetImplementation()
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := explainer.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}
	addLoggerAnnotations(isvc.Spec.Explainer.Logger, annotations)
	// Add StorageSpec annotations so mutator will mount storage credentials to InferenceService's explainer
	addStorageSpecAnnotations(explainer.GetStorageSpec(), annotations)
	objectMeta := metav1.ObjectMeta{
		Name:      constants.DefaultExplainerServiceName(isvc.Name),
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(v1beta1.ExplainerComponent),
		}),
		Annotations: annotations,
	}
	container := explainer.GetContainer(isvc.ObjectMeta, isvc.Spec.Explainer.GetExtensions(), e.inferenceServiceConfig)
	if len(isvc.Spec.Explainer.PodSpec.Containers) == 0 {
		isvc.Spec.Explainer.PodSpec.Containers = []v1.Container{
			*container,
		}
	} else {
		isvc.Spec.Explainer.PodSpec.Containers[0] = *container
	}

	podSpec := v1.PodSpec(isvc.Spec.Explainer.PodSpec)
	deployConfig, err := v1beta1.NewDeployConfig(e.client)
	if err != nil {
		return err
	}

	// Here we allow switch between knative and vanilla deployment
	if isvcutils.GetDeploymentMode(annotations, deployConfig) == constants.RawDeployment {
		r, err := raw.NewRawKubeReconciler(e.client, e.scheme, objectMeta, &isvc.Spec.Explainer.ComponentExtensionSpec,
			&podSpec)
		if err != nil {
			return errors.Wrapf(err, "fails to create NewRawKubeReconciler for explainer")
		}
		//set Deployment Controller
		if err := controllerutil.SetControllerReference(isvc, r.Deployment.Deployment, e.scheme); err != nil {
			return errors.Wrapf(err, "fails to set deployment owner reference for explainer")
		}
		//set Service Controller
		if err := controllerutil.SetControllerReference(isvc, r.Service.Service, e.scheme); err != nil {
			return errors.Wrapf(err, "fails to set service owner reference for explainer")
		}
		//set autoscaler Controller
		if r.Scaler.Autoscaler.AutoscalerClass == constants.AutoscalerClassHPA {
			if err := controllerutil.SetControllerReference(isvc, r.Scaler.Autoscaler.HPA.HPA, e.scheme); err != nil {
				return errors.Wrapf(err, "fails to set HPA owner reference for explainer")
			}
		}

		deployment, err := r.Reconcile()
		if err != nil {
			return errors.Wrapf(err, "fails to reconcile explainer")
		}
		isvc.Status.PropagateRawStatus(v1beta1.ExplainerComponent, deployment, r.URL)
	} else {
		r := knative.NewKsvcReconciler(e.client, e.scheme, objectMeta, &isvc.Spec.Explainer.ComponentExtensionSpec,
			&podSpec, isvc.Status.Components[v1beta1.ExplainerComponent])

		if err := controllerutil.SetControllerReference(isvc, r.Service, e.scheme); err != nil {
			return errors.Wrapf(err, "fails to set owner reference for explainer")
		}
		status, err := r.Reconcile()
		if err != nil {
			return errors.Wrapf(err, "fails to reconcile explainer")
		}
		isvc.Status.PropagateStatus(v1beta1.ExplainerComponent, status)
	}
	return nil
}
