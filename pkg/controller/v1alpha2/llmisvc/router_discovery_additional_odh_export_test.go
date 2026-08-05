//go:build distro

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

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const GatewayNameLabelForTest = gatewayNameLabel

func HasRouteOriginForTest(llmSvc *v1alpha2.LLMInferenceService, routeName, routeNamespace string) bool {
	return hasRouteOrigin(llmSvc, routeName, routeNamespace)
}

func HTTPRouteReferencesGatewayForTest(httpRoute *gwapiv1.HTTPRoute, gwName, gwNamespace string) bool {
	return httpRouteReferencesGateway(httpRoute, gwName, gwNamespace)
}

func DiscoverRouteURLsForTest(r *LLMISVCReconciler, ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, gw gatewayWithPaths) ([]DiscoveredURL, error) {
	return r.discoverRouteURLs(ctx, llmSvc, gw)
}

func DiscoverAdditionalURLsForTest(r *LLMISVCReconciler, ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, discovered []DiscoveredURL) ([]DiscoveredURL, error) {
	return r.discoverAdditionalURLs(ctx, llmSvc, discovered)
}

func CheckRoutePortMismatchForTest(route *routev1.Route, gwServices map[string]*corev1.Service) string {
	return checkRoutePortMismatch(route, gwServices)
}

func CheckRouteTLSMismatchForTest(route *routev1.Route, gwServices map[string]*corev1.Service) string {
	return checkRouteTLSMismatch(route, gwServices)
}

func RouteChangePredicateForTest() predicate.Predicate {
	return routeChangePredicate()
}

func MapRouteToRequestsForTest(r *LLMISVCReconciler, ctx context.Context, route *routev1.Route) []reconcile.Request {
	return r.mapRouteToRequests(ctx, ctrl.Log.WithName("test"), route)
}

func NewGatewayWithPaths(name, ns string, paths ...string) gatewayWithPaths {
	return gatewayWithPaths{
		ref: gwapiv1.ObjectReference{
			Group:     gwapiv1.GroupName,
			Kind:      "Gateway",
			Name:      gwapiv1.ObjectName(name),
			Namespace: ptr.To(gwapiv1.Namespace(ns)),
		},
		paths: paths,
	}
}
