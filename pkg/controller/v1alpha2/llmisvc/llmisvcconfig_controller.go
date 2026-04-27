/*
Copyright 2026 The KServe Authors.

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
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// LLMISVCConfigReconciler reconciles LLMInferenceServiceConfig objects.
// It manages a finalizer to prevent deletion of configs that are still
// referenced by LLMInferenceService instances via spec.baseRefs or status.annotations.
type LLMISVCConfigReconciler struct {
	client.Client
	record.EventRecorder
}

//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices,verbs=get;list;watch

// Reconcile manages the finalizer on LLMInferenceServiceConfig resources to prevent
// deletion while they are still referenced by LLMInferenceService instances.
// It also updates the config's status conditions to surface why deletion is blocked,
// similar to how Kubernetes namespaces report finalizer status during termination.
func (r *LLMISVCConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("LLMInferenceServiceConfig").
		WithValues("Namespace", req.Namespace, "Name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	config := &v1alpha2.LLMInferenceServiceConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := constants.KServeAPIGroupName + "/llmisvcconfig-finalizer"

	if config.DeletionTimestamp.IsZero() {
		// Resource is not being deleted, ensure finalizer is present and mark Ready
		if controllerutil.AddFinalizer(config, finalizerName) {
			if err := r.Update(ctx, config); err != nil {
				return ctrl.Result{}, err
			}
		}

		config.MarkReady()
		if err := r.updateStatus(ctx, config); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Resource is being deleted
	if !controllerutil.ContainsFinalizer(config, finalizerName) {
		return ctrl.Result{}, nil
	}

	inUse, referencing, err := r.isConfigInUse(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check if config is in use: %w", err)
	}

	if inUse {
		msg := "still referenced by LLMInferenceService(s): " + strings.Join(referencing, ", ")

		logger.Info("LLMInferenceServiceConfig is still referenced, blocking deletion",
			"referencedBy", referencing)
		r.Eventf(config, corev1.EventTypeWarning, "DeletionBlocked",
			"Cannot delete LLMInferenceServiceConfig %s/%s: %s",
			config.Namespace, config.Name, msg)

		config.MarkNotReady("DeletionBlocked", msg)
		if err := r.updateStatus(ctx, config); err != nil {
			return ctrl.Result{}, err
		}

		// Requeue as a safety net in case a watch event is missed (e.g. a baseRef removal
		// from an LLMInferenceService update where only the new object is observed).
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// No longer in use, remove finalizer to allow deletion
	logger.Info("LLMInferenceServiceConfig is no longer referenced, allowing deletion")
	controllerutil.RemoveFinalizer(config, finalizerName)
	if err := r.Update(ctx, config); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the status of the LLMInferenceServiceConfig with retry on conflict.
func (r *LLMISVCConfigReconciler) updateStatus(ctx context.Context, desired *v1alpha2.LLMInferenceServiceConfig) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1alpha2.LLMInferenceServiceConfig{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(desired), latest); err != nil {
			return client.IgnoreNotFound(err)
		}

		if equality.Semantic.DeepEqual(latest.Status, desired.Status) {
			return nil
		}

		latest.Status = desired.Status

		if err := r.Client.Status().Update(ctx, latest); err != nil {
			return fmt.Errorf("failed to update status for LLMInferenceServiceConfig: %w", err)
		}
		return nil
	})
}

// isConfigInUse checks if any LLMInferenceService references this config
// via spec.baseRefs, status.annotations, or implicitly as a well-known default.
// It returns whether the config is in use and a list of referencing service names.
//
// Well-known default configs (those in WellKnownDefaultConfigs) are treated as implicitly
// referenced by all LLMInferenceService instances in the same namespace (or cluster-wide
// for system namespace configs). This is intentionally conservative: well-known configs
// are resolved implicitly by the controller, so any existing service could depend on them
// even without an explicit baseRef. Operators must drain all services before deleting a
// well-known config.
func (r *LLMISVCConfigReconciler) isConfigInUse(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig) (bool, []string, error) {
	// System namespace configs can be used by services in any namespace as a fallback.
	// Non-system namespace configs can only be used by services in the same namespace.
	listNamespace := config.Namespace
	if config.Namespace == constants.KServeNamespace {
		listNamespace = corev1.NamespaceAll
	}

	isWellKnown := WellKnownDefaultConfigs.Has(config.Name)

	var referencing []string
	continueToken := ""
	for {
		llmSvcList := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmSvcList, &client.ListOptions{
			Namespace: listNamespace,
			Continue:  continueToken,
		}); err != nil {
			return false, nil, fmt.Errorf("failed to list LLMInferenceService: %w", err)
		}

		for _, llmSvc := range llmSvcList.Items {
			// Well-known default configs are implicitly used by all services in the
			// same namespace (or all namespaces for system namespace configs).
			// This early return is intentional: since the config is implicitly available to
			// any service, we report one example and short-circuit to avoid scanning the entire list.
			if isWellKnown && (config.Namespace == constants.KServeNamespace || config.Namespace == llmSvc.Namespace) {
				return true, []string{fmt.Sprintf("%s/%s (and potentially others — well-known config)", llmSvc.Namespace, llmSvc.Name)}, nil
			}

			// Check explicit references via spec.baseRefs and status.annotations.
			// Status.Annotations stores versioned config resolution as {key: annotation-key, value: config-name},
			// so iterating values matches config names (consistent with IsUsingLLMInferenceServiceConfig).
			if llmSvc.IsUsingLLMInferenceServiceConfig(config.Name) {
				referencing = append(referencing, fmt.Sprintf("%s/%s", llmSvc.Namespace, llmSvc.Name))
			}
		}

		if llmSvcList.Continue == "" {
			break
		}
		continueToken = llmSvcList.Continue
	}

	return len(referencing) > 0, referencing, nil
}

// SetupWithManager sets up the controller with the Manager.
// It watches LLMInferenceServiceConfig as the primary resource and LLMInferenceService
// as a secondary resource to re-evaluate finalizers when service references change.
func (r *LLMISVCConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("LLMInferenceServiceConfig.SetupWithManager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.LLMInferenceServiceConfig{}).
		Watches(&v1alpha2.LLMInferenceService{}, r.enqueueOnLLMInferenceServiceChange(logger)).
		Complete(r)
}

// enqueueOnLLMInferenceServiceChange creates an event handler that enqueues
// LLMInferenceServiceConfig reconcile requests when an LLMInferenceService
// is created, updated, or deleted. This ensures finalizers are re-evaluated
// when services add or remove config references.
//
// Note: This handler only enqueues configs explicitly referenced in spec.baseRefs
// and status.annotations. Well-known default configs are NOT reactively enqueued here
// to avoid fanning out to all well-known configs on every service change. Instead,
// pending-deletion well-known configs rely on the RequeueAfter safety net (10s) in
// the Reconcile method to re-evaluate references.
func (r *LLMISVCConfigReconciler) enqueueOnLLMInferenceServiceChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnLLMInferenceServiceChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		llmSvc, ok := object.(*v1alpha2.LLMInferenceService)
		if !ok {
			return nil
		}

		seen := make(map[types.NamespacedName]struct{})
		var reqs []reconcile.Request

		enqueue := func(namespace, name string) {
			key := types.NamespacedName{Namespace: namespace, Name: name}
			if _, exists := seen[key]; exists {
				return
			}
			seen[key] = struct{}{}
			reqs = append(reqs, reconcile.Request{NamespacedName: key})
		}

		// Enqueue configs referenced in spec.baseRefs.
		// The config could be in the service's namespace or the system namespace.
		for _, ref := range llmSvc.Spec.BaseRefs {
			enqueue(llmSvc.Namespace, ref.Name)
			if llmSvc.Namespace != constants.KServeNamespace {
				enqueue(constants.KServeNamespace, ref.Name)
			}
		}

		// Enqueue configs referenced in status.annotations (versioned config resolution).
		// Status.Annotations is map[string]string where values are config names
		// (e.g., "kserve-config-llm-template"), consistent with IsUsingLLMInferenceServiceConfig.
		for _, name := range llmSvc.Status.Annotations {
			enqueue(llmSvc.Namespace, name)
			if llmSvc.Namespace != constants.KServeNamespace {
				enqueue(constants.KServeNamespace, name)
			}
		}

		if len(reqs) > 0 {
			logger.V(2).Info("Enqueuing LLMInferenceServiceConfig reconcile requests",
				"llmisvc", fmt.Sprintf("%s/%s", llmSvc.Namespace, llmSvc.Name),
				"requests", len(reqs))
		}

		return reqs
	})
}
