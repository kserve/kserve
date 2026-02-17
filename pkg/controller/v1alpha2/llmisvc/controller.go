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
	"fmt"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/cabundleconfigmap"

	"knative.dev/pkg/reconciler"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"

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
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/utils"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
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

// LLMISVCReconciler reconciles an LLMInferenceService object.
// It orchestrates the reconciliation of child resources based on the spec.
type LLMISVCReconciler struct {
	client.Client
	record.EventRecorder
	Clientset kubernetes.Interface

	Validator func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error

	// InferencePool CRD availability flags (set during SetupWithManager)
	// These determine which pool versions can be created/managed
	InferencePoolV1Available       bool
	InferencePoolV1Alpha2Available bool
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
//+kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencepools;inferenceobjectives,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=inference.networking.k8s.io,resources=inferencepools;inferenceobjectives,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews;subjectaccessreviews,verbs=create
//+kubebuilder:rbac:urls=/metrics,verbs=get
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

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
			if cleanupErr := r.finalize(ctx, original); cleanupErr != nil {
				logger.Error(cleanupErr, "Finalization failed")
				return ctrl.Result{}, cleanupErr
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

	// Load global configuration from KServe configmap
	// TODO(ctrl): add watch on CfgMap with predicate and cache tuning to trigger reconcile when it changes
	config, configErr := LoadConfig(ctx, r.Clientset)
	if configErr != nil {
		return fmt.Errorf("failed to load ingress config: %w", configErr)
	}

	// Combine base configurations with service-specific overrides
	// This includes default configs based on deployment pattern (single node, multi-node, etc.)
	baseCfg, err := r.combineBaseRefsConfig(ctx, llmSvc, config)
	if err != nil {
		llmSvc.MarkPresetsCombinedNotReady("CombineBaseError", err.Error())
		return fmt.Errorf("failed to combine base-configurations: %w", err)
	}
	llmSvc.MarkPresetsCombinedReady()

	logger.V(2).Info("Reconciling with combined base configurations", "combined.spec", baseCfg.Spec, "original.spec", llmSvc.Spec)
	// Replace the spec with the merged configuration for reconciliation
	// We are only writing to status, so we can safely use the original object.
	llmSvc.Spec = baseCfg.Spec

	if err := r.reconcileWorkload(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile workload: %w", err)
	}

	if err := r.reconcileRouter(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile networking: %w", err)
	}

	return nil
}

// finalize performs cleanup operations when the LLMInferenceService is being deleted
func (r *LLMISVCReconciler) finalize(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to finalize scheduler service account: %w", err)
	}

	return nil
}

// updateStatus updates the status of the LLMInferenceService with retry on conflict
// This prevents race conditions when multiple controllers update the same resource
func (r *LLMISVCReconciler) updateStatus(ctx context.Context, desired *v1alpha2.LLMInferenceService) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Always fetch the latest version to avoid conflicts
		latest := &v1alpha2.LLMInferenceService{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(desired), latest); err != nil {
			return err
		}

		// Skip update if status hasn't changed
		if equality.Semantic.DeepEqual(latest.Status, desired.Status) {
			return nil
		}

		latest.Status = desired.Status

		if err := r.Client.Status().Update(ctx, latest); err != nil {
			return fmt.Errorf("failed to update status for LLMInferenceService: %w", err)
		}

		return nil
	})
}

