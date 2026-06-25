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
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/reconciler"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

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

	original := &v1alpha2.LLMInferenceServiceConfig{}
	if err := r.Get(ctx, req.NamespacedName, original); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := constants.KServeAPIGroupName + "/llmisvcconfig-finalizer"

	if original.DeletionTimestamp.IsZero() {
		if controllerutil.AddFinalizer(original, finalizerName) {
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(original, finalizerName) {
			return r.reconcileDelete(ctx, original, finalizerName)
		}
		return ctrl.Result{}, nil
	}

	resource := original.DeepCopy()

	reconciler.PreProcessReconcile(ctx, resource)
	reconcileErr := r.reconcile(ctx, resource)
	reconciler.PostProcessReconcile(ctx, resource, original)

	if reconcileErr != nil {
		logger.Error(reconcileErr, "Failed to reconcile LLMInferenceServiceConfig")
		r.Eventf(original, corev1.EventTypeWarning, "Error", "Reconciliation failed: %v", reconcileErr.Error())
	}

	if err := r.updateStatus(ctx, resource); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, reconcileErr
}

func (r *LLMISVCConfigReconciler) reconcile(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig) error {
	referencing, err := r.referencingServices(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to check if config is in use: %w", err)
	}

	config.Status.ReferencedBy = referencing

	if len(referencing) > 0 {
		config.MarkConfigInUse("InUse", "referenced by %d LLMInferenceService(s)", len(referencing))
	} else {
		config.MarkConfigNotInUse("NotInUse", "not referenced by any LLMInferenceService")
	}

	config.MarkReady()
	return nil
}

// reconcileDelete handles deletion of a config that still has the finalizer.
// It checks whether the config is still referenced by any LLMInferenceService,
// blocking deletion if so, or removing the finalizer to allow it.
func (r *LLMISVCConfigReconciler) reconcileDelete(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig, finalizerName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	referencing, err := r.referencingServices(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check if config is in use: %w", err)
	}

	if len(referencing) > 0 {
		logger.Info("LLMInferenceServiceConfig is still referenced, blocking deletion",
			"referencedBy", referencing)
		r.Eventf(config, corev1.EventTypeWarning, "DeletionBlocked",
			"Cannot delete LLMInferenceServiceConfig %s/%s: referenced by %d LLMInferenceService(s)",
			config.Namespace, config.Name, len(referencing))

		config.Status.ReferencedBy = referencing
		config.MarkConfigInUse("DeletionBlocked", "referenced by %d LLMInferenceService(s)", len(referencing))
		if err := r.updateStatus(ctx, config); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

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

// referencingServices returns the LLMInferenceService instances that reference this config
// via spec.baseRefs, status.annotations, or implicitly as a well-known default.
//
// Well-known default configs (those in WellKnownDefaultConfigs) are treated as implicitly
// referenced by all LLMInferenceService instances in the same namespace (or cluster-wide
// for system namespace configs). This is intentionally conservative: well-known configs
// are resolved implicitly by the controller, so any existing service could depend on them
// even without an explicit baseRef. Operators must drain all services before deleting a
// well-known config.
func (r *LLMISVCConfigReconciler) referencingServices(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig) ([]v1alpha2.UntypedObjectReference, error) {
	listNamespace := config.Namespace
	if config.Namespace == constants.KServeNamespace {
		listNamespace = corev1.NamespaceAll
	}

	isWellKnown := WellKnownDefaultConfigs.Has(config.Name)

	var referencing []v1alpha2.UntypedObjectReference
	llmSvcList := &v1alpha2.LLMInferenceServiceList{}
	if err := r.List(ctx, llmSvcList, &client.ListOptions{
		Namespace: listNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list LLMInferenceService: %w", err)
	}

	for i := range llmSvcList.Items {
		llmSvc := &llmSvcList.Items[i]
		if isWellKnown && (config.Namespace == constants.KServeNamespace || config.Namespace == llmSvc.Namespace) {
			referencing = append(referencing, svcRef(llmSvc))
			continue
		}

		if llmSvc.IsUsingLLMInferenceServiceConfigInNamespace(config.Name, config.Namespace) {
			referencing = append(referencing, svcRef(llmSvc))
		}
	}

	sort.Slice(referencing, func(i, j int) bool {
		if referencing[i].Namespace != referencing[j].Namespace {
			return referencing[i].Namespace < referencing[j].Namespace
		}
		return referencing[i].Name < referencing[j].Name
	})

	return referencing, nil
}

func svcRef(svc *v1alpha2.LLMInferenceService) v1alpha2.UntypedObjectReference {
	return v1alpha2.UntypedObjectReference{
		Name:      gwapiv1.ObjectName(svc.Name),
		Namespace: gwapiv1.Namespace(svc.Namespace),
	}
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
// For updates, EnqueueRequestsFromMapFunc calls the map function for both
// ObjectOld and ObjectNew, so configs from removed baseRefs are also enqueued.
//
// Well-known default configs are always enqueued because they are implicitly
// referenced by all services. The fan-out is bounded by the small, fixed size
// of WellKnownDefaultConfigs and each no-op reconciliation (config not pending
// deletion) is very cheap.
func (r *LLMISVCConfigReconciler) enqueueOnLLMInferenceServiceChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnLLMInferenceServiceChange")
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		llmSvc, ok := object.(*v1alpha2.LLMInferenceService)
		if !ok {
			return nil
		}

		seen := make(map[types.NamespacedName]struct{})
		var reqs []reconcile.Request

		enqueue := func(name string) {
			for _, ns := range []string{llmSvc.Namespace, constants.KServeNamespace} {
				key := types.NamespacedName{Namespace: ns, Name: name}
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				reqs = append(reqs, reconcile.Request{NamespacedName: key})
			}
		}

		for _, ref := range llmSvc.Spec.BaseRefs {
			enqueue(ref.Name)
		}

		for _, name := range llmSvc.Status.Annotations {
			enqueue(name)
		}

		for _, name := range WellKnownDefaultConfigs.UnsortedList() {
			enqueue(name)
		}

		if len(reqs) > 0 {
			logger.V(2).Info("Enqueuing LLMInferenceServiceConfig reconcile requests",
				"llmisvc", fmt.Sprintf("%s/%s", llmSvc.Namespace, llmSvc.Name),
				"requests", len(reqs))
		}

		return reqs
	})
}
