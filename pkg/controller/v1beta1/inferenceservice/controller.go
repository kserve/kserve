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

package inferenceservice

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/components"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/cabundleconfigmap"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/ingress"
	modelconfig "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/modelconfig"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices;inferenceservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes;servingruntimes/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterservingruntimes;clusterservingruntimes/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterservingruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterstoragecontainers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opentelemetry.io,resources=opentelemetrycollectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opentelemetry.io,resources=opentelemetrycollectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opentelemetry.io,resources=opentelemetrycollectors/finalizers,verbs=get;list;watch;create;update;patch;delete

// InferenceServiceState describes the Readiness of the InferenceService
type InferenceServiceState string

// Different InferenceServiceState an InferenceService may have.
const (
	InferenceServiceReadyState    InferenceServiceState = "InferenceServiceReady"
	InferenceServiceNotReadyState InferenceServiceState = "InferenceServiceNotReady"
)

// InferenceServiceReconciler reconciles a InferenceService object
type InferenceServiceReconciler struct {
	client.Client
	ClientConfig *rest.Config
	Clientset    kubernetes.Interface
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
}

func (r *InferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the InferenceService instance
	isvc := &v1beta1.InferenceService{}
	if err := r.Get(ctx, req.NamespacedName, isvc); err != nil {
		if apierr.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, r.Clientset)
	if err != nil {
		r.Log.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return reconcile.Result{}, err
	}
	isvcConfig, err := v1beta1.NewInferenceServicesConfig(isvcConfigMap)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create InferenceServicesConfig")
	}

	// get annotations from isvc
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})

	deployConfig, err := v1beta1.NewDeployConfig(isvcConfigMap)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create DeployConfig")
	}

	deploymentMode := isvcutils.GetDeploymentMode(annotations, deployConfig)
	r.Log.Info("Inference service deployment mode ", "deployment mode ", deploymentMode)

	if deploymentMode == constants.ModelMeshDeployment {
		if isvc.Spec.Transformer == nil {
			// Skip if no transformers
			r.Log.Info("Skipping reconciliation for InferenceService", constants.DeploymentMode, deploymentMode,
				"apiVersion", isvc.APIVersion, "isvc", isvc.Name)
			return ctrl.Result{}, nil
		}
		// Continue to reconcile when there is a transformer
		r.Log.Info("Continue reconciliation for InferenceService", constants.DeploymentMode, deploymentMode,
			"apiVersion", isvc.APIVersion, "isvc", isvc.Name)
	}
	// name of our custom finalizer
	finalizerName := "inferenceservice.finalizers"

	// examine DeletionTimestamp to determine if object is under deletion
	if isvc.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(isvc, finalizerName) {
			controllerutil.AddFinalizer(isvc, finalizerName)
			patchYaml := "metadata:\n  finalizers: [" + strings.Join(isvc.ObjectMeta.Finalizers, ",") + "]"
			patchJson, _ := yaml.YAMLToJSON([]byte(patchYaml))
			if err := r.Patch(ctx, isvc, client.RawPatch(types.MergePatchType, patchJson)); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(isvc, finalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, isvc); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(isvc, finalizerName)
			patchYaml := "metadata:\n  finalizers: [" + strings.Join(isvc.ObjectMeta.Finalizers, ",") + "]"
			patchJson, _ := yaml.YAMLToJSON([]byte(patchYaml))
			if err := r.Patch(ctx, isvc, client.RawPatch(types.MergePatchType, patchJson)); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Abort early if the resolved deployment mode is Serverless, but Knative Services are not available
	if deploymentMode == constants.Serverless {
		ksvcAvailable, checkKsvcErr := utils.IsCrdAvailable(r.ClientConfig, knservingv1.SchemeGroupVersion.String(), constants.KnativeServiceKind)
		if checkKsvcErr != nil {
			return reconcile.Result{}, checkKsvcErr
		}

		if !ksvcAvailable {
			r.Recorder.Event(isvc, corev1.EventTypeWarning, "ServerlessModeRejected",
				"It is not possible to use Serverless deployment mode when Knative Services are not available")
			return reconcile.Result{Requeue: false}, reconcile.TerminalError(fmt.Errorf("the resolved deployment mode of InferenceService '%s' is Serverless, but Knative Serving is not available", isvc.Name))
		}
	}

	// Setup reconcilers
	r.Log.Info("Reconciling inference service", "apiVersion", isvc.APIVersion, "isvc", isvc.Name)

	// Reconcile cabundleConfigMap
	caBundleConfigMapReconciler := cabundleconfigmap.NewCaBundleConfigMapReconciler(r.Client, r.Clientset, r.Scheme)
	if err := caBundleConfigMapReconciler.Reconcile(ctx, isvc); err != nil {
		return reconcile.Result{}, err
	}

	reconcilers := []components.Component{}
	if deploymentMode != constants.ModelMeshDeployment {
		reconcilers = append(reconcilers, components.NewPredictor(r.Client, r.Clientset, r.Scheme, isvcConfig, deploymentMode))
	}
	if isvc.Spec.Transformer != nil {
		reconcilers = append(reconcilers, components.NewTransformer(r.Client, r.Clientset, r.Scheme, isvcConfig, deploymentMode))
	}
	if isvc.Spec.Explainer != nil {
		reconcilers = append(reconcilers, components.NewExplainer(r.Client, r.Clientset, r.Scheme, isvcConfig, deploymentMode))
	}
	for _, reconciler := range reconcilers {
		result, err := reconciler.Reconcile(ctx, isvc)
		if err != nil {
			r.Log.Error(err, "Failed to reconcile", "reconciler", reflect.ValueOf(reconciler), "Name", isvc.Name)
			r.Recorder.Eventf(isvc, corev1.EventTypeWarning, "InternalError", err.Error())
			if err := r.updateStatus(ctx, isvc, deploymentMode); err != nil {
				r.Log.Error(err, "Error updating status")
				return result, err
			}
			return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile component")
		}
		if result.Requeue || result.RequeueAfter > 0 {
			return result, nil
		}
	}
	// reconcile RoutesReady and LatestDeploymentReady conditions for serverless deployment
	if deploymentMode == constants.Serverless {
		componentList := []v1beta1.ComponentType{v1beta1.PredictorComponent}
		if isvc.Spec.Transformer != nil {
			componentList = append(componentList, v1beta1.TransformerComponent)
		}
		if isvc.Spec.Explainer != nil {
			componentList = append(componentList, v1beta1.ExplainerComponent)
		}
		isvc.Status.PropagateCrossComponentStatus(componentList, v1beta1.RoutesReady)
		isvc.Status.PropagateCrossComponentStatus(componentList, v1beta1.LatestDeploymentReady)
	}
	// Reconcile ingress
	ingressConfig, err := v1beta1.NewIngressConfig(isvcConfigMap)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create IngressConfig")
	}

	// check raw deployment
	if deploymentMode == constants.RawDeployment {
		if ingressConfig.EnableGatewayAPI {
			reconciler := ingress.NewRawHTTPRouteReconciler(r.Client, r.Scheme, ingressConfig, isvcConfig)

			if err := reconciler.Reconcile(ctx, isvc); err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
			}
		} else {
			reconciler, err := ingress.NewRawIngressReconciler(r.Client, r.Scheme, ingressConfig, isvcConfig)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
			}
			if err := reconciler.Reconcile(ctx, isvc); err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
			}
		}
	} else {
		reconciler := ingress.NewIngressReconciler(r.Client, r.Clientset, r.Scheme, ingressConfig, isvcConfig)
		r.Log.Info("Reconciling ingress for inference service", "isvc", isvc.Name)
		if err := reconciler.Reconcile(ctx, isvc); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile ingress")
		}
	}

	// Reconcile modelConfig
	configMapReconciler := modelconfig.NewModelConfigReconciler(r.Client, r.Clientset, r.Scheme)
	if err := configMapReconciler.Reconcile(ctx, isvc); err != nil {
		return reconcile.Result{}, err
	}

	if err = r.updateStatus(ctx, isvc, deploymentMode); err != nil {
		r.Recorder.Event(isvc, corev1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *InferenceServiceReconciler) updateStatus(ctx context.Context, desiredService *v1beta1.InferenceService,
	deploymentMode constants.DeploymentModeType,
) error {
	existingService := &v1beta1.InferenceService{}
	namespacedName := types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}
	if err := r.Get(ctx, namespacedName, existingService); err != nil {
		return err
	}
	wasReady := inferenceServiceReadiness(existingService.Status)
	if inferenceServiceStatusEqual(existingService.Status, desiredService.Status, deploymentMode) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if err := r.Status().Update(ctx, desiredService); err != nil {
		r.Log.Error(err, "Failed to update InferenceService status", "InferenceService", desiredService.Name)
		r.Recorder.Eventf(desiredService, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for InferenceService %q: %v", desiredService.Name, err)
		return errors.Wrapf(err, "fails to update InferenceService status")
	} else {
		// If there was a difference and there was no error.
		isReady := inferenceServiceReadiness(desiredService.Status)
		isReadyFalse := inferenceServiceReadinessFalse(desiredService.Status)
		if wasReady && isReadyFalse { // Moved to NotReady State
			r.Recorder.Eventf(desiredService, corev1.EventTypeWarning, string(InferenceServiceNotReadyState),
				fmt.Sprintf("InferenceService [%v] is no longer Ready because of: %v", desiredService.GetName(), r.GetFailConditions(desiredService)))
		} else if !wasReady && isReady { // Moved to Ready State
			r.Recorder.Eventf(desiredService, corev1.EventTypeNormal, string(InferenceServiceReadyState),
				fmt.Sprintf("InferenceService [%v] is Ready", desiredService.GetName()))
		}
	}
	return nil
}

func inferenceServiceReadiness(status v1beta1.InferenceServiceStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == corev1.ConditionTrue
}

func inferenceServiceReadinessFalse(status v1beta1.InferenceServiceStatus) bool {
	readyCondition := status.GetCondition(apis.ConditionReady)
	return readyCondition != nil && readyCondition.Status == corev1.ConditionFalse
}

func inferenceServiceStatusEqual(s1, s2 v1beta1.InferenceServiceStatus, deploymentMode constants.DeploymentModeType) bool {
	if deploymentMode == constants.ModelMeshDeployment {
		// If the deployment mode is ModelMesh, reduce the status scope to compare.
		// Exclude Predictor and ModelStatus which are mananged by ModelMesh controllers
		return equality.Semantic.DeepEqual(s1.Address, s2.Address) &&
			equality.Semantic.DeepEqual(s1.URL, s2.URL) &&
			equality.Semantic.DeepEqual(s1.Status, s2.Status) &&
			equality.Semantic.DeepEqual(s1.Components[v1beta1.TransformerComponent], s2.Components[v1beta1.TransformerComponent]) &&
			equality.Semantic.DeepEqual(s1.Components[v1beta1.ExplainerComponent], s2.Components[v1beta1.ExplainerComponent])
	}
	return equality.Semantic.DeepEqual(s1, s2)
}

func (r *InferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager, deployConfig *v1beta1.DeployConfig, ingressConfig *v1beta1.IngressConfig) error {
	r.ClientConfig = mgr.GetConfig()

	ksvcFound, err := utils.IsCrdAvailable(r.ClientConfig, knservingv1.SchemeGroupVersion.String(), constants.KnativeServiceKind)
	if err != nil {
		return err
	}

	kedaFound, err := utils.IsCrdAvailable(r.ClientConfig, kedav1alpha1.SchemeGroupVersion.String(), constants.KedaScaledObjectKind)
	if err != nil {
		return err
	}

	otelFound, err := utils.IsCrdAvailable(r.ClientConfig, otelv1alpha1.GroupVersion.String(), constants.OpenTelemetryCollector)
	if err != nil {
		return err
	}

	vsFound, err := utils.IsCrdAvailable(r.ClientConfig, istioclientv1beta1.SchemeGroupVersion.String(), constants.IstioVirtualServiceKind)
	if err != nil {
		return err
	}

	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.InferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{})

	if ksvcFound {
		ctrlBuilder = ctrlBuilder.Owns(&knservingv1.Service{})
	} else {
		r.Log.Info("The InferenceService controller won't watch serving.knative.dev/v1/Service resources because the CRD is not available.")
	}

	if kedaFound {
		// GenerationChangedPredicate only trigger reconciliation when the `spec` of the ScaledObject changes,
		// ignoring updates to `status` or metadata fields like `resourceVersion`.
		ctrlBuilder = ctrlBuilder.Owns(&kedav1alpha1.ScaledObject{}).
			WithEventFilter(predicate.GenerationChangedPredicate{})
	} else {
		r.Log.Info("The InferenceService controller won't watch keda.sh/v1/ScaledObject resources because the CRD is not available.")
	}

	if otelFound {
		// Only trigger reconciliation when the `spec` of the OpenTelemetryCollector changes,
		// ignoring updates to `status` or metadata fields like `resourceVersion`.
		ctrlBuilder = ctrlBuilder.Owns(&otelv1alpha1.OpenTelemetryCollector{}).
			WithEventFilter(predicate.GenerationChangedPredicate{})
	} else {
		r.Log.Info("The InferenceService controller won't watch opentelemetry-collector resources because the CRD is not available.")
	}

	if vsFound && !ingressConfig.DisableIstioVirtualHost {
		ctrlBuilder = ctrlBuilder.Owns(&istioclientv1beta1.VirtualService{})
	} else {
		r.Log.Info("The InferenceService controller won't watch networking.istio.io/v1beta1/VirtualService resources because the CRD is not available.")
	}

	if ingressConfig.EnableGatewayAPI {
		gatewayapiFound, err := utils.IsCrdAvailable(r.ClientConfig, gatewayapiv1.GroupVersion.String(), constants.HTTPRouteKind)
		if err != nil {
			return err
		}

		if gatewayapiFound {
			ctrlBuilder = ctrlBuilder.Owns(&gatewayapiv1.HTTPRoute{})
		} else {
			r.Log.Info("The InferenceService controller won't watch gateway.networking.k8s.io/v1/HTTPRoute resources because the CRD is not available.")
			panic("Gateway API CRD not available")
		}
	} else {
		ctrlBuilder = ctrlBuilder.Owns(&netv1.Ingress{})
	}

	return ctrlBuilder.Complete(r)
}

