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
	"net"
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

type resolvedGateway struct {
	gateway      *gatewayapi.Gateway
	gatewayClass *gatewayapi.GatewayClass
	parentRef    gatewayapi.ParentReference
}

func DiscoverGateways(ctx context.Context, c client.Client, route *gatewayapi.HTTPRoute) ([]resolvedGateway, error) {
	gateways := make([]resolvedGateway, 0)
	for _, parentRef := range route.Spec.ParentRefs {
		ns := ptr.Deref((&parentRef).Namespace, gatewayapi.Namespace(route.Namespace))
		gwNS, gwName := string(ns), string((&parentRef).Name)

		gateway := &gatewayapi.Gateway{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: gwNS, Name: gwName}, gateway); err != nil {
			return nil, fmt.Errorf("failed to get Gateway %s/%s for route %s/%s: %w", gwNS, gwName, route.Namespace, route.Name, err)
		}

		gatewayClass := &gatewayapi.GatewayClass{}
		if err := c.Get(ctx, types.NamespacedName{Name: string(gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
			return nil, fmt.Errorf("failed to get GatewayClass %q for gateway %s/%s: %w", string(gateway.Spec.GatewayClassName), gwNS, gwName, err)
		}
		gateways = append(gateways, resolvedGateway{
			gateway:      gateway,
			gatewayClass: gatewayClass,
			parentRef:    parentRef,
		})
	}
	return gateways, nil
}

func DiscoverURLs(ctx context.Context, c client.Client, route *gatewayapi.HTTPRoute) ([]*apis.URL, error) {
	var urls []*apis.URL

	gateways, err := DiscoverGateways(ctx, c, route)
	if err != nil {
		return nil, fmt.Errorf("failed to discover gateways: %w", err)
	}

	for _, g := range gateways {
		listener := selectListener(g.gateway, g.parentRef.SectionName)
		scheme := extractSchemeFromListener(listener)
		port := listener.Port

		addresses := g.gateway.Status.Addresses
		if len(addresses) == 0 {
			return nil, &ExternalAddressNotFoundError{
				GatewayNamespace: g.gateway.Namespace,
				GatewayName:      g.gateway.Name,
			}
		}

		hostnames := extractRouteHostnames(route)
		if len(hostnames) == 0 {
			hostnames = extractAddressValues(addresses)
		}

		path := extractRoutePath(route)

		gatewayURLs, err := combineIntoURLs(hostnames, scheme, port, path)
		if err != nil {
			return nil, fmt.Errorf("failed to combine URLs for Gateway %s/%s: %w", g.gateway.Namespace, g.gateway.Name, err)
		}

		urls = append(urls, gatewayURLs...)
	}

	return urls, nil
}

func extractRoutePath(route *gatewayapi.HTTPRoute) string {
	if len(route.Spec.Rules) > 0 && len(route.Spec.Rules[0].Matches) > 0 {
		// TODO how do we deal with regexp
		return ptr.Deref(route.Spec.Rules[0].Matches[0].Path.Value, "/")
	}
	return "/"
}

func selectListener(gateway *gatewayapi.Gateway, sectionName *gatewayapi.SectionName) *gatewayapi.Listener {
	if sectionName != nil {
		for _, listener := range gateway.Spec.Listeners {
			if listener.Name == *sectionName {
				return &listener
			}
		}
	}

	return &gateway.Spec.Listeners[0]
}

func extractSchemeFromListener(listener *gatewayapi.Listener) string {
	if listener.Protocol == gatewayapi.HTTPSProtocolType {
		return "https"
	}
	return "http"
}

func extractRouteHostnames(route *gatewayapi.HTTPRoute) []string {
	var hostnames []string
	for _, h := range route.Spec.Hostnames {
		host := string(h)
		if host != "" && host != "*" {
			hostnames = append(hostnames, host)
		}
	}
	return hostnames
}

func extractAddressValues(addresses []gatewayapi.GatewayStatusAddress) []string {
	var values []string
	for _, addr := range addresses {
		if addr.Value != "" {
			values = append(values, addr.Value)
		}
	}
	return values
}

func combineIntoURLs(hostnames []string, scheme string, port gatewayapi.PortNumber, path string) ([]*apis.URL, error) {
	urls := make([]*apis.URL, 0, len(hostnames))

	sortedHostnames := make([]string, len(hostnames))
	copy(sortedHostnames, hostnames)
	slices.Sort(sortedHostnames)

	for _, hostname := range sortedHostnames {
		var urlStr string
		if (scheme == "http" && port != 80) || (scheme == "https" && port != 443) {
			urlStr = fmt.Sprintf("%s://%s%s", scheme, joinHostPort(hostname, &port), path)
		} else {
			urlStr = fmt.Sprintf("%s://%s%s", scheme, hostname, path)
		}

		url, err := apis.ParseURL(urlStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URL %s: %w", urlStr, err)
		}

		urls = append(urls, url)
	}

	return urls, nil
}

func joinHostPort(host string, port *gatewayapi.PortNumber) string {
	if port != nil && *port != 0 {
		return net.JoinHostPort(host, fmt.Sprint(*port))
	}
	return host
}

type ExternalAddressNotFoundError struct {
	GatewayNamespace string
	GatewayName      string
}

func (e *ExternalAddressNotFoundError) Error() string {
	return fmt.Sprintf("Gateway %s/%s has no external address found", e.GatewayNamespace, e.GatewayName)
}

func IgnoreExternalAddressNotFound(err error) error {
	if IsExternalAddressNotFound(err) {
		return nil
	}
	return err
}

func IsExternalAddressNotFound(err error) bool {
	var externalAddrNotFoundErr *ExternalAddressNotFoundError
	return errors.As(err, &externalAddrNotFoundErr)
}

func filter[T any](s []T, predicateFn func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, x := range s {
		if predicateFn(x) {
			out = append(out, x)
		}
	}
	return out
}

// EvaluateGatewayReadiness checks the readiness status of Gateways and returns those that are not ready
func EvaluateGatewayReadiness(ctx context.Context, gateways []*gatewayapi.Gateway) []*gatewayapi.Gateway {
	logger := log.FromContext(ctx)
	notReadyGateways := make([]*gatewayapi.Gateway, 0)

	for _, gateway := range gateways {
		ready := IsGatewayReady(gateway)
		logger.Info("Gateway readiness evaluated", "gateway", fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name), "ready", ready)

		if !ready {
			notReadyGateways = append(notReadyGateways, gateway)
		}
	}

	return notReadyGateways
}

