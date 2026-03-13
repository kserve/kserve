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
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/utils/ptr"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// AnnotationInferencePoolMigrated records when the HTTPRoute has migrated to v1 InferencePool.
// Once set to "v1", traffic will never fall back to v1alpha2 even during transient failures.
const AnnotationInferencePoolMigrated = "serving.kserve.io/inference-pool-migrated"

// ErrPreconditionNotMet is a sentinel error returned by ensureGatewayPreconditions
// when a non-transient precondition is not met (e.g. a required CRD is missing).
// The caller should mark status but not propagate the error to avoid infinite requeue.
var ErrPreconditionNotMet = errors.New("precondition not met")

// reconcileRouter handles the networking and routing components for the LLM service
// This includes schedulers, HTTP routes, and various validation checks
func (r *LLMISVCReconciler) reconcileRouter(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	logger := log.FromContext(ctx).WithName("reconcileRouter")
	ctx = log.IntoContext(ctx, logger)

	logger.Info("Reconciling Router")

	// Ensure readiness is determined even if errors occur
	defer llmSvc.DetermineRouterReadiness()

	// Ensure platform-specific preconditions are met before proceeding.
	// A non-transient precondition failure (e.g. missing CRD) marks status and stops
	// reconciliation without requeuing — the condition won't resolve by retrying.
	if err := r.ensureGatewayPreconditions(ctx, llmSvc); err != nil {
		if errors.Is(err, ErrPreconditionNotMet) {
			llmSvc.MarkHTTPRoutesNotReady("GatewayPreconditionNotMet", err.Error())
			return nil
		}
		llmSvc.MarkHTTPRoutesNotReady("HTTPRouteReconcileError", err.Error())
		return fmt.Errorf("failed to ensure gateway preconditions: %w", err)
	}

	// Validate that referenced resources exist before proceeding
	if err := r.validateRouterReferences(ctx, llmSvc); err != nil {
		return err
	}

	// Reconcile the scheduler component that manages inference pools
	if err := r.reconcileScheduler(ctx, llmSvc, config.SchedulerConfig); err != nil {
		llmSvc.MarkSchedulerWorkloadNotReady("SchedulerReconcileError", "Failed to reconcile scheduler: %v", err.Error())
		return fmt.Errorf("failed to reconcile scheduler: %w", err)
	}

	// Reconcile HTTP routes for traffic routing
	// We do not support Gateway's spec, when creating HTTPRoutes either the default gateway or those provided
	// as refs are attached to reconciled routes
	if err := r.reconcileHTTPRoutes(ctx, llmSvc); err != nil {
		llmSvc.MarkHTTPRoutesNotReady("HTTPRouteReconcileError", "Failed to reconcile HTTPRoute: %v", err.Error())
		return fmt.Errorf("failed to reconcile HTTP routes: %w", err)
	}

	// Reconcile platform-specific networking resources
	if err := r.reconcileRouterPlatformNetworking(ctx, llmSvc); err != nil {
		llmSvc.MarkHTTPRoutesNotReady("PlatformNetworkingReconcileError", "Failed to reconcile platform networking: %v", err.Error())
		return fmt.Errorf("failed to reconcile router platform networking: %w", err)
	}

	// Evaluate the subconditions to determine overall router health
	if err := r.EvaluateInferencePoolConditions(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to evaluate Inference Pool conditions: %w", err)
	}

	if err := r.EvaluateGatewayConditions(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to evaluate gateway conditions: %w", err)
	}

	if err := r.EvaluateHTTPRouteConditions(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to evaluate HTTPRoute conditions: %w", err)
	}

	return nil
}