// SetupWithManager sets up the controller with the Manager.
// It configures watches for the LLMInferenceService and its owned resources.
// The controller conditionally registers watchers based on CRD availability to avoid errors in environments where optional CRDs are not installed.
func (r *LLMISVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("LLMInferenceService.SetupWithManager")

	b := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.LLMInferenceService{}).
		Watches(&v1alpha2.LLMInferenceServiceConfig{}, r.enqueueOnLLMInferenceServiceConfigChange(logger)).
		Owns(&netv1.Ingress{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&corev1.Secret{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&corev1.Service{}, builder.WithPredicates(childResourcesPredicate)).
		Watches(&corev1.ConfigMap{}, r.enqueueOnConfigMapChange(logger)).
		Watches(&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.PodInitContainersFunc),
			builder.WithPredicates(PodInitContainersPredicate()))

	if err := gwapiv1.Install(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add GIE APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), gwapiv1.GroupVersion.String(), "HTTPRoute"); ok && err == nil {
		b = b.Owns(&gwapiv1.HTTPRoute{}, builder.WithPredicates(childResourcesPredicate)).
			Watches(&gwapiv1.HTTPRoute{}, r.enqueueOnHttpRouteChange(logger))
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), gwapiv1.GroupVersion.String(), "Gateway"); ok && err == nil {
		b = b.Watches(&gwapiv1.Gateway{}, r.enqueueOnGatewayChange(logger))
	}

	// Install GIE v1 API and check availability
	if err := igwapi.Install(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add GIE v1 APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), igwapi.GroupVersion.String(), "InferencePool"); ok && err == nil {
		r.InferencePoolV1Available = true
		b = b.Owns(&igwapi.InferencePool{}, builder.WithPredicates(childResourcesPredicate))
	}

	// Install GIE v1alpha2 API and check availability (for backwards compatibility during migration)
	if err := igwapiv1alpha2.Install(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add GIE v1alpha2 APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), igwapiv1alpha2.GroupVersion.String(), "InferencePool"); ok && err == nil {
		r.InferencePoolV1Alpha2Available = true
		b = b.Owns(&igwapiv1alpha2.InferencePool{}, builder.WithPredicates(childResourcesPredicate))
	}

	logger.Info("InferencePool CRD availability", "v1", r.InferencePoolV1Available, "v1alpha2", r.InferencePoolV1Alpha2Available)

	if err := lwsapi.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add LeaderWorkerSet APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), lwsapi.GroupVersion.String(), "LeaderWorkerSet"); ok && err == nil {
		b = b.Owns(&lwsapi.LeaderWorkerSet{}, builder.WithPredicates(childResourcesPredicate))
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

		cfg, err := LoadConfig(ctx, r.Clientset)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When a Gateway is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		// Use pagination to handle large numbers of services efficiently
		continueToken := ""
		for {
			llmSvcList := &v1alpha2.LLMInferenceServiceList{}
			if err := r.Client.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace, Continue: continueToken}); err != nil {
				logger.Error(err, "Failed to list LLMInferenceService")
				return reqs
			}
			for _, llmSvc := range llmSvcList.Items {
				// Use a deep copy to avoid modifying the original object
				llmSvcCopy := llmSvc.DeepCopy()
				combinedCfg, err := r.combineBaseRefsConfig(ctx, llmSvcCopy, cfg)
				if err != nil {
					logger.Error(err, "Failed to combine base refs config", "llmSvc", llmSvc.Name)
					continue
				}

				// Skip services that don't use gateways
				if combinedCfg.Spec.Router == nil || combinedCfg.Spec.Router.Gateway == nil {
					continue
				}

				// Check if service uses the global default gateway
				if !combinedCfg.Spec.Router.Gateway.HasRefs() && sub.Name == cfg.IngressGatewayName && sub.Namespace == cfg.IngressGatewayNamespace {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
						Namespace: llmSvc.Namespace,
						Name:      llmSvc.Name,
					}})
					continue
				}

				// Check if service explicitly references this gateway
				for _, ref := range combinedCfg.Spec.Router.Gateway.Refs {
					if string(ref.Name) == sub.Name && string(ref.Namespace) == sub.Namespace {
						reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
							Namespace: llmSvc.Namespace,
							Name:      llmSvc.Name,
						}})
					}
				}
			}

			if llmSvcList.Continue == "" {
				break
			}
			continueToken = llmSvcList.Continue
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

		cfg, err := LoadConfig(ctx, r.Clientset)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When an HTTPRoute is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		// Use pagination to handle large numbers of services efficiently
		continueToken := ""
		for {
			llmSvcList := &v1alpha2.LLMInferenceServiceList{}
			if err := r.Client.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace, Continue: continueToken}); err != nil {
				logger.Error(err, "Failed to list LLMInferenceService")
				return reqs
			}
			for _, llmSvc := range llmSvcList.Items {
				// Use a deep copy to avoid modifying the original object
				llmSvcCopy := llmSvc.DeepCopy()
				combinedCfg, err := r.combineBaseRefsConfig(ctx, llmSvcCopy, cfg)
				if err != nil {
					logger.Error(err, "Failed to combine base refs config", "llmSvc", llmSvc.Name)
					continue
				}

				// Skip services that don't use HTTPRoute refs
				if combinedCfg.Spec.Router == nil || combinedCfg.Spec.Router.Route == nil || !combinedCfg.Spec.Router.Route.HTTP.HasRefs() {
					continue
				}

				// Check if service explicitly references this HTTPRoute
				for _, ref := range combinedCfg.Spec.Router.Route.HTTP.Refs {
					if ref.Name == sub.Name && sub.Namespace == llmSvc.Namespace {
						reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
							Namespace: llmSvc.Namespace,
							Name:      llmSvc.Name,
						}})
						break
					}
				}
			}

			if llmSvcList.Continue == "" {
				break
			}
			continueToken = llmSvcList.Continue
		}

		return reqs
	})
}

