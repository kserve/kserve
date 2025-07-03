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
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	knutils "github.com/kserve/kserve/pkg/controller/v1alpha1/utils"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	raw "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

var _ Component = &Transformer{}

// Transformer reconciles resources for this component.
type Transformer struct {
	client                 client.Client
	clientset              kubernetes.Interface
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	deploymentMode         constants.DeploymentModeType
	allowZeroInitialScale  bool
	Log                    logr.Logger
}

func NewTransformer(client client.Client, clientset kubernetes.Interface, scheme *runtime.Scheme,
	inferenceServiceConfig *v1beta1.InferenceServicesConfig, deploymentMode constants.DeploymentModeType, allowZeroInitialScale bool,
) Component {
	return &Transformer{
		client:                 client,
		clientset:              clientset,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		deploymentMode:         deploymentMode,
		allowZeroInitialScale:  allowZeroInitialScale,
		Log:                    ctrl.Log.WithName("TransformerReconciler"),
	}
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (p *Transformer) Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) (ctrl.Result, error) {
	p.Log.Info("Reconciling Transformer", "TransformerSpec", isvc.Spec.Transformer)
	transformer := isvc.Spec.Transformer.GetImplementation()
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
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := transformer.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
		err := isvcutils.ValidateStorageURI(ctx, sourceURI, p.client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("StorageURI not supported: %w", err)
		}
	}
	addLoggerAnnotations(isvc.Spec.Transformer.Logger, annotations)
	addBatcherAnnotations(isvc.Spec.Transformer.Batcher, annotations)

	transformerName := constants.TransformerServiceName(isvc.Name)
	predictorName := constants.PredictorServiceName(isvc.Name)

	// Labels and annotations from transformer component
	// Label filter will be handled in ksvc_reconciler and raw reconciler
	transformerLabels := isvc.Spec.Transformer.Labels
	var transformerAnnotations map[string]string
	if p.deploymentMode == constants.RawDeployment {
		transformerAnnotations = utils.Filter(isvc.Spec.Transformer.Annotations, func(key string) bool {
			// https://issues.redhat.com/browse/RHOAIENG-20326
			// For RawDeployment, we allow the security.opendatahub.io/enable-auth annotation
			return !utils.Includes(isvcutils.FilterList(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, constants.ODHKserveRawAuth), key)
		})
	} else {
		transformerAnnotations = utils.Filter(isvc.Spec.Transformer.Annotations, func(key string) bool {
			return !utils.Includes(p.inferenceServiceConfig.ServiceAnnotationDisallowedList, key)
		})
	}

	// Labels and annotations priority: transformer component > isvc
	// Labels and annotations from high priority will overwrite that from low priority
	objectMeta := metav1.ObjectMeta{
		Name:      transformerName,
		Namespace: isvc.Namespace,
		Labels: utils.Union(
			isvc.Labels,
			transformerLabels,
			map[string]string{
				constants.InferenceServicePodLabelKey: isvc.Name,
				constants.KServiceComponentLabel:      string(v1beta1.TransformerComponent),
			},
		),
		Annotations: utils.Union(
			annotations,
			transformerAnnotations,
		),
	}

	// Need to wait for predictor URL in modelmesh deployment mode
	if p.deploymentMode == constants.ModelMeshDeployment {
		// check if predictor URL is populated
		predictorURL := (*url.URL)(isvc.Status.Components["predictor"].URL)
		if predictorURL == nil {
			// transformer reconcile will retry every 3 second until predictor URL is populated
			p.Log.Info("Transformer reconciliation is waiting for predictor URL to be populated")
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}

		// add predictor host and protocol to metadata
		isvc.ObjectMeta.Annotations[constants.PredictorHostAnnotationKey] = predictorURL.Host
		switch predictorURL.Scheme {
		case "grpc":
			isvc.ObjectMeta.Annotations[constants.PredictorProtocolAnnotationKey] = string(constants.ProtocolGRPCV2)
		case "http", "https":
			// modelmesh supports v2 only
			isvc.ObjectMeta.Annotations[constants.PredictorProtocolAnnotationKey] = string(constants.ProtocolV2)
		default:
			return ctrl.Result{}, fmt.Errorf("Predictor URL Scheme not supported: %v", predictorURL.Scheme)
		}
	}

	if len(isvc.Spec.Transformer.PodSpec.Containers) == 0 {
		container := transformer.GetContainer(isvc.ObjectMeta, isvc.Spec.Transformer.GetExtensions(), p.inferenceServiceConfig, predictorName)
		isvc.Spec.Transformer.PodSpec = v1beta1.PodSpec{
			Containers: []corev1.Container{
				*container,
			},
		}
	} else {
		container := transformer.GetContainer(isvc.ObjectMeta, isvc.Spec.Transformer.GetExtensions(), p.inferenceServiceConfig, predictorName)
		isvc.Spec.Transformer.PodSpec.Containers[0] = *container
	}

	podSpec := corev1.PodSpec(isvc.Spec.Transformer.PodSpec)

	// Here we allow switch between knative and vanilla deployment
	if p.deploymentMode == constants.RawDeployment {
		if err := p.reconcileTransformerRawDeployment(ctx, isvc, &objectMeta, &podSpec); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := p.reconcileTransformerKnativeDeployment(ctx, isvc, &objectMeta, &podSpec); err != nil {
			return ctrl.Result{}, err
		}
	}

	if utils.GetForceStopRuntime(isvc) {
		// Exit early if we have already set the transformer's status to stopped
		existingTransformerCondition := isvc.Status.GetCondition(v1beta1.TransformerReady)
		if existingTransformerCondition != nil && existingTransformerCondition.Status == corev1.ConditionFalse && existingTransformerCondition.Reason == v1beta1.StoppedISVCReason {
			return ctrl.Result{}, nil
		}

		// Set the ready condition to false
		isvc.Status.SetCondition(v1beta1.TransformerReady, &apis.Condition{
			Type:   v1beta1.TransformerReady,
			Status: corev1.ConditionFalse,
			Reason: v1beta1.StoppedISVCReason,
		})
	}
	return ctrl.Result{}, nil
}

