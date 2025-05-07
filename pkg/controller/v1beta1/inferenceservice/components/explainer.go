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
	e.Log.Info("Reconciling Explainer", "ExplainerSpec", isvc.Spec.Explainer)
	explainer := isvc.Spec.Explainer.GetImplementation()
	var annotations map[string]string
	if e.deploymentMode == constants.RawDeployment {
		annotations = utils.Filter(isvc.Annotations, func(key string) bool {
			// https://issues.redhat.com/browse/RHOAIENG-20326
			// For RawDeployment, we allow the security.opendatahub.io/enable-auth annotation
			return !utils.Includes(isvcutils.FilterList(e.inferenceServiceConfig.ServiceAnnotationDisallowedList, constants.ODHKserveRawAuth), key)
		})
	} else {
		annotations = utils.Filter(isvc.Annotations, func(key string) bool {
			return !utils.Includes(e.inferenceServiceConfig.ServiceAnnotationDisallowedList, key)
		})
	}

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
	// Label filter will be handled in ksvc_reconciler and raw reconciler
	explainerLabels := isvc.Spec.Explainer.Labels
	var explainerAnnotations map[string]string
	if e.deploymentMode == constants.RawDeployment {
		explainerAnnotations = utils.Filter(isvc.Spec.Explainer.Annotations, func(key string) bool {
			// https://issues.redhat.com/browse/RHOAIENG-20326
			// For RawDeployment, we allow the security.opendatahub.io/enable-auth annotation
			return !utils.Includes(isvcutils.FilterList(e.inferenceServiceConfig.ServiceAnnotationDisallowedList, constants.ODHKserveRawAuth), key)
		})
	} else {
		explainerAnnotations = utils.Filter(isvc.Spec.Explainer.Annotations, func(key string) bool {
			return !utils.Includes(e.inferenceServiceConfig.ServiceAnnotationDisallowedList, key)
		})
	}

	// Labels and annotations priority: explainer component > isvc
	// Labels and annotations from high priority will overwrite that from low priority
	objectMeta := metav1.ObjectMeta{
		Name:      explainerName,
		Namespace: isvc.Namespace,
		Labels: utils.Union(
			isvc.Labels,
			explainerLabels,
			map[string]string{
				constants.InferenceServicePodLabelKey: isvc.Name,
				constants.KServiceComponentLabel:      string(v1beta1.ExplainerComponent),
			},
		),
		Annotations: utils.Union(
			annotations,
			explainerAnnotations,
		),
	}
	container := explainer.GetContainer(isvc.ObjectMeta, isvc.Spec.Explainer.GetExtensions(), e.inferenceServiceConfig, predictorName)
	if len(isvc.Spec.Explainer.PodSpec.Containers) == 0 {
		isvc.Spec.Explainer.PodSpec.Containers = []v1.Container{
			*container,
		}
	} else {
		isvc.Spec.Explainer.PodSpec.Containers[0] = *container
	}

	podSpec := v1.PodSpec(isvc.Spec.Explainer.PodSpec)

	// Here we allow switch between knative and vanilla deployment
	if e.deploymentMode == constants.RawDeployment {
		r, err := raw.NewRawKubeReconciler(e.client, e.clientset, e.scheme, constants.InferenceServiceResource, objectMeta, metav1.ObjectMeta{},
			&isvc.Spec.Explainer.ComponentExtensionSpec, &podSpec, nil)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to create NewRawKubeReconciler for explainer")
		}
		// set Deployment Controller
		for _, deployment := range r.Deployment.DeploymentList {
			if err := controllerutil.SetControllerReference(isvc, deployment, e.scheme); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "fails to set deployment owner reference for explainer")
			}
		}
		// set Service Controller
		for _, svc := range r.Service.ServiceList {
			if err := controllerutil.SetControllerReference(isvc, svc, e.scheme); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "fails to set service owner reference for explainer")
			}
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
		r, err := knative.NewKsvcReconciler(e.client, e.scheme, objectMeta, &isvc.Spec.Explainer.ComponentExtensionSpec,
			&podSpec, isvc.Status.Components[v1beta1.ExplainerComponent], e.inferenceServiceConfig.ServiceLabelDisallowedList)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to create new knative service reconciler for explainer")
		}

		if err := controllerutil.SetControllerReference(isvc, r.Service, e.scheme); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to set owner reference for explainer")
		}
		status, err := r.Reconcile()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile explainer")
		}
		isvc.Status.PropagateStatus(v1beta1.ExplainerComponent, status)
	}
	return ctrl.Result{}, nil
}