// reconcileHTTPRoutes manages HTTPRoute resources for traffic routing
// It handles both custom routes (via refs) and generated routes (via spec)
func (r *LLMISVCReconciler) reconcileHTTPRoutes(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling HTTPRoute")

	expectedHTTPRoute := r.expectedHTTPRoute(ctx, llmSvc)

	// Clean up if router or routes are not configured
	if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil {
		_ = r.updateRoutingStatus(ctx, llmSvc)
		return Delete(ctx, r, llmSvc, expectedHTTPRoute)
	}

	// Collect any explicitly referenced HTTPRoutes
	referencedRoutes, err := r.collectReferencedRoutes(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to collect referenced routes: %w", err)
	}

	route := llmSvc.Spec.Router.Route

	// If using custom routes via refs, delete our own
	if route.HTTP.HasRefs() {
		if err := Delete(ctx, r, llmSvc, expectedHTTPRoute); err != nil {
			return err
		}
	}

	if route.HTTP.HasSpec() {
		if err := Reconcile(ctx, r, llmSvc, &gwapiv1.HTTPRoute{}, expectedHTTPRoute, semanticHTTPRouteIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile HTTPRoute %s/%s: %w", expectedHTTPRoute.GetNamespace(), expectedHTTPRoute.GetName(), err)
		}
		referencedRoutes = append(referencedRoutes, expectedHTTPRoute)
	}

	return r.updateRoutingStatus(ctx, llmSvc, referencedRoutes...)
}

// collectReferencedRoutes gathers all HTTPRoutes referenced by the service
// This is used for status updates and condition evaluation
func (r *LLMISVCReconciler) collectReferencedRoutes(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ([]*gwapiv1.HTTPRoute, error) {
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || !llmSvc.Spec.Router.Route.HTTP.HasRefs() {
		return nil, nil
	}

	referencedRoutes := make([]*gwapiv1.HTTPRoute, 0, len(llmSvc.Spec.Router.Route.HTTP.Refs))

	// Fetch each referenced route
	for _, routeRef := range llmSvc.Spec.Router.Route.HTTP.Refs {
		route := &gwapiv1.HTTPRoute{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: routeRef.Name}, route); err != nil {
			if apierrors.IsNotFound(err) {
				// Skip missing routes - validation is handled separately
				continue
			}
			return referencedRoutes, fmt.Errorf("failed to get HTTPRoute %s/%s: %w", llmSvc.GetName(), routeRef.Name, err)
		}

		referencedRoutes = append(referencedRoutes, route)
	}

	return referencedRoutes, nil
}

