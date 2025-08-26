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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"k8s.io/apimachinery/pkg/api/equality"

	"github.com/kserve/kserve/pkg/constants"

	"knative.dev/pkg/reconciler"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/utils"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

var childResourcesPredicate, _ = predicate.LabelSelectorPredicate(metav1.LabelSelector{
	MatchLabels: map[string]string{
		"app.kubernetes.io/part-of": "llminferenceservice",
	},
})

// LLMISVCReconciler reconciles an LLMInferenceService object.
// It orchestrates the reconciliation of child resources based on the spec.
type LLMISVCReconciler struct {
	client.Client
	record.EventRecorder
	Clientset kubernetes.Interface
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
//+kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencepools;inferencemodels;,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews;subjectaccessreviews,verbs=create
//+kubebuilder:rbac:urls=/metrics,verbs=get

// Reconcile is the main entry point for the reconciliation loop.
// It fetches the LLMInferenceService and delegates the reconciliation of its constituent parts.
func (r *LLMISVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("LLMInferenceService")
	ctx = log.IntoContext(ctx, logger)

	logger.Info("Starting reconciliation")
	original := &v1alpha1.LLMInferenceService{}
	if err := r.Get(ctx, req.NamespacedName, original); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := constants.KServeAPIGroupName + "/llmisvc-finalizer"
	if original.DeletionTimestamp.IsZero() {
		if controllerutil.AddFinalizer(original, finalizerName) {
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		logger.Info("Marked for deletion, finalizing resources")
		if controllerutil.ContainsFinalizer(original, finalizerName) {
			if cleanupErr := r.finalize(ctx, original); cleanupErr != nil {
				logger.Error(cleanupErr, "Finalization failed")
				return ctrl.Result{}, cleanupErr
			}

			controllerutil.RemoveFinalizer(original, finalizerName)
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Do not reconcile, because llmisvc is being deleted.
		return ctrl.Result{}, nil
	}

	resource := original.DeepCopy()

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

func (r *LLMISVCReconciler) reconcile(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("reconcile")
	ctx = log.IntoContext(ctx, logger)

	// TODO(ctrl): add watch on CfgMap with predicate and cache tuning to trigger reconcile when it changes
	config, configErr := LoadConfig(ctx, r.Clientset)
	if configErr != nil {
		return fmt.Errorf("failed to load ingress config: %w", configErr)
	}

	baseCfg, err := r.combineBaseRefsConfig(ctx, llmSvc, config)
	if err != nil {
		llmSvc.MarkPresetsCombinedNotReady("CombineBaseError", err.Error())
		return fmt.Errorf("failed to combine base-configurations: %w", err)
	}
	llmSvc.MarkPresetsCombinedReady()

	logger.Info("Reconciling with combined base configurations", "combined.spec", baseCfg.Spec, "original.spec", llmSvc.Spec)
	// We are only writing to status, so we can safely use the original object.
	llmSvc.Spec = baseCfg.Spec

	if err := r.reconcileWorkload(ctx, llmSvc, config.StorageConfig, config.CredentialConfig); err != nil {
		return fmt.Errorf("failed to reconcile workload: %w", err)
	}

	if err := r.reconcileRouter(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile networking: %w", err)
	}

	return nil
}

func (r *LLMISVCReconciler) finalize(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to finalize scheduler service account: %w", err)
	}

	return nil
}

func (r *LLMISVCReconciler) updateStatus(ctx context.Context, desired *v1alpha1.LLMInferenceService) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1alpha1.LLMInferenceService{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(desired), latest); err != nil {
			return err
		}

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
func (r *LLMISVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("LLMInferenceService.SetupWithManager")

	b := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LLMInferenceService{}).
		Watches(&v1alpha1.LLMInferenceServiceConfig{}, r.enqueueOnLLMInferenceServiceConfigChange(logger)).
		Owns(&netv1.Ingress{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&corev1.Secret{}, builder.WithPredicates(childResourcesPredicate)).
		Owns(&corev1.Service{}, builder.WithPredicates(childResourcesPredicate))

	if err := gatewayapi.Install(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add GIE APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), gatewayapi.GroupVersion.String(), "HTTPRoute"); ok && err == nil {
		b = b.Owns(&gatewayapi.HTTPRoute{}, builder.WithPredicates(childResourcesPredicate))
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), gatewayapi.GroupVersion.String(), "Gateway"); ok && err == nil {
		b = b.Watches(&gatewayapi.Gateway{}, r.enqueueOnGatewayChange(logger))
	}

	if err := igwapi.Install(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add GIE APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), igwapi.GroupVersion.String(), "InferencePool"); ok && err == nil {
		b = b.Owns(&igwapi.InferencePool{}, builder.WithPredicates(childResourcesPredicate))
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), igwapi.GroupVersion.String(), "InferenceModel"); ok && err == nil {
		b = b.Owns(&igwapi.InferenceModel{}, builder.WithPredicates(childResourcesPredicate))
	}

	if err := lwsapi.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add LeaderWorkerSet APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), lwsapi.GroupVersion.String(), "LeaderWorkerSet"); ok && err == nil {
		b = b.Owns(&lwsapi.LeaderWorkerSet{}, builder.WithPredicates(childResourcesPredicate))
	}

	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), monitoringv1.SchemeGroupVersion.String(), "ServiceMonitor"); ok && err == nil {
		b = b.Owns(&monitoringv1.ServiceMonitor{}, builder.WithPredicates(childResourcesPredicate))
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), monitoringv1.SchemeGroupVersion.String(), "PodMonitor"); ok && err == nil {
		b = b.Owns(&monitoringv1.PodMonitor{}, builder.WithPredicates(childResourcesPredicate))
	}

	return b.Complete(r)
}

