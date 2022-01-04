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
	raw "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

var _ Component = &Transformer{}

// Transformer reconciles resources for this component.
type Transformer struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
	Log                    logr.Logger
}

func NewTransformer(client client.Client, scheme *runtime.Scheme, inferenceServiceConfig *v1beta1.InferenceServicesConfig) Component {
	return &Transformer{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		Log:                    ctrl.Log.WithName("TransformerReconciler"),
	}
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (p *Transformer) Reconcile(isvc *v1beta1.InferenceService) error {
	p.Log.Info("Reconciling Transformer", "TransformerSpec", isvc.Spec.Transformer)
	transformer := isvc.Spec.Transformer.GetImplementation()
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := transformer.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}
	hasInferenceLogging := addLoggerAnnotations(isvc.Spec.Transformer.Logger, annotations)
	hasInferenceBatcher := addBatcherAnnotations(isvc.Spec.Transformer.Batcher, annotations)

	objectMeta := metav1.ObjectMeta{
		Name:      constants.DefaultTransformerServiceName(isvc.Name),
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(v1beta1.TransformerComponent),
		}),
		Annotations: annotations,
	}
	if len(isvc.Spec.Transformer.PodSpec.Containers) == 0 {
		container := transformer.GetContainer(isvc.ObjectMeta, isvc.Spec.Transformer.GetExtensions(), p.inferenceServiceConfig)
		isvc.Spec.Transformer.PodSpec = v1beta1.PodSpec{
			Containers: []corev1.Container{
				*container,
			},
		}
	} else {
		container := transformer.GetContainer(isvc.ObjectMeta, isvc.Spec.Transformer.GetExtensions(), p.inferenceServiceConfig)
		isvc.Spec.Transformer.PodSpec.Containers[0] = *container
	}

	if hasInferenceLogging || hasInferenceBatcher {
		addAgentContainerPort(&isvc.Spec.Transformer.PodSpec.Containers[0])
	}
	podSpec := corev1.PodSpec(isvc.Spec.Transformer.PodSpec)

	deployConfig, err := v1beta1.NewDeployConfig(p.client)
	if err != nil {
		return err
	}
	// Here we allow switch between knative and vanilla deployment
	if isvcutils.GetDeploymentMode(annotations, deployConfig) == constants.RawDeployment {
		r, err := raw.NewRawKubeReconciler(p.client, p.scheme, objectMeta, &isvc.Spec.Transformer.ComponentExtensionSpec,
			&podSpec)
		if err != nil {
			return errors.Wrapf(err, "fails to create NewRawKubeReconciler for transformer")
		}
		//set Deployment Controller
		if err := controllerutil.SetControllerReference(isvc, r.Deployment.Deployment, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set deployment owner reference for transformer")
		}
		//set Service Controller
		if err := controllerutil.SetControllerReference(isvc, r.Service.Service, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set service owner reference for transformer")
		}
		//set autoscaler Controller
		if r.Scaler.Autoscaler.AutoscalerClass == constants.AutoscalerClassHPA {
			if err := controllerutil.SetControllerReference(isvc, r.Scaler.Autoscaler.HPA.HPA, p.scheme); err != nil {
				return errors.Wrapf(err, "fails to set HPA owner reference for transformer")
			}
		}

		deployment, err := r.Reconcile()
		if err != nil {
			return errors.Wrapf(err, "fails to reconcile transformer")
		}
		isvc.Status.PropagateRawStatus(v1beta1.TransformerComponent, deployment, r.URL)

	} else {
		r := knative.NewKsvcReconciler(p.client, p.scheme, objectMeta, &isvc.Spec.Transformer.ComponentExtensionSpec,
			&podSpec, isvc.Status.Components[v1beta1.TransformerComponent])
		if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set owner reference for predictor")
		}
		status, err := r.Reconcile()
		if err != nil {
			return errors.Wrapf(err, "fails to reconcile predictor")
		}
		isvc.Status.PropagateStatus(v1beta1.TransformerComponent, status)
	}

	return nil
}