func (r *InferenceServiceReconciler) deleteExternalResources(ctx context.Context, isvc *v1beta1.InferenceService) error {
	// Delete all the TrainedModel that uses this InferenceService as parent
	r.Log.Info("Deleting external resources", "InferenceService", isvc.Name)
	var trainedModels v1alpha1.TrainedModelList
	if err := r.List(ctx,
		&trainedModels,
		client.MatchingLabels{constants.ParentInferenceServiceLabel: isvc.Name},
		client.InNamespace(isvc.Namespace),
	); err != nil {
		r.Log.Error(err, "unable to list trained models", "inferenceservice", isvc.Name)
		return err
	}

	// #nosec G601
	for i, v := range trainedModels.Items {
		if err := r.Delete(ctx, &trainedModels.Items[i], client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			r.Log.Error(err, "unable to delete trainedmodel", "trainedmodel", v)
		}
	}
	return nil
}

func (r *InferenceServiceReconciler) GetFailConditions(isvc *v1beta1.InferenceService) string {
	msg := ""
	for _, cond := range isvc.Status.Conditions {
		if string(cond.Status) == "False" {
			if msg == "" {
				msg = string(cond.Type)
			} else {
				msg = fmt.Sprintf("%s, %s", msg, string(cond.Type))
			}
		}
	}
	return msg
}
