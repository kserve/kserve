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
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/utils/ptr"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// reconcileRouter handles the networking and routing components for the LLM service
// This includes schedulers, HTTP routes, and various validation checks
func (r *LLMISVCReconciler) reconcileRouter(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("reconcileRouter")
	ctx = log.IntoContext(ctx, logger)

	logger.Info("Reconciling Router")

	// Ensure readiness is determined even if errors occur
	defer llmSvc.DetermineRouterReadiness()

	// Validate that referenced resources exist before proceeding
	if err := r.validateRouterReferences(ctx, llmSvc); err != nil {
		return err
	}

	// Reconcile the scheduler component that manages inference pools
	if err := r.reconcileScheduler(ctx, llmSvc); err != nil {
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
func (r *LLMISVCReconciler) reconcileHTTPRoutes(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling HTTPRoute")

	expectedHTTPRoute := r.expectedHTTPRoute(ctx, llmSvc)

	// Clean up if router or routes are not configured
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil {
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
func (r *LLMISVCReconciler) collectReferencedRoutes(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) ([]*gwapiv1.HTTPRoute, error) {
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || !llmSvc.Spec.Router.Route.HTTP.HasRefs() {
		return nil, nil
	}

	referencedRoutes := make([]*gwapiv1.HTTPRoute, 0, len(llmSvc.Spec.Router.Route.HTTP.Refs))

	// Fetch each referenced route
	for _, routeRef := range llmSvc.Spec.Router.Route.HTTP.Refs {
		route := &gwapiv1.HTTPRoute{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: routeRef.Name}, route); err != nil {
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
func (r *LLMISVCReconciler) expectedHTTPRoute(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *gwapiv1.HTTPRoute {
	httpRoute := &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-route"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: RouterLabels(llmSvc),
		},
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Route != nil && llmSvc.Spec.Router.Route.HTTP.Spec != nil {
		httpRoute.Spec = *llmSvc.Spec.Router.Route.HTTP.Spec.DeepCopy()
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Gateway != nil {
		log.FromContext(ctx).Info("Reconciling Gateway", "gateway", llmSvc.Spec.Router.Gateway)

		// If Gateway is not managed (has .refs), re-attach the expected route to the referenced gateways
		if llmSvc.Spec.Router.Gateway.HasRefs() {
			httpRoute.Spec.CommonRouteSpec.ParentRefs = make([]gwapiv1.ParentReference, 0, len(llmSvc.Spec.Router.Gateway.Refs))
			for _, ref := range llmSvc.Spec.Router.Gateway.Refs {
				httpRoute.Spec.CommonRouteSpec.ParentRefs = append(httpRoute.Spec.CommonRouteSpec.ParentRefs, toGatewayRef(ref))
			}
		}
	}

	return httpRoute
}

func (r *LLMISVCReconciler) updateRoutingStatus(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, routes ...*gwapiv1.HTTPRoute) error {
	logger := log.FromContext(ctx)

	var urls []*apis.URL
	for _, route := range routes {
		discoverURL, err := DiscoverURLs(ctx, r.Client, route)
		if IgnoreExternalAddressNotFound(err) != nil {
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
	} else {
		llmSvc.Status.URL = externalURLs[0]
	}

	llmSvc.Status.Addresses = make([]duckv1.Addressable, 0, len(urls))
	for _, url := range urls {
		llmSvc.Status.Addresses = append(llmSvc.Status.Addresses, duckv1.Addressable{
			URL: url,
		})
	}

	return nil
}

func toGatewayRef(ref v1alpha1.UntypedObjectReference) gwapiv1.ParentReference {
	return gwapiv1.ParentReference{
		// TODO(api): With this structure we are missing the ability to narrow a section of targeted gateway by the route we are creating
		// missing SectionName and Port will implicitly bind the route to the first listener in the parent
		Name:      ref.Name,
		Namespace: &ref.Namespace,
		Group:     ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
		Kind:      ptr.To(gwapiv1.Kind("Gateway")),
	}
}

func RouterLabels(llmSvc *v1alpha1.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-router",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
}

func semanticHTTPRouteIsEqual(e *gwapiv1.HTTPRoute, c *gwapiv1.HTTPRoute) bool {
	return equality.Semantic.DeepDerivative(e.Spec, c.Spec) &&
		equality.Semantic.DeepDerivative(e.Labels, c.Labels) &&
		equality.Semantic.DeepDerivative(e.Annotations, c.Annotations)
}

// EvaluateGatewayConditions evaluates the readiness of all Gateways referenced by the LLMInferenceService
// and updates the GatewaysReady condition accordingly
func (r *LLMISVCReconciler) EvaluateGatewayConditions(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("evaluateGatewayConditions")

	// If no router or gateway configuration, skip Gateway evaluation
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Gateway == nil || !llmSvc.Spec.Router.Gateway.HasRefs() {
		logger.Info("No Gateway references found, skipping Gateway condition evaluation")
		return nil
	}

	// Check if there's already a validation failure condition set
	condition := llmSvc.GetStatus().GetCondition(v1alpha1.GatewaysReady)
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
func (r *LLMISVCReconciler) CollectReferencedGateways(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) ([]*gwapiv1.Gateway, error) {
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Gateway == nil || !llmSvc.Spec.Router.Gateway.HasRefs() {
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

		err := r.Client.Get(ctx, gatewayKey, gateway)
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
// and updates the HTTPRoutesReady condition accordingly
func (r *LLMISVCReconciler) EvaluateHTTPRouteConditions(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("evaluateHTTPRouteConditions")

	// If no router or route configuration, mark HTTPRoutes as ready (no routes to evaluate)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || llmSvc.Spec.Router.Route.HTTP == nil {
		logger.Info("No HTTPRoute configuration found, marking HTTPRoutesReady as True")
		llmSvc.MarkHTTPRoutesReady()
		return nil
	}

	// Check if there's already a validation failure condition set
	condition := llmSvc.GetStatus().GetCondition(v1alpha1.HTTPRoutesReady)
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
		if err := r.Client.Get(ctx, types.NamespacedName{
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
			topLevelCondition, _ := nonReadyHTTPRouteTopLevelCondition(route)
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
// and updates the InferencePoolReady condition accordingly
func (r *LLMISVCReconciler) EvaluateInferencePoolConditions(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("EvaluateInferencePoolConditions")

	// If no router or scheduler configuration, mark Inference Pools as ready (no Inference Pools to evaluate)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		logger.V(2).Info("Scheduler is disabled, marking InferencePoolReady as True")
		llmSvc.MarkInferencePoolReady()
		return nil
	}

	curr := &igwapi.InferencePool{}

	if llmSvc.Spec.Router.Scheduler.Pool != nil && llmSvc.Spec.Router.Scheduler.Pool.Ref != nil && llmSvc.Spec.Router.Scheduler.Pool.Ref.Name != "" {
		poolRef := llmSvc.Spec.Router.Scheduler.Pool.Ref
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: llmSvc.Namespace, Name: poolRef.Name}, curr)
		if err != nil {
			err := fmt.Errorf("failed to fetch referenced Inference Pool %s/%s: %w", llmSvc.Namespace, poolRef.Name, err)
			llmSvc.MarkInferencePoolNotReady("InferencePoolFetchError", err.Error())
			return err
		}
	} else {
		expected := r.expectedSchedulerInferencePool(ctx, llmSvc)
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: expected.Namespace, Name: expected.Name}, curr)
		if err != nil {
			err := fmt.Errorf("failed to fetch embedded Inference Pool %s/%s: %w", llmSvc.Namespace, llmSvc.Name, err)
			llmSvc.MarkInferencePoolNotReady("InferencePoolFetchError", err.Error())
			return err
		}
	}

	if !IsInferencePoolReady(curr) {
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
	logger.V(2).Info("Inference Pool is ready", "pool", curr)
	return nil
}