// expectedHTTPRoute creates the HTTPRoute specification for this service
// This route is created when the service specifies inline routing configuration
func (r *LLMISVCReconciler) expectedHTTPRoute(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *gwapiv1.HTTPRoute {
	httpRoute := &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-route"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: RouterLabels(llmSvc),
		},
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Route != nil && llmSvc.Spec.Router.Route.HTTP.Spec != nil {
		httpRoute.Spec = *llmSvc.Spec.Router.Route.HTTP.Spec.DeepCopy()
	}

	// Migration logic: check if we should switch from v1alpha2 to v1 InferencePool
	// Only applies to managed routes with a scheduler (not using external pool refs)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil ||
		llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		return httpRoute
	}

	logger := log.FromContext(ctx).WithValues("migration", "InferencePool")

	curr := &gwapiv1.HTTPRoute{}
	routeExists := r.Get(ctx, client.ObjectKeyFromObject(httpRoute), curr) == nil

	const v1MigrationValue = "v1"
	var isMigrated bool
	var infPoolV1Alpha2Support, infPoolV1Support metav1.ConditionStatus

	if routeExists {
		migrationValue, hasMigrationAnnotation := curr.Annotations[AnnotationInferencePoolMigrated]
		isMigrated = hasMigrationAnnotation && migrationValue == v1MigrationValue
		infPoolV1Alpha2Support = IsInferencePoolV1Alpha2Supported(curr)
		infPoolV1Support = IsInferencePoolV1Supported(curr)
	}

	// Switch to v1 if:
	// - Gateway accepted v1 (route using v1 and ResolvedRefs=True), OR
	// - Gateway rejected v1alpha2 (route using v1alpha2 and ResolvedRefs=False/InvalidKind), OR
	// - Already migrated (annotation exists - one-way lock)
	if isMigrated || infPoolV1Support == metav1.ConditionTrue || infPoolV1Alpha2Support == metav1.ConditionFalse {
		// Switch backendRef to v1 API group
		for i := range httpRoute.Spec.Rules {
			for j := range httpRoute.Spec.Rules[i].BackendRefs {
				if isDefaultBackendRef(llmSvc, httpRoute.Spec.Rules[i].BackendRefs[j].BackendRef) {
					httpRoute.Spec.Rules[i].BackendRefs[j].Group = ptr.To(gwapiv1.Group(constants.InferencePoolV1APIGroupName))
				}
			}
		}
		// Persist migration annotation
		if httpRoute.Annotations == nil {
			httpRoute.Annotations = make(map[string]string, 1)
		}
		httpRoute.Annotations[AnnotationInferencePoolMigrated] = v1MigrationValue

		logger.Info("Using InferencePool v1 API for HTTPRoute",
			"isMigrated", isMigrated,
			"infPoolV1Support", infPoolV1Support,
			"infPoolV1Alpha2Support", infPoolV1Alpha2Support,
			"httproute.curr.spec", curr.Spec,
			"httproute.curr.status", curr.Status,
			"httproute.expected.spec", httpRoute.Spec,
		)
	} else {
		// Not migrated yet, use v1alpha2
		for i := range httpRoute.Spec.Rules {
			for j := range httpRoute.Spec.Rules[i].BackendRefs {
				if isDefaultBackendRef(llmSvc, httpRoute.Spec.Rules[i].BackendRefs[j].BackendRef) {
					httpRoute.Spec.Rules[i].BackendRefs[j].Group = ptr.To(gwapiv1.Group(constants.InferencePoolV1Alpha2APIGroupName))
				}
			}
		}

		logger.Info("Using InferencePool v1alpha2 API for HTTPRoute",
			"isMigrated", isMigrated,
			"infPoolV1Support", infPoolV1Support,
			"infPoolV1Alpha2Support", infPoolV1Alpha2Support,
			"httproute.curr.spec", curr.Spec,
			"httproute.curr.status", curr.Status,
			"httproute.expected.spec", httpRoute.Spec,
		)
	}

	return httpRoute
}

func (r *LLMISVCReconciler) updateRoutingStatus(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, routes ...*gwapiv1.HTTPRoute) error {
	logger := log.FromContext(ctx)

	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.Status.Addresses = nil
		llmSvc.Status.Address = nil
		llmSvc.MarkHTTPRoutesNotReady("Stopped", "Service is stopped")
		return nil
	}

	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil {
		llmSvc.Status.Addresses = []duckv1.Addressable{{
			URL: apis.HTTPS(network.GetServiceHostname(
				kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"),
				llmSvc.GetNamespace(),
			)),
		}}
		return nil
	}

	// TODO: LoadConfig fetches the configmap from the API server on every
	// reconciliation. Consider caching with an informer-based watch to reduce
	// API server pressure under high reconciliation load.
	cfg, err := LoadConfig(ctx, r.Clientset)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var urls []*apis.URL
	for _, route := range routes {
		discoverURL, err := DiscoverURLs(ctx, r.Client, route, cfg.UrlScheme)
		if IgnoreNoURLsDiscovered(err) != nil {
			return fmt.Errorf("failed to discover URL for route %s/%s: %w", route.GetNamespace(), route.GetName(), err)
		}
		if discoverURL != nil {
			urls = append(urls, discoverURL...)
		}
	}

	slices.SortStableFunc(urls, func(a, b *apis.URL) int {
		return cmp.Compare(a.String(), b.String())
	})

	externalURLs := FilterExternalURLs(urls)
	if len(externalURLs) == 0 {
		logger.Info("no public URL discovered")
		llmSvc.Status.URL = nil
	} else {
		llmSvc.Status.URL = externalURLs[0]
	}

	llmSvc.Status.Addresses = make([]duckv1.Addressable, 0, len(urls))
	for _, url := range urls {
		addressType := AddressTypeName(url)
		llmSvc.Status.Addresses = append(llmSvc.Status.Addresses, duckv1.Addressable{
			Name: &addressType,
			URL:  url,
		})
	}

	return nil
}