// enqueueOnLLMInferenceServiceConfigChange creates an event handler that triggers reconciliation of LLMInferenceServices
// when a referenced LLMInferenceServiceConfig changes. This ensures services are updated when their base configs change.
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

		// When an LLMInferenceServiceConfig is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		continueToken := ""
		for {
			llmSvcList := &v1alpha2.LLMInferenceServiceList{}
			if err := r.Client.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace, Continue: continueToken}); err != nil {
				logger.Error(err, "Failed to list LLMInferenceService")
				return reqs
			}
			for _, llmSvc := range llmSvcList.Items {
				// Check if this is a well-known config template that services automatically inherit
				if WellKnownDefaultConfigs.Has(sub.Name) && (sub.Namespace == constants.KServeNamespace || sub.Namespace == llmSvc.Namespace) {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
						Namespace: llmSvc.Namespace,
						Name:      llmSvc.Name,
					}})
					continue
				}

				// Check if service explicitly references this config
				for _, ref := range llmSvc.Spec.BaseRefs {
					if ref.Name == sub.Name {
						reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
							Namespace: llmSvc.Namespace,
							Name:      llmSvc.Name,
						}})
					}
				}
			}

			if llmSvcList.Continue == "" {
				break
			}
			continueToken = llmSvcList.Continue
		}

		return reqs
	})
}

func (r *LLMISVCReconciler) enqueueOnConfigMapChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnConfigMapChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*corev1.ConfigMap)
		reqs := make([]reconcile.Request, 0)

		listNamespace := sub.GetNamespace()

		cfg, err := LoadConfig(ctx, r.Clientset)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// System namespace configs are global and can affect services in any namespace
		if sub.Namespace == constants.KServeNamespace {
			listNamespace = corev1.NamespaceAll
		}

		// When a ConfigMap is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.Client.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace}); err != nil {
			logger.Error(err, "Failed to list LLMInferenceService")
			return reqs
		}

		for _, llmSvc := range llmSvcList.Items {
			// Use WithSkipClearSchedulerConfigRef to preserve the Ref for matching
			resolved, err := r.combineBaseRefsConfig(ctx, &llmSvc, cfg, WithSkipClearSchedulerConfigRef())
			if err != nil {
				logger.Error(err, "Failed to combine baseRefs config", "namespace", llmSvc.Namespace, "name", llmSvc.Name)
				continue
			}

			if resolved.Spec.Router == nil ||
				resolved.Spec.Router.Scheduler == nil ||
				resolved.Spec.Router.Scheduler.Config == nil ||
				resolved.Spec.Router.Scheduler.Config.Ref == nil ||
				resolved.Spec.Router.Scheduler.Config.Ref.Name != sub.Name {
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

// PodInitContainersFunc maps pod events to LLMInferenceService reconcile requests.
// It extracts the owning LLMInferenceService name from pod labels.
func (r *LLMISVCReconciler) PodInitContainersFunc(ctx context.Context, obj client.Object) []reconcile.Request {
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

// PodInitContainersPredicate filters pod updates to those where InitContainerStatuses changed.
// Pod identity (part-of/name labels) is enforced by the cache (cmd/llmisvc) and PodInitContainersFunc.
func PodInitContainersPredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newPod, ok := e.ObjectNew.(*corev1.Pod)
			if !ok || newPod == nil {
				return false
			}
			oldPod := e.ObjectOld.(*corev1.Pod)
			return !equality.Semantic.DeepEqual(
				oldPod.Status.InitContainerStatuses,
				newPod.Status.InitContainerStatuses,
			)
		},
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}