// IsGatewayReady determines if a Gateway is ready based on its status conditions
func IsGatewayReady(gateway *gatewayapi.Gateway) bool {
	// Check for the standard Gateway API "Programmed" condition
	for _, condition := range gateway.Status.Conditions {
		if condition.Type == string(gatewayapi.GatewayConditionProgrammed) {
			return condition.Status == metav1.ConditionTrue
		}
	}

	// If no Programmed condition is found, Gateway is considered not ready
	return false
}

// EvaluateHTTPRouteReadiness checks the readiness status of HTTPRoutes and returns those that are not ready
func EvaluateHTTPRouteReadiness(ctx context.Context, routes []*gatewayapi.HTTPRoute) []*gatewayapi.HTTPRoute {
	logger := log.FromContext(ctx)
	notReadyRoutes := make([]*gatewayapi.HTTPRoute, 0)

	for _, route := range routes {
		ready := IsHTTPRouteReady(route)
		logger.Info("HTTPRoute readiness evaluated", "route", fmt.Sprintf("%s/%s", route.Namespace, route.Name), "ready", ready)

		if !ready {
			notReadyRoutes = append(notReadyRoutes, route)
		}
	}

	return notReadyRoutes
}

// IsHTTPRouteReady determines if an HTTPRoute is ready based on its status conditions.
func IsHTTPRouteReady(route *gatewayapi.HTTPRoute) bool {
	if route == nil || len(route.Spec.ParentRefs) == 0 {
		return false
	}

	if len(route.Status.RouteStatus.Parents) != len(route.Spec.ParentRefs) {
		// HTTPRoute is ready only when _all_ parents have accepted the route.
		return false
	}

	if cond, missing := nonReadyHTTPRouteTopLevelCondition(route); cond != nil || missing {
		return false
	}

	return true
}

func nonReadyHTTPRouteTopLevelCondition(route *gatewayapi.HTTPRoute) (*metav1.Condition, bool) {
	if route == nil {
		return nil, true
	}

	for _, parent := range route.Status.RouteStatus.Parents {
		cond := meta.FindStatusCondition(parent.Conditions, string(gatewayapi.RouteConditionAccepted))
		if cond == nil {
			return nil, true
		}
		staleCondition := cond.ObservedGeneration > 0 && cond.ObservedGeneration < route.Generation
		if cond.Status != metav1.ConditionTrue || staleCondition {
			return cond, false
		}
	}

	return nil, false
}

// IsInferencePoolReady checks if an InferencePool has been accepted by all parents.
func IsInferencePoolReady(pool *igwapi.InferencePool) bool {
	if pool == nil || len(pool.Status.Parents) == 0 {
		return false
	}

	if cond, missing := nonReadyInferencePoolTopLevelCondition(pool); cond != nil || missing {
		return false
	}

	return true
}

func nonReadyInferencePoolTopLevelCondition(pool *igwapi.InferencePool) (*metav1.Condition, bool) {
	if pool == nil {
		return nil, true
	}

	for _, parent := range pool.Status.Parents {
		cond := meta.FindStatusCondition(parent.Conditions, string(igwapi.InferencePoolConditionAccepted))
		if cond == nil {
			return nil, true
		}
		staleCondition := cond.ObservedGeneration > 0 && cond.ObservedGeneration < pool.Generation
		if cond.Status != metav1.ConditionTrue || staleCondition {
			return cond, false
		}
	}

	return nil, false
}