func RouterLabels(llmSvc *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentRouter,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
}

func semanticHTTPRouteIsEqual(e *gwapiv1.HTTPRoute, c *gwapiv1.HTTPRoute) bool {
	return equality.Semantic.DeepDerivative(e.Spec, c.Spec) &&
		equality.Semantic.DeepDerivative(e.Labels, c.Labels) &&
		equality.Semantic.DeepDerivative(e.Annotations, c.Annotations)
}

// EvaluateGatewayConditions evaluates the readiness of all Gateways referenced by the LLMInferenceService
// and updates the GatewaysReady condition accordingly
func (r *LLMISVCReconciler) EvaluateGatewayConditions(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("evaluateGatewayConditions")

	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.MarkGatewaysNotReady("Stopped", "Service is stopped")
		return nil
	}

	// If no router or gateway configuration, mark as ready to clear any previous stopped state
	if llmSvc.Spec.Router == nil || !llmSvc.Spec.Router.Gateway.HasRefs() {
		logger.Info("No Gateway references found, skipping Gateway condition evaluation")
		llmSvc.MarkGatewaysReadyUnset()
		return nil
	}

	// Check if there's already a validation failure condition set
	condition := llmSvc.GetStatus().GetCondition(v1alpha2.GatewaysReady)
	if condition != nil && condition.IsFalse() && condition.Reason == RefsInvalidReason {
		logger.Info("Gateway validation failed, skipping readiness evaluation", "reason", condition.Reason, "message", condition.Message)
		return nil
	}

	gateways, err := r.CollectReferencedGateways(ctx, llmSvc)
	if err != nil {
		llmSvc.MarkGatewaysNotReady("GatewayFetchError", "Failed to fetch referenced Gateways: %v", err.Error())
		return fmt.Errorf("failed to fetch referenced gateways: %w", err)
	}

	notReadyGateways := EvaluateGatewayReadiness(ctx, gateways)

	if len(notReadyGateways) > 0 {
		gatewayNames := make([]string, len(notReadyGateways))
		for i, gw := range notReadyGateways {
			gatewayNames[i] = fmt.Sprintf("%s/%s", gw.Namespace, gw.Name)
		}
		llmSvc.MarkGatewaysNotReady("GatewaysNotReady", "The following Gateways are not ready: %v", gatewayNames)
		logger.V(2).Info("Some referenced Gateways are not ready", "gateways", notReadyGateways)
		return nil
	}
	llmSvc.MarkGatewaysReady()
	logger.Info("All referenced Gateways are ready")
	return nil
}

