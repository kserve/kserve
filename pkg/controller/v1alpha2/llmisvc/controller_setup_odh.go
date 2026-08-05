//go:build distro

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

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	istioapi "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

func (r *LLMISVCReconciler) extendControllerSetup(mgr manager.Manager, b *builder.Builder) error {
	if err := istioapi.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Istio v1 APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), istioapi.SchemeGroupVersion.String(), "DestinationRule"); ok && err == nil {
		b.Owns(&istioapi.DestinationRule{}, builder.WithPredicates(childResourcesPredicate))
	}

	if err := routev1.Install(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Route v1 APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), routev1.GroupVersion.String(), "Route"); ok && err == nil {
		logger := mgr.GetLogger().WithName("LLMInferenceService.SetupWithManager")

		b.Watches(&routev1.Route{},
			r.enqueueOnRouteChange(logger),
			builder.WithPredicates(routeChangePredicate()),
		)
	}

	return nil
}

// routeChangePredicate fires on Route events that could affect URL discovery:
// admission state changes, host changes, path changes, or backend retargeting.
func routeChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			route, ok := e.Object.(*routev1.Route)

			return ok && admittedHost(route) != ""
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldRoute, ok1 := e.ObjectOld.(*routev1.Route)
			newRoute, ok2 := e.ObjectNew.(*routev1.Route)
			if !ok1 || !ok2 {
				return false
			}

			if admittedHost(oldRoute) != admittedHost(newRoute) {
				return true
			}

			return oldRoute.Spec.Path != newRoute.Spec.Path ||
				targetChanged(oldRoute, newRoute) ||
				tlsTerminationChanged(oldRoute, newRoute) ||
				!equality.Semantic.DeepEqual(oldRoute.Spec.Port, newRoute.Spec.Port)
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return true
		},
	}
}

func targetChanged(old, new *routev1.Route) bool {
	return old.Spec.To.Name != new.Spec.To.Name || old.Spec.To.Kind != new.Spec.To.Kind
}

func tlsTerminationChanged(old, new *routev1.Route) bool {
	if (old.Spec.TLS == nil) != (new.Spec.TLS == nil) {
		return true
	}

	return old.Spec.TLS != nil && old.Spec.TLS.Termination != new.Spec.TLS.Termination
}

// enqueueOnRouteChange maps a Route change to reconcile requests for
// LLMInferenceServices affected by this Route. See mapRouteToRequests for details.
func (r *LLMISVCReconciler) enqueueOnRouteChange(logger logr.Logger) handler.EventHandler {
	logger = logger.WithName("enqueueOnRouteChange")

	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		route, ok := object.(*routev1.Route)
		if !ok {
			return nil
		}

		return r.mapRouteToRequests(ctx, logger, route)
	})
}