func (p *Transformer) reconcileTransformerRawDeployment(ctx context.Context, isvc *v1beta1.InferenceService, objectMeta *metav1.ObjectMeta, podSpec *corev1.PodSpec) error {
	r, err := raw.NewRawKubeReconciler(ctx, p.client, p.clientset, p.scheme, constants.InferenceServiceResource, *objectMeta, metav1.ObjectMeta{},
		&isvc.Spec.Transformer.ComponentExtensionSpec, podSpec, nil)
	if err != nil {
		return errors.Wrapf(err, "fails to create NewRawKubeReconciler for transformer")
	}
	// set Deployment Controller
	for _, deployment := range r.Deployment.DeploymentList {
		if err := controllerutil.SetControllerReference(isvc, deployment, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set deployment owner reference for transformer")
		}
	}
	// set Service Controller
	for _, svc := range r.Service.ServiceList {
		if err := controllerutil.SetControllerReference(isvc, svc, p.scheme); err != nil {
			return errors.Wrapf(err, "fails to set service owner reference for transformer")
		}
	}
	// set autoscaler Controller
	if err := r.Scaler.Autoscaler.SetControllerReferences(isvc, p.scheme); err != nil {
		return errors.Wrapf(err, "fails to set autoscaler owner references for transformer")
	}

	deployment, err := r.Reconcile(ctx)
	if err != nil {
		return errors.Wrapf(err, "fails to reconcile transformer")
	}
	if !utils.GetForceStopRuntime(isvc) {
		isvc.Status.PropagateRawStatus(v1beta1.TransformerComponent, deployment, r.URL)
	}
	return nil
}

func (p *Transformer) reconcileTransformerKnativeDeployment(ctx context.Context, isvc *v1beta1.InferenceService, objectMeta *metav1.ObjectMeta, podSpec *corev1.PodSpec) error {
	knutils.ValidateInitialScaleAnnotation(objectMeta.Annotations, p.allowZeroInitialScale, isvc.Spec.Transformer.MinReplicas, p.Log)
	r := knative.NewKsvcReconciler(p.client, p.scheme, *objectMeta, &isvc.Spec.Transformer.ComponentExtensionSpec,
		podSpec, isvc.Status.Components[v1beta1.TransformerComponent], p.inferenceServiceConfig.ServiceLabelDisallowedList)

	if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
		return errors.Wrapf(err, "fails to set owner reference for transformer")
	}
	kstatus, err := r.Reconcile(ctx)
	if err != nil {
		return errors.Wrapf(err, "fails to reconcile transformer")
	}
	if !utils.GetForceStopRuntime(isvc) {
		isvc.Status.PropagateStatus(v1beta1.TransformerComponent, kstatus)
	}
	return nil
}