// CollectReferencedGateways retrieves all Gateway objects referenced in the LLMInferenceService spec
func (r *LLMISVCReconciler) CollectReferencedGateways(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ([]*gwapiv1.Gateway, error) {
	if llmSvc.Spec.Router == nil || !llmSvc.Spec.Router.Gateway.HasRefs() {
		return nil, nil
	}

	// Use a map to ensure gateways are not repeated (keyed by namespace/name)
	gatewayMap := make(map[string]*gwapiv1.Gateway)
	routes, err := r.collectReferencedRoutes(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to collect referenced routes: %w", err)
	}
	for _, route := range routes {
		discoveredGateways, err := DiscoverGateways(ctx, r.Client, route)
		if err != nil {
			return nil, fmt.Errorf("failed to discover gateways: %w", err)
		}
		for _, gateway := range discoveredGateways {
			key := gateway.gateway.Namespace + "/" + gateway.gateway.Name
			gatewayMap[key] = gateway.gateway
		}
	}

	for _, ref := range llmSvc.Spec.Router.Gateway.Refs {
		gateway := &gwapiv1.Gateway{}
		gatewayKey := types.NamespacedName{
			Name:      string(ref.Name),
			Namespace: string(ref.Namespace),
		}

		// If namespace is not specified, use the same namespace as the LLMInferenceService
		if gatewayKey.Namespace == "" {
			gatewayKey.Namespace = llmSvc.GetNamespace()
		}

		err := r.Get(ctx, gatewayKey, gateway)
		if err != nil {
			return nil, fmt.Errorf("failed to get Gateway %s: %w", gatewayKey, err)
		}

		key := gateway.Namespace + "/" + gateway.Name
		gatewayMap[key] = gateway
	}

	// Convert map values to slice
	gateways := make([]*gwapiv1.Gateway, 0, len(gatewayMap))
	for _, gw := range gatewayMap {
		gateways = append(gateways, gw)
	}

	return gateways, nil
}

// EvaluateHTTPRouteConditions evaluates the readiness of all HTTPRoutes referenced by the LLMInferenceService
// and updates the HTTPRoutesReady condition accordingly. Also detects Gateway rejection of v1alpha2 backendRefs.
func (r *LLMISVCReconciler) EvaluateHTTPRouteConditions(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("evaluateHTTPRouteConditions")

	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.MarkHTTPRoutesNotReady("Stopped", "Service is stopped")
		return nil
	}

	// If no router or route configuration, mark HTTPRoutes as ready (no routes to evaluate)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || llmSvc.Spec.Router.Route.HTTP == nil {
		logger.Info("No HTTPRoute configuration found, clearing HTTPRoutesReady condition")
		llmSvc.MarkHTTPRoutesReadyUnset()
		return nil
	}

	// Check if there's already a validation failure condition set
	condition := llmSvc.GetStatus().GetCondition(v1alpha2.HTTPRoutesReady)
	if condition != nil && condition.IsFalse() && condition.Reason == RefsInvalidReason {
		logger.Info("HTTPRoute validation failed, skipping readiness evaluation", "reason", condition.Reason, "message", condition.Message)
		return nil
	}

	// Collect all HTTPRoutes (both referenced and managed)
	var allRoutes []*gwapiv1.HTTPRoute

	// Get referenced routes
	referencedRoutes, err := r.collectReferencedRoutes(ctx, llmSvc)
	if err != nil {
		llmSvc.MarkHTTPRoutesNotReady("HTTPRouteFetchError", "Failed to fetch referenced HTTPRoutes: %v", err.Error())
		return fmt.Errorf("failed to fetch referenced HTTPRoutes: %w", err)
	}
	allRoutes = append(allRoutes, referencedRoutes...)

	// Get managed route if it exists
	if llmSvc.Spec.Router.Route.HTTP.HasSpec() {
		expectedHTTPRoute := r.expectedHTTPRoute(ctx, llmSvc)
		// Try to get the actual managed route from the cluster
		managedRoute := &gwapiv1.HTTPRoute{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: expectedHTTPRoute.Namespace,
			Name:      expectedHTTPRoute.Name,
		}, managedRoute); err == nil {
			allRoutes = append(allRoutes, managedRoute)
		}
	}

	// If no routes found, mark as ready (nothing to evaluate)
	if len(allRoutes) == 0 {
		llmSvc.MarkHTTPRoutesReady()
		logger.Info("No HTTPRoutes found, marking HTTPRoutesReady as true")
		return nil
	}

	notReadyRoutes := EvaluateHTTPRouteReadiness(ctx, allRoutes)

	if len(notReadyRoutes) > 0 {
		nonReadyRouteMessages := make([]string, len(notReadyRoutes))
		for i, route := range notReadyRoutes {
			topLevelCondition := findNonReadyGatewayCondition(route)
			if topLevelCondition != nil {
				nonReadyRouteMessages[i] = fmt.Sprintf("%s/%s: %#v (reason %q, message %q)", route.Namespace, route.Name, topLevelCondition.Status, topLevelCondition.Reason, topLevelCondition.Message)
			} else {
				nonReadyRouteMessages[i] = fmt.Sprintf("%s/%s: %#v", route.Namespace, route.Name, route.Status)
			}
		}
		llmSvc.MarkHTTPRoutesNotReady("HTTPRoutesNotReady", "The following HTTPRoutes are not ready: %v", nonReadyRouteMessages)
		logger.V(2).Info("Some HTTPRoutes are not ready", "routes", notReadyRoutes)
		return nil
	}

	llmSvc.MarkHTTPRoutesReady()
	logger.V(2).Info("All HTTPRoutes are ready", "routes", allRoutes)
	return nil
}

