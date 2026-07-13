/*
Copyright 2025 The KServe Authors.

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

package llmisvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/cabundleconfigmap"

	"knative.dev/pkg/apis"
	"knative.dev/pkg/reconciler"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	resourcev1 "k8s.io/api/resource/v1"

	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	wvav1alpha1 "github.com/llm-d/llm-d-workload-variant-autoscaler/api/v1alpha1"

	"github.com/kserve/kserve/pkg/utils"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

// ChildResourcesLabelSelector matches resources belonging to LLMInferenceService.
// Used by the controller predicate below and by the manager cache (cmd/llmisvc/main.go).
var ChildResourcesLabelSelector = metav1.LabelSelector{
	MatchLabels: map[string]string{
		constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue,
	},
}

// childResourcesPredicate filters events to only those from resources owned by LLMInferenceService
// This prevents unnecessary reconciliation triggers from unrelated resources
var childResourcesPredicate, _ = predicate.LabelSelectorPredicate(ChildResourcesLabelSelector)

var (
	inferenceServiceConfigMapPredicate = predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isInferenceServiceConfigMap(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isInferenceServiceConfigMap(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isInferenceServiceConfigMap(e.ObjectNew) && inferenceServiceConfigChanged(e.ObjectOld, e.ObjectNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isInferenceServiceConfigMap(e.Object)
		},
	}
	nonInferenceServiceConfigMapPredicate = predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return !isInferenceServiceConfigMap(obj)
	})
)

func isInferenceServiceConfigMap(obj client.Object) bool {
	return obj.GetNamespace() == constants.KServeNamespace &&
		obj.GetName() == constants.InferenceServiceConfigMapName
}

func inferenceServiceConfigChanged(oldObj, newObj client.Object) bool {
	oldConfigMap := oldObj.(*corev1.ConfigMap)
	newConfigMap := newObj.(*corev1.ConfigMap)

	oldConfig, oldErr := toConfig(oldConfigMap)
	newConfig, newErr := toConfig(newConfigMap)
	if oldErr != nil || newErr != nil {
		ctrl.Log.WithName("LLMInferenceService").Error(
			errors.Join(oldErr, newErr),
			"Failed to parse inferenceservice-config, falling back to raw comparison",
		)
		return !equality.Semantic.DeepEqual(oldConfigMap.Data, newConfigMap.Data)
	}

	return !equality.Semantic.DeepEqual(oldConfig, newConfig)
}

// LLMInferenceServiceState describes the readiness of the LLMInferenceService.
type LLMInferenceServiceState string

const (
	LLMInferenceServiceReadyState    LLMInferenceServiceState = "LLMInferenceServiceReady"
	LLMInferenceServiceNotReadyState LLMInferenceServiceState = "LLMInferenceServiceNotReady"
)

// LLMISVCReconciler reconciles an LLMInferenceService object.
// It orchestrates the reconciliation of child resources based on the spec.
type LLMISVCReconciler struct {
	client.Client
	Config *rest.Config
	record.EventRecorder
	Clientset kubernetes.Interface

	Validator func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error
}

//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices/finalizers,verbs=update
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=leaderworkerset.x-k8s.io,resources=leaderworkersets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;gateways;gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencepools;inferenceobjectives;inferencemodels;inferencemodelrewrites;inferencepoolimports,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=inference.networking.k8s.io,resources=inferencepools;inferenceobjectives;inferencemodels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=resource.k8s.io,resources=resourceclaims;resourceclaimtemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=llm-d.ai,resources=inferenceobjectives;inferencemodelrewrites,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews;subjectaccessreviews,verbs=create
//+kubebuilder:rbac:urls=/metrics,verbs=get
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,resourceNames=llminferenceservices.serving.kserve.io;llminferenceserviceconfigs.serving.kserve.io,verbs=get;list;watch
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions/status,resourceNames=llminferenceservices.serving.kserve.io;llminferenceserviceconfigs.serving.kserve.io,verbs=update;patch
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=llmd.ai,resources=variantautoscalings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches,verbs=get;list;watch
//+kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnamespacecaches,verbs=get;list;watch
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main entry point for the reconciliation loop.
// It fetches the LLMInferenceService and delegates the reconciliation of its constituent parts.
// The reconciler follows the standard Kubernetes controller pattern with finalizers for cleanup.
func (r *LLMISVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("LLMInferenceService").
		WithValues("Namespace", req.Namespace, "Name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	logger.Info("Starting reconciliation")
	original := &v1alpha2.LLMInferenceService{}
	if err := r.Get(ctx, req.NamespacedName, original); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle finalizer for proper cleanup on deletion
	finalizerName := constants.KServeAPIGroupName + "/llmisvc-finalizer"
	if original.DeletionTimestamp.IsZero() {
		// Resource is not being deleted, ensure finalizer is present
		if controllerutil.AddFinalizer(original, finalizerName) {
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// Resource is being deleted, perform cleanup
		logger.Info("Marked for deletion, finalizing resources")
		if controllerutil.ContainsFinalizer(original, finalizerName) {
			done, cleanupErr := r.finalize(ctx, original)
			if cleanupErr != nil {
				logger.Error(cleanupErr, "Finalization failed")
				return ctrl.Result{}, cleanupErr
			}
			if !done {
				logger.Info("Finalization incomplete, requeueing")
				if statusErr := r.updateStatus(ctx, original); statusErr != nil {
					logger.Error(statusErr, "Failed to persist finalization status")
				}
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			// Cleanup successful, remove finalizer to allow deletion
			controllerutil.RemoveFinalizer(original, finalizerName)
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Do not reconcile, because llmisvc is being deleted.
		return ctrl.Result{}, nil
	}

	// Reconcile cabundleConfigMap
	caBundleConfigMapReconciler := cabundleconfigmap.NewCaBundleConfigMapReconciler(r.Client, r.Clientset)
	if err := caBundleConfigMapReconciler.Reconcile(ctx, original.Namespace); err != nil {
		return reconcile.Result{}, err
	}

	// Work with a copy to avoid modifying the original until status update
	resource := original.DeepCopy()

	// Pre/post process hooks for status management
	reconciler.PreProcessReconcile(ctx, resource)
	reconcileErr := r.reconcile(ctx, resource)
	reconciler.PostProcessReconcile(ctx, resource, original)

	if reconcileErr != nil {
		logger.Error(reconcileErr, "Failed to reconcile LLMInferenceService")
		r.Eventf(original, corev1.EventTypeWarning, "Error", "Reconciliation failed: %v", reconcileErr.Error())
	}

	if err := r.updateStatus(ctx, resource); err != nil {
		logger.Error(err, "Failed to update status for LLMInferenceService")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, reconcileErr
}

// reconcile handles the core business logic of reconciling an LLMInferenceService
// It loads configuration, merges base configs, and reconciles workload and router components
func (r *LLMISVCReconciler) reconcile(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("reconcile")
	ctx = log.IntoContext(ctx, logger)

	// Load global configuration from KServe configmap.
	config, configErr := r.loadConfig(ctx)
	if configErr != nil {
		return fmt.Errorf("failed to load ingress config: %w", configErr)
	}

	// Advisory warning: if oci+native:// mode is configured, check the cluster K8s version
	// and surface an OciImageVolumeCompatible condition when ImageVolume support may be absent.
	if config.StorageConfig != nil {
		warnIfImageVolumeUnsupported(ctx, r.Clientset.Discovery(),
			llmSvc, kserveTypes.ResolveOciModelMode(config.StorageConfig))
	}

	// nil baseCfg means config resolution set a condition (e.g. ConfigNotFound) and there's nothing more to do.
	baseCfg, err := r.reconcileBaseRefs(ctx, llmSvc, config)
	if err != nil || baseCfg == nil {
		return err
	}

	logger.V(2).Info("Reconciling with combined base configurations", "combined.spec", baseCfg.Spec, "original.spec", llmSvc.Spec)
	// Replace the spec with the merged configuration for reconciliation
	// We are only writing to status, so we can safely use the original object.
	llmSvc.Spec = baseCfg.Spec

	if err := r.reconcileWorkload(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile workload: %w", err)
	}

	if err := r.reconcileRouter(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile networking: %w", err)
	}

	observeWorkloadStatus(llmSvc)

	return nil
}

// finalize performs cleanup operations when the LLMInferenceService is being deleted.
// Returns (done, err): done=false signals that cleanup is still in progress and the
// caller should requeue.
func (r *LLMISVCReconciler) finalize(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (bool, error) {
	done, err := r.finalizeGroupMembership(ctx, llmSvc)
	if err != nil {
		return false, err
	}
	if !done {
		return false, nil
	}

	if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
		return false, fmt.Errorf("failed to finalize scheduler service account: %w", err)
	}

	return true, nil
}

// updateStatus updates the status of the LLMInferenceService with retry on conflict.
// It also emits K8s Events on readiness state transitions (Ready <-> NotReady),
// mirroring the pattern used by the InferenceService controller.
func (r *LLMISVCReconciler) updateStatus(ctx context.Context, desired *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1alpha2.LLMInferenceService{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(desired), latest); err != nil {
			return client.IgnoreNotFound(err)
		}

		wasReady := llmInferenceServiceReadiness(latest.Status)

		if equality.Semantic.DeepEqual(latest.Status, desired.Status) {
			return nil
		}

		latest.Status = desired.Status

		if err := r.Client.Status().Update(ctx, latest); err != nil {
			logger.Error(err, "Failed to update LLMInferenceService status", "LLMInferenceService", desired.Name)
			r.Eventf(desired, corev1.EventTypeWarning, "UpdateFailed",
				"Failed to update status for LLMInferenceService %q: %v", desired.Name, err)
			return fmt.Errorf("failed to update status for LLMInferenceService: %w", err)
		}

		isReady := llmInferenceServiceReadiness(desired.Status)
		isReadyFalse := llmInferenceServiceReadinessFalse(desired.Status)
		if wasReady && isReadyFalse {
			r.Eventf(desired, corev1.EventTypeWarning, string(LLMInferenceServiceNotReadyState),
				"LLMInferenceService [%v] is no longer Ready because of: %v", desired.GetName(), GetFailConditions(desired))
		} else if !wasReady && isReady {
			r.Eventf(desired, corev1.EventTypeNormal, string(LLMInferenceServiceReadyState),
				"LLMInferenceService [%v] is Ready", desired.GetName())
		}

		return nil
	})
}

func llmInferenceServiceReadiness(status v1alpha2.LLMInferenceServiceStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == corev1.ConditionTrue
}

func llmInferenceServiceReadinessFalse(status v1alpha2.LLMInferenceServiceStatus) bool {
	readyCondition := status.GetCondition(apis.ConditionReady)
	return readyCondition != nil && readyCondition.Status == corev1.ConditionFalse
}

// GetFailConditions returns a comma-separated list of sub-condition Types whose Status is False.
// The top-level apis.ConditionReady is intentionally excluded because it is the aggregate that
// is being reported on; including it would be self-referential ("Ready is no longer Ready
// because of: Ready, ...").
func GetFailConditions(svc *v1alpha2.LLMInferenceService) string {
	msg := ""
	for _, cond := range svc.Status.Conditions {
		if cond.Type == apis.ConditionReady {
			continue
		}
		if cond.Status == corev1.ConditionFalse {
			if msg == "" {
				msg = string(cond.Type)
			} else {
				msg = fmt.Sprintf("%s, %s", msg, string(cond.Type))
			}
		}
	}
	return msg
}

// SetupWithManager sets up the controller with the Manager.
// It configures watches for the LLMInferenceService and its owned resources.
// The controller conditionally registers watchers based on CRD availability to avoid errors in environments where optional CRDs are not installed.
func (r *LLMISVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("LLMInferenceService.SetupWithManager")

	if err := setupGroupFieldIndex(context.Background(), mgr.GetFieldIndexer()); err != nil {
		return fmt.Errorf("failed to set up field indexer for routing group: %w", err)
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.LLMInferenceService{}).
		Watches(&v1alpha2.LLMInferenceServiceConfig{}, r.enqueueOnLLMInferenceServiceConfigChange(logger)).
		Owns(&netv1.Ingress{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&corev1.Secret{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&corev1.Service{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}, builder.WithPredicates(childResourcesPredicate)).
		Watches(&corev1.ConfigMap{}, r.enqueueOnConfigMapChange(logger), builder.WithPredicates(nonInferenceServiceConfigMapPredicate)).
		Watches(&corev1.ConfigMap{}, r.enqueueOnInferenceServiceConfigMapChange(logger), builder.WithPredicates(inferenceServiceConfigMapPredicate)).
		Watches(&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.EnqueueOnLLMInferenceServicePods),
			builder.WithPredicates(PodStatusPredicate()))

	b = b.Watches(
		&v1alpha2.LLMInferenceService{},
		&groupMemberEventHandler{reconciler: r},
		builder.WithPredicates(groupMemberChangePredicate()),
	)

	if err := r.extendControllerSetup(mgr, b); err != nil {
		return fmt.Errorf("failed to extend controller setup: %w", err)
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), gwapiv1.GroupVersion.String(), "HTTPRoute"); ok && err == nil {
		b = b.Owns(&gwapiv1.HTTPRoute{}, builder.WithPredicates(childResourcesPredicate)).
			Watches(&gwapiv1.HTTPRoute{}, r.enqueueOnHttpRouteChange(logger))
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), gwapiv1.GroupVersion.String(), "Gateway"); ok && err == nil {
		b = b.Watches(&gwapiv1.Gateway{}, r.enqueueOnGatewayChange(logger))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), igwapi.GroupVersion.String(), "InferencePool"); ok && err == nil {
		b = b.Owns(&igwapi.InferencePool{}, builder.WithPredicates(childResourcesPredicate)).
			Watches(&igwapi.InferencePool{}, r.enqueueOnInferencePoolChange(logger))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), igwapiv1alpha2.GroupVersion.String(), "InferencePool"); ok && err == nil {
		b = b.Owns(&igwapiv1alpha2.InferencePool{}, builder.WithPredicates(childResourcesPredicate)).
			Watches(&igwapiv1alpha2.InferencePool{}, r.enqueueOnInferencePoolChange(logger))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), wvav1alpha1.GroupVersion.String(), "VariantAutoscaling"); ok && err == nil {
		b = b.Owns(&wvav1alpha1.VariantAutoscaling{}, builder.WithPredicates(childResourcesPredicate))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), kedav1alpha1.SchemeGroupVersion.String(), "ScaledObject"); ok && err == nil {
		b = b.Owns(&kedav1alpha1.ScaledObject{}, builder.WithPredicates(childResourcesPredicate))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), lwsapi.GroupVersion.String(), "LeaderWorkerSet"); ok && err == nil {
		b = b.Owns(&lwsapi.LeaderWorkerSet{}, builder.WithPredicates(childResourcesPredicate))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), resourcev1.SchemeGroupVersion.String(), "ResourceClaimTemplate"); ok && err == nil {
		b = b.Owns(&resourcev1.ResourceClaimTemplate{}, builder.WithPredicates(childResourcesPredicate))
	}

	return b.Complete(r)
}

// enqueueOnGatewayChange creates an event handler that triggers reconciliation of LLMInferenceServices
// when a referenced Gateway changes. This ensures routing is updated when Gateway status changes.
func (r *LLMISVCReconciler) enqueueOnGatewayChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnGatewayChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*gwapiv1.Gateway)
		reqs := make([]reconcile.Request, 0, 2)

		listNamespace := corev1.NamespaceAll

		cfg, err := r.loadConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When a Gateway is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}
		for _, llmSvc := range llmSvcList.Items {
			if hasRoutingGatewayRef(&llmSvc, gwapiv1.ObjectName(sub.Name), gwapiv1.Namespace(sub.Namespace)) {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: llmSvc.Namespace,
					Name:      llmSvc.Name,
				}})
				continue // skip the expensive combineBaseRefsConfig fallback
			}

			// Fallback: service created before status.routing was introduced.
			// Use the old derivation path until it reconciles and populates status.
			llmSvcCopy := llmSvc.DeepCopy()
			result, err := r.combineBaseRefsConfig(ctx, llmSvcCopy, cfg)
			if err != nil {
				logger.Error(err, "Failed to combine base refs config", "llmSvc", llmSvc.Name)
				continue
			}

			combinedCfg := result.Config.Spec

			// Skip services that don't use gateways
			if combinedCfg.Router == nil || combinedCfg.Router.Gateway == nil {
				continue
			}

			// Check if service uses the global default gateway
			if !combinedCfg.Router.Gateway.HasRefs() && sub.Name == cfg.IngressGatewayName && sub.Namespace == cfg.IngressGatewayNamespace {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: llmSvc.Namespace,
					Name:      llmSvc.Name,
				}})
				continue
			}

			// Check if service explicitly references this gateway
			for _, ref := range combinedCfg.Router.Gateway.Refs {
				refNamespace := string(ref.Namespace)
				if refNamespace == "" {
					refNamespace = llmSvc.Namespace
				}
				if string(ref.Name) == sub.Name && refNamespace == sub.Namespace {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
						Namespace: llmSvc.Namespace,
						Name:      llmSvc.Name,
					}})
				}
			}
		}

		return reqs
	})
}

// enqueueOnHttpRouteChange creates an event handler that triggers reconciliation of LLMInferenceServices
// when a referenced HTTPRoute changes. This ensures routing is updated when HTTPRoute status changes.
func (r *LLMISVCReconciler) enqueueOnHttpRouteChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnHttpRouteChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*gwapiv1.HTTPRoute)
		reqs := make([]reconcile.Request, 0, 2)

		listNamespace := corev1.NamespaceAll

		cfg, err := r.loadConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When an HTTPRoute is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}
		for _, llmSvc := range llmSvcList.Items {
			if hasRoutingHTTPRouteRef(&llmSvc, gwapiv1.ObjectName(sub.Name), sub.Namespace) {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: llmSvc.Namespace,
					Name:      llmSvc.Name,
				}})
				continue
			}

			// Fallback: service created before status.routing was introduced.
			// Use the old derivation path until it reconciles and populates status.
			llmSvcCopy := llmSvc.DeepCopy()
			result, err := r.combineBaseRefsConfig(ctx, llmSvcCopy, cfg)
			if err != nil {
				logger.Error(err, "Failed to combine base refs config", "llmSvc", llmSvc.Name)
				continue
			}

			combinedCfg := result.Config.Spec

			// Skip services that don't use HTTPRoute refs
			if combinedCfg.Router == nil || combinedCfg.Router.Route == nil || !combinedCfg.Router.Route.HTTP.HasRefs() {
				continue
			}

			// Check if service explicitly references this HTTPRoute
			for _, ref := range combinedCfg.Router.Route.HTTP.Refs {
				if ref.Name == sub.Name && sub.Namespace == llmSvc.Namespace {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
						Namespace: llmSvc.Namespace,
						Name:      llmSvc.Name,
					}})
					break
				}
			}
		}

		return reqs
	})
}

// enqueueOnInferencePoolChange creates an event handler that triggers reconciliation of
// LLMInferenceServices that reference an external InferencePool via scheduler.pool.ref.
// Managed pools are already covered by Owns(...) watches.
func (r *LLMISVCReconciler) enqueueOnInferencePoolChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnInferencePoolChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		// Intentionally client.Object - this handler is registered for both igwapi.InferencePool (v1) and igwapiv1alpha2.InferencePool.
		sub := object
		reqs := make([]reconcile.Request, 0, 2)
		listNamespace := sub.GetNamespace()

		cfg, err := r.loadConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When an InferencePool is modified, we need to find all LLMInferenceService instances that might
		// depend on it through scheduler.pool.ref and trigger their reconciliation.
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}
		for _, llmSvc := range llmSvcList.Items {
			llmSvcCopy := llmSvc.DeepCopy()
			result, err := r.combineBaseRefsConfig(ctx, llmSvcCopy, cfg)
			if err != nil {
				logger.Error(err, "Failed to combine base refs config", "llmSvc", llmSvc.Name)
				continue
			}

			combinedCfg := result.Config.Spec
			if combinedCfg.Router == nil ||
				combinedCfg.Router.Scheduler == nil ||
				combinedCfg.Router.Scheduler.Pool == nil ||
				combinedCfg.Router.Scheduler.Pool.Ref == nil ||
				combinedCfg.Router.Scheduler.Pool.Ref.Name != sub.GetName() {
				continue
			}

			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: llmSvc.Namespace,
				Name:      llmSvc.Name,
			}})
		}

		return reqs
	})
}

// enqueueOnLLMInferenceServiceConfigChange triggers reconciliation of every LLMInferenceService
// that used (or may use) the changed config - matched via status.appliedConfigs, spec.baseRefs,
// or well-known config membership.
func (r *LLMISVCReconciler) enqueueOnLLMInferenceServiceConfigChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnLLMInferenceServiceConfigChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*v1alpha2.LLMInferenceServiceConfig)
		reqs := make([]reconcile.Request, 0, 2)

		listNamespace := sub.GetNamespace()

		// System namespace configs are global and can affect services in any namespace
		if sub.Namespace == constants.KServeNamespace {
			listNamespace = corev1.NamespaceAll
		}

		// Find all LLMInferenceService instances that depend on (or may depend on) this config.
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}
		for _, llmSvc := range llmSvcList.Items {
			// Check status.appliedConfigs first (populated on success, retained
			// when stopped), then fall back to annotations/baseRefs.
			if llmSvc.IsUsingLLMInferenceServiceConfig(sub.Name) {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: llmSvc.Namespace,
					Name:      llmSvc.Name,
				}})
				continue
			}

			// Fallback when appliedConfigs is empty (not yet reconciled, or
			// cleared after a config-merge error): enqueue if the changed
			// config is a well-known default that could apply to this service.
			if len(llmSvc.Status.AppliedConfigRefs) == 0 &&
				WellKnownDefaultConfigs.Has(sub.Name) &&
				(sub.Namespace == constants.KServeNamespace || sub.Namespace == llmSvc.Namespace) {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: llmSvc.Namespace,
					Name:      llmSvc.Name,
				}})
			}
		}

		return reqs
	})
}

func hasRoutingGatewayRef(llmSvc *v1alpha2.LLMInferenceService, gatewayName gwapiv1.ObjectName, gatewayNamespace gwapiv1.Namespace) bool {
	if llmSvc.Status.Router == nil || len(llmSvc.Status.Router.Gateways) == 0 {
		return false
	}

	for _, gw := range llmSvc.Status.Router.Gateways {
		if string(gw.Name) == string(gatewayName) &&
			gw.Namespace != nil && string(*gw.Namespace) == string(gatewayNamespace) {
			return true
		}
	}

	return false
}

func hasRoutingHTTPRouteRef(llmSvc *v1alpha2.LLMInferenceService, routeName gwapiv1.ObjectName, routeNamespace string) bool {
	if llmSvc.Status.Router == nil || len(llmSvc.Status.Router.Gateways) == 0 {
		return false
	}

	for _, gw := range llmSvc.Status.Router.Gateways {
		for _, route := range gw.HTTPRoutes {
			if string(route.Name) == string(routeName) &&
				route.Namespace != nil && string(*route.Namespace) == routeNamespace {
				return true
			}
		}
	}

	return false
}

func (r *LLMISVCReconciler) enqueueOnConfigMapChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnConfigMapChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*corev1.ConfigMap)
		reqs := make([]reconcile.Request, 0)

		listNamespace := sub.GetNamespace()

		cfg, err := r.loadConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When a ConfigMap is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}

		for _, llmSvc := range llmSvcList.Items {
			// Use WithSkipClearSchedulerConfigRef to preserve the Ref for matching
			result, err := r.combineBaseRefsConfig(ctx, &llmSvc, cfg, WithSkipClearSchedulerConfigRef())
			if err != nil {
				logger.Error(err, "Failed to combine baseRefs config", "namespace", llmSvc.Namespace, "name", llmSvc.Name)
				continue
			}

			combinedCfg := result.Config.Spec

			if combinedCfg.Router == nil ||
				combinedCfg.Router.Scheduler == nil ||
				combinedCfg.Router.Scheduler.Config == nil ||
				combinedCfg.Router.Scheduler.Config.Ref == nil ||
				combinedCfg.Router.Scheduler.Config.Ref.Name != sub.Name {
				continue
			}

			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: llmSvc.Namespace,
				Name:      llmSvc.Name,
			}})
		}

		return reqs
	})
}

func (r *LLMISVCReconciler) enqueueOnInferenceServiceConfigMapChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnInferenceServiceConfigMapChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		reqs := make([]reconcile.Request, 0)
		if !isInferenceServiceConfigMap(object) {
			return reqs
		}

		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{Namespace: corev1.NamespaceAll}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}

		for _, llmSvc := range llmSvcList.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: llmSvc.Namespace,
				Name:      llmSvc.Name,
			}})
		}

		return reqs
	})
}

// EnqueueOnLLMInferenceServicePods maps pod events to LLMInferenceService reconcile requests.
// It extracts the owning LLMInferenceService name from pod labels.
func (r *LLMISVCReconciler) EnqueueOnLLMInferenceServicePods(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok || pod == nil {
		return nil
	}
	// Cache is already restricted to pods with part-of label (see cmd/llmisvc main.go).
	// Get the LLMInferenceService name from the name label.
	if llmSvcName, found := pod.Labels[constants.KubernetesAppNameLabelKey]; found && llmSvcName != "" {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      llmSvcName,
			},
		}}
	}
	return nil
}

// PodStatusPredicate filters pod updates to those where InitContainerStatuses or PodIPs changed.
// Pod identity (part-of/name labels) is enforced by the cache (cmd/llmisvc) and EnqueueOnLLMInferenceServicePods.
func PodStatusPredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newPod, ok := e.ObjectNew.(*corev1.Pod)
			if !ok || newPod == nil {
				return false
			}
			oldPod := e.ObjectOld.(*corev1.Pod)
			initContainersChanged := !equality.Semantic.DeepEqual(
				oldPod.Status.InitContainerStatuses,
				newPod.Status.InitContainerStatuses,
			)
			podIPsChanged := !equality.Semantic.DeepEqual(
				oldPod.Status.PodIPs,
				newPod.Status.PodIPs,
			)
			return initContainersChanged || podIPsChanged
		},
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

// ociImageVolumeCompatible is the advisory condition type surfaced on an
// LLMInferenceService when native OCI ImageVolume mode is in use and the cluster
// Kubernetes version may not support it.
const ociImageVolumeCompatible apis.ConditionType = "OciImageVolumeCompatible"

// serverVersioner is a single-method interface for K8s version discovery.
type serverVersioner interface {
	ServerVersion() (*version.Info, error)
}

// warnIfImageVolumeUnsupported mirrors the ISVC-controller helper for
// LLMInferenceService. Compatibility thresholds are handled by the shared helper
// utils.CheckImageVolumeCompatibility; this function translates the result into
// the LLMInferenceService condition format (MarkFalse path on its conditionSet).
func warnIfImageVolumeUnsupported(ctx context.Context, sv serverVersioner, llmSvc *v1alpha2.LLMInferenceService, resolvedMode string) {
	mgr := llmSvc.GetConditionSet().Manage(llmSvc.GetStatus())

	if resolvedMode != kserveTypes.OciModelModeNative {
		_ = mgr.ClearCondition(ociImageVolumeCompatible)
		return
	}

	result := utils.CheckImageVolumeCompatibility(ctx, sv)

	switch result.Status {
	case utils.ImageVolumeUnsupported:
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeUnsupported",
			"Cluster K8s %s.%s does not support ImageVolume (introduced in 1.31 as alpha). Falling back to modelcar may be required.",
			result.Major, result.Minor)
	case utils.ImageVolumeSubPathUnsupported:
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeSubPathUnsupported",
			"Cluster K8s %s.%s (alpha) does not support subPath on ImageVolume VolumeMounts. Upgrade to K8s 1.33+ (beta) for full oci+native:// support.",
			result.Major, result.Minor)
	case utils.ImageVolumeNeedsGate:
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeAlpha",
			"Cluster K8s %s.%s has ImageVolume feature-gated (K8s 1.33–1.34 beta). Ensure --feature-gates=ImageVolume=true is set on kube-apiserver and kubelet.",
			result.Major, result.Minor)
	default:
		// ImageVolumeOK (≥ 1.35) or ImageVolumeUnknown — clear any previous warning.
		_ = mgr.ClearCondition(ociImageVolumeCompatible)
	}
}