// mapRouteToRequests resolves which LLMInferenceServices should be reconciled
// when an OpenShift Route changes. Uses reverse lookup (status.addresses) and
// three forward strategies (managed HTTPRoute owners, explicit refs, observed status).
func (r *LLMISVCReconciler) mapRouteToRequests(ctx context.Context, logger logr.Logger, route *routev1.Route) []reconcile.Request {
	llmSvcList := &v1alpha2.LLMInferenceServiceList{}
	if err := r.List(ctx, llmSvcList); err != nil {
		logger.Error(err, "failed to list LLMInferenceServices")

		return nil
	}

	seen := make(map[types.NamespacedName]bool)
	var reqs []reconcile.Request

	enqueue := func(key types.NamespacedName) {
		if !seen[key] {
			seen[key] = true
			reqs = append(reqs, reconcile.Request{NamespacedName: key})
		}
	}

	// Reverse: enqueue services that currently publish this Route in status.
	for i := range llmSvcList.Items {
		if hasRouteOrigin(&llmSvcList.Items[i], route.Name, route.Namespace) {
			enqueue(types.NamespacedName{Namespace: llmSvcList.Items[i].Namespace, Name: llmSvcList.Items[i].Name})
		}
	}

	// Forward: trace Route - Service - gateway, then match via three strategies:
	//   a) Managed HTTPRoutes: check parentRefs for the gateway, enqueue owning LLMISvc.
	//   b) Refs-based services: look up explicitly referenced HTTPRoutes and check
	//      their parentRefs for the gateway.
	//   c) Previously observed state: fall back to status.router.gateways for services
	//      whose gateway linkage is derived from merged configs/baseRefs.
	gwName, gwNamespace := r.resolveGatewayFromRoute(ctx, logger, route)
	if gwName == "" {
		return reqs
	}

	// (a) Only managed HTTPRoutes - external refs handled in strategy (b) below.
	httpRouteList := &gwapiv1.HTTPRouteList{}
	if err := r.List(ctx, httpRouteList, client.MatchingLabels(ChildResourcesLabelSelector.MatchLabels)); err != nil {
		logger.Error(err, "failed to list HTTPRoutes")
	}

	for i := range httpRouteList.Items {
		httpRoute := &httpRouteList.Items[i]
		if !httpRouteReferencesGateway(httpRoute, gwName, gwNamespace) {
			continue
		}

		for _, ownerRef := range httpRoute.GetOwnerReferences() {
			if ownerRef.Kind == "LLMInferenceService" {
				enqueue(types.NamespacedName{Namespace: httpRoute.Namespace, Name: ownerRef.Name})
			}
		}
	}

	// (b) Catch services using external HTTPRoute refs (no managed HTTPRoute with owner ref).
	for i := range llmSvcList.Items {
		llmSvc := &llmSvcList.Items[i]
		if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || !llmSvc.Spec.Router.Route.HTTP.HasRefs() {
			continue
		}

		for _, ref := range llmSvc.Spec.Router.Route.HTTP.Refs {
			if ref.Name == "" {
				continue
			}

			httpRoute := &gwapiv1.HTTPRoute{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: llmSvc.Namespace, Name: ref.Name}, httpRoute); err != nil {
				logger.Error(err, "failed to get referenced HTTPRoute",
					"llmSvc", llmSvc.Name, "httpRoute", ref.Name)

				continue
			}

			if httpRouteReferencesGateway(httpRoute, gwName, gwNamespace) {
				enqueue(types.NamespacedName{Namespace: llmSvc.Namespace, Name: llmSvc.Name})

				break
			}
		}
	}

	// (c) Last resort: check previously observed gateway associations from status.
	for i := range llmSvcList.Items {
		if hasRoutingGatewayRef(&llmSvcList.Items[i], gwapiv1.ObjectName(gwName), gwapiv1.Namespace(gwNamespace)) {
			enqueue(types.NamespacedName{Namespace: llmSvcList.Items[i].Namespace, Name: llmSvcList.Items[i].Name})
		}
	}

	return reqs
}

func httpRouteReferencesGateway(httpRoute *gwapiv1.HTTPRoute, gwName, gwNamespace string) bool {
	for _, ref := range httpRoute.Spec.ParentRefs {
		if ref.Group != nil && string(*ref.Group) != gwapiv1.GroupName {
			continue
		}
		if ref.Kind != nil && string(*ref.Kind) != "Gateway" {
			continue
		}

		refNS := httpRoute.Namespace
		if ref.Namespace != nil {
			refNS = string(*ref.Namespace)
		}

		if string(ref.Name) == gwName && refNS == gwNamespace {
			return true
		}
	}

	return false
}

func hasRouteOrigin(llmSvc *v1alpha2.LLMInferenceService, routeName, routeNamespace string) bool {
	for _, addr := range llmSvc.Status.Addresses {
		if addr.Origin != nil &&
			addr.Origin.Kind == "Route" &&
			string(addr.Origin.Name) == routeName &&
			addr.Origin.Namespace != nil &&
			string(*addr.Origin.Namespace) == routeNamespace {
			return true
		}
	}

	return false
}

// resolveGatewayFromRoute traces Route - target Service - gateway name.
// Returns empty strings if the chain can't be resolved.
func (r *LLMISVCReconciler) resolveGatewayFromRoute(ctx context.Context, logger logr.Logger, route *routev1.Route) (gwName, gwNamespace string) {
	if route.Spec.To.Kind != "" && route.Spec.To.Kind != "Service" {
		return "", ""
	}

	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: route.Namespace,
		Name:      route.Spec.To.Name,
	}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("service not found for route",
				"route", route.Name, "service", route.Spec.To.Name)
		} else {
			logger.Error(err, "failed to get service for route",
				"route", route.Name, "service", route.Spec.To.Name)
		}

		return "", ""
	}

	name := svc.Spec.Selector[gatewayNameLabel]
	if name == "" {
		return "", ""
	}

	return name, route.Namespace
}