// EvaluateInferencePoolConditions evaluates the readiness of all Inference Pools in the LLMInferenceService
// and updates the InferencePoolReady condition accordingly.
// During the v1alpha2 to v1 migration, it checks both pool versions for managed pools
// and considers the pool ready if at least one is ready.
func (r *LLMISVCReconciler) EvaluateInferencePoolConditions(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("EvaluateInferencePoolConditions")

	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.MarkInferencePoolNotReady("Stopped", "Service is stopped")
		return nil
	}

	// If no router or scheduler configuration, mark Inference Pools as ready (no Inference Pools to evaluate)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		logger.V(2).Info("Scheduler is disabled, clearing InferencePoolReady condition")
		llmSvc.MarkInferencePoolReadyUnset()
		return nil
	}

	// For referenced pools (external), only check that pool
	if llmSvc.Spec.Router.Scheduler.Pool != nil && llmSvc.Spec.Router.Scheduler.Pool.Ref != nil && llmSvc.Spec.Router.Scheduler.Pool.Ref.Name != "" {
		poolRef := llmSvc.Spec.Router.Scheduler.Pool.Ref
		curr := &igwapi.InferencePool{}
		err := r.Get(ctx, types.NamespacedName{Namespace: llmSvc.Namespace, Name: poolRef.Name}, curr)
		if err != nil {
			err := fmt.Errorf("failed to fetch referenced Inference Pool %s/%s: %w", llmSvc.Namespace, poolRef.Name, err)
			llmSvc.MarkInferencePoolNotReady("InferencePoolFetchError", err.Error())
			return err
		}
		return r.evaluateSingleInferencePoolCondition(ctx, llmSvc, curr)
	}

	// For managed pools, check both v1 and v1alpha2 pools during migration.
	// Both pool versions are created by the scheduler reconciler - they live in different
	// API groups and can coexist. During migration, the gateway may only accept one version.
	expected := r.expectedSchedulerInferencePool(ctx, llmSvc)
	poolName := expected.Name
	poolNamespace := expected.Namespace

	// Check v1 pool
	v1Pool := &igwapi.InferencePool{}
	v1Err := r.Get(ctx, types.NamespacedName{Namespace: poolNamespace, Name: poolName}, v1Pool)
	v1Ready := v1Err == nil && IsInferencePoolReady(v1Pool)

	// Check v1alpha2 pool - treat CRD-not-installed as "version unavailable" (not an error)
	v1alpha2Pool := &igwapiv1alpha2.InferencePool{}
	v1alpha2Err := r.Get(ctx, types.NamespacedName{Namespace: poolNamespace, Name: poolName}, v1alpha2Pool)
	v1alpha2Ready := v1alpha2Err == nil && IsInferencePoolV1Alpha2Ready(v1alpha2Pool)

	logger.V(2).Info("Checking InferencePool readiness",
		"pool", poolNamespace+"/"+poolName,
		"v1Ready", v1Ready,
		"v1Err", v1Err,
		"v1alpha2Ready", v1alpha2Ready,
		"v1alpha2Err", v1alpha2Err,
	)

	// If at least one is ready, mark as ready
	if v1Ready || v1alpha2Ready {
		llmSvc.MarkInferencePoolReady()
		logger.V(2).Info("Inference Pool is ready", "v1Ready", v1Ready, "v1alpha2Ready", v1alpha2Ready)
		return nil
	}

	// Neither is ready - report status from the one that exists (prefer v1 since it's the target)
	if v1Err == nil {
		return r.evaluateSingleInferencePoolCondition(ctx, llmSvc, v1Pool)
	}

	if v1alpha2Err == nil {
		if len(v1alpha2Pool.Status.Parents) == 0 {
			llmSvc.MarkInferencePoolNotReady("WaitingForGateway",
				"Inference Pool %s/%s exists but no Gateway controller has accepted it yet", poolNamespace, poolName)
			return nil
		}
		topLevelCondition, _ := nonReadyInferencePoolV1Alpha2TopLevelCondition(v1alpha2Pool)
		if topLevelCondition != nil {
			llmSvc.MarkInferencePoolNotReady("InferencePoolNotReady", fmt.Sprintf(
				"%s/%s: %v=%#v (reason %q, message %q)",
				poolNamespace,
				poolName,
				topLevelCondition.Type,
				topLevelCondition.Status,
				topLevelCondition.Reason,
				topLevelCondition.Message,
			))
		} else {
			llmSvc.MarkInferencePoolNotReady("InferencePoolNotReady", fmt.Sprintf("The inference pool %s/%s is not ready", poolNamespace, poolName))
		}
		return nil
	}

	// Both failed to fetch - distinguish between "not found" and transient errors
	if (apierrors.IsNotFound(v1Err) || meta.IsNoMatchError(v1Err)) &&
		(apierrors.IsNotFound(v1alpha2Err) || meta.IsNoMatchError(v1alpha2Err)) {
		llmSvc.MarkInferencePoolNotReady("InferencePoolFetchError",
			fmt.Sprintf("Inference Pool %s/%s not found (checked v1 and v1alpha2)", poolNamespace, poolName))
		return nil
	}

	// At least one was a transient error - return it so the controller requeues
	err := fmt.Errorf("failed to fetch Inference Pool %s/%s: v1: %w, v1alpha2: %w", poolNamespace, poolName, v1Err, v1alpha2Err)
	llmSvc.MarkInferencePoolNotReady("InferencePoolFetchError", err.Error())
	return err
}