func (r *LLMISVCReconciler) enqueueOnGatewayChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnGatewayChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*gatewayapi.Gateway)
		reqs := make([]reconcile.Request, 0, 2)

		listNamespace := corev1.NamespaceAll

		cfg, err := LoadConfig(ctx, r.Clientset)
		if err != nil {
			logger.Error(err, "Failed to load config")
			return reqs
		}

		// When a Gateway is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		continueToken := ""
		for {
			llmSvcList := &v1alpha1.LLMInferenceServiceList{}
			if err := r.Client.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace, Continue: continueToken}); err != nil {
				logger.Error(err, "Failed to list LLMInferenceService")
				return reqs
			}
			for _, llmSvc := range llmSvcList.Items {
				// If it's not using the router or gateway, skip the resource.
				if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Gateway == nil {
					continue
				}

				// If the LLMInferenceService is using the global gateway, requeue the resource.
				if !llmSvc.Spec.Router.Gateway.HasRefs() && sub.Name == cfg.IngressGatewayName && sub.Namespace == cfg.IngressGatewayNamespace {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
						Namespace: llmSvc.Namespace,
						Name:      llmSvc.Name,
					}})
					continue
				}

				for _, ref := range llmSvc.Spec.Router.Gateway.Refs {
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

func (r *LLMISVCReconciler) enqueueOnLLMInferenceServiceConfigChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnLLMInferenceServiceConfigChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		sub := object.(*v1alpha1.LLMInferenceServiceConfig)
		reqs := make([]reconcile.Request, 0, 2)

		listNamespace := sub.GetNamespace()

		// LLMInferenceServiceConfig in the system namespace can be used by any LLMInferenceService.
		if sub.Namespace == constants.KServeNamespace {
			listNamespace = corev1.NamespaceAll
		}

		// When an LLMInferenceServiceConfig is modified, we need to find all LLMInferenceService instances that might
		// depend on it and trigger their reconciliation.
		continueToken := ""
		for {
			llmSvcList := &v1alpha1.LLMInferenceServiceList{}
			if err := r.Client.List(ctx, llmSvcList, &client.ListOptions{Namespace: listNamespace, Continue: continueToken}); err != nil {
				logger.Error(err, "Failed to list LLMInferenceService")
				return reqs
			}
			for _, llmSvc := range llmSvcList.Items {
				// If the mutated LLMInferenceServiceConfig is a well-known template and is in the system or
				// LLMInferenceService namespace, we need to re-queue the specific LLMInferenceService.
				if WellKnownDefaultConfigs.Has(sub.Name) && (sub.Namespace == constants.KServeNamespace || sub.Namespace == llmSvc.Namespace) {
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
						Namespace: llmSvc.Namespace,
						Name:      llmSvc.Name,
					}})
					continue
				}

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