// evaluateSingleInferencePoolCondition evaluates a single v1 InferencePool and updates the condition.
func (r *LLMISVCReconciler) evaluateSingleInferencePoolCondition(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, curr *igwapi.InferencePool) error {
	if !IsInferencePoolReady(curr) {
		if len(curr.Status.Parents) == 0 {
			// Pool exists but no Gateway controller has claimed it yet.
			// The Owns() watch will trigger re-reconciliation when the Gateway updates the pool's status.
			llmSvc.MarkInferencePoolNotReady("WaitingForGateway",
				"Inference Pool %s/%s exists but no Gateway controller has accepted it yet", curr.Namespace, curr.Name)
			return nil
		}
		topLevelCondition, _ := nonReadyInferencePoolTopLevelCondition(curr)
		if topLevelCondition != nil {
			llmSvc.MarkInferencePoolNotReady("InferencePoolNotReady", fmt.Sprintf(
				"%s/%s: %v=%#v (reason %q, message %q)",
				curr.Namespace,
				curr.Name,
				topLevelCondition.Type,
				topLevelCondition.Status,
				topLevelCondition.Reason,
				topLevelCondition.Message,
			))
		} else {
			llmSvc.MarkInferencePoolNotReady("InferencePoolNotReady", fmt.Sprintf("The inference pool %s/%s is not ready", curr.Namespace, curr.Name))
		}
		return nil
	}

	llmSvc.MarkInferencePoolReady()
	log.FromContext(ctx).V(2).Info("Inference Pool is ready", "pool", curr)
	return nil
}
