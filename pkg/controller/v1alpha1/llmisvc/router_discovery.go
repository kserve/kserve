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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// resolvedGateway contains a Gateway and its associated GatewayClass
// This provides all the information needed to understand gateway capabilities
type resolvedGateway struct {
	gateway      *gwapiv1.Gateway
	gatewayClass *gwapiv1.GatewayClass
	parentRef    gwapiv1.ParentReference
}

// DiscoverGateways finds and resolves all gateways referenced by an HTTPRoute
// It fetches the Gateway and GatewayClass resources to provide complete routing context
func DiscoverGateways(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute) ([]resolvedGateway, error) {
	gateways := make([]resolvedGateway, 0)
	for _, parentRef := range route.Spec.ParentRefs {
		// Resolve namespace (defaults to route's namespace if not specified)
		ns := ptr.Deref((&parentRef).Namespace, gwapiv1.Namespace(route.Namespace))
		gwNS, gwName := string(ns), string((&parentRef).Name)

		gateway := &gwapiv1.Gateway{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: gwNS, Name: gwName}, gateway); err != nil {
			return nil, fmt.Errorf("failed to get Gateway %s/%s for route %s/%s: %w", gwNS, gwName, route.Namespace, route.Name, err)
		}

		gatewayClass := &gwapiv1.GatewayClass{}
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

// DiscoverURLs extracts accessible URLs from an HTTPRoute by examining its gateways
// It constructs URLs based on gateway listeners and addresses
func DiscoverURLs(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute) ([]*apis.URL, error) {
	var urls []*apis.URL

	gateways, err := DiscoverGateways(ctx, c, route)
	if err != nil {
		return nil, fmt.Errorf("failed to discover gateways: %w", err)
	}

	// Extract URLs from each gateway based on its listeners and addresses
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

// extractRoutePath extracts the path from the first rule of an HTTPRoute
// This is used to construct the complete URL for the route
// Currently only handles exact path matches, not regex patterns
func extractRoutePath(route *gwapiv1.HTTPRoute) string {
	if len(route.Spec.Rules) > 0 && len(route.Spec.Rules[0].Matches) > 0 {
		// TODO how do we deal with regexp
		return ptr.Deref(route.Spec.Rules[0].Matches[0].Path.Value, "/")
	}
	return "/"
}

// selectListener chooses the appropriate listener from a Gateway
// If a specific sectionName is provided, it searches for that listener by name
// Otherwise, it defaults to the first listener in the Gateway
func selectListener(gateway *gwapiv1.Gateway, sectionName *gwapiv1.SectionName) *gwapiv1.Listener {
	if sectionName != nil {
		// Search for the specifically named listener
		for _, listener := range gateway.Spec.Listeners {
			if listener.Name == *sectionName {
				return &listener
			}
		}
	}

	// Default to the first listener if no specific section is requested
	return &gateway.Spec.Listeners[0]
}

// extractSchemeFromListener determines the URL scheme (http/https) based on the listener protocol
// This is essential for constructing valid URLs that match the Gateway's configuration
func extractSchemeFromListener(listener *gwapiv1.Listener) string {
	if listener.Protocol == gwapiv1.HTTPSProtocolType {
		return "https"
	}
	// Default to HTTP for all other protocols (HTTP, TCP, etc.)
	return "http"
}

// extractRouteHostnames extracts valid hostnames from an HTTPRoute specification
// It filters out empty strings and wildcard hostnames which cannot be used in URLs
func extractRouteHostnames(route *gwapiv1.HTTPRoute) []string {
	var hostnames []string
	for _, h := range route.Spec.Hostnames {
		host := string(h)
		// Skip empty hostnames and wildcards as they cannot form valid URLs
		if host != "" && host != "*" {
			hostnames = append(hostnames, host)
		}
	}
	return hostnames
}

// extractAddressValues extracts the address values from Gateway status addresses
// These addresses are typically IP addresses or hostnames where the Gateway is accessible
// Used as fallback when no specific hostnames are defined in the HTTPRoute
func extractAddressValues(addresses []gwapiv1.GatewayStatusAddress) []string {
	var values []string
	for _, addr := range addresses {
		// Only include non-empty address values
		if addr.Value != "" {
			values = append(values, addr.Value)
		}
	}
	return values
}

// combineIntoURLs constructs complete URLs from hostnames, scheme, port, and path components
// It handles standard ports (80 for HTTP, 443 for HTTPS) by omitting them from the URL
// Returns a sorted list of URLs for consistent ordering
func combineIntoURLs(hostnames []string, scheme string, port gwapiv1.PortNumber, path string) ([]*apis.URL, error) {
	urls := make([]*apis.URL, 0, len(hostnames))

	// Sort hostnames for consistent URL ordering
	sortedHostnames := make([]string, len(hostnames))
	copy(sortedHostnames, hostnames)
	slices.Sort(sortedHostnames)

	for _, hostname := range sortedHostnames {
		var urlStr string
		// Include port in URL only if it's not the standard port for the scheme
		if (scheme == "http" && port != 80) || (scheme == "https" && port != 443) {
			urlStr = fmt.Sprintf("%s://%s%s", scheme, joinHostPort(hostname, &port), path)
		} else {
			// Use standard port - omit from URL for cleaner appearance
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

// joinHostPort safely combines a hostname and port into a host:port string
// Uses net.JoinHostPort to handle IPv6 addresses correctly with brackets
// Returns just the host if port is nil or zero
func joinHostPort(host string, port *gwapiv1.PortNumber) string {
	if port != nil && *port != 0 {
		return net.JoinHostPort(host, fmt.Sprint(*port))
	}
	return host
}

// ExternalAddressNotFoundError indicates that a Gateway has no external addresses
// This typically occurs when the Gateway is not yet provisioned by the infrastructure
type ExternalAddressNotFoundError struct {
	GatewayNamespace string
	GatewayName      string
}

func (e *ExternalAddressNotFoundError) Error() string {
	return fmt.Sprintf("Gateway %s/%s has no external address found", e.GatewayNamespace, e.GatewayName)
}

// IgnoreExternalAddressNotFound converts ExternalAddressNotFoundError to nil
// This is useful when external addresses are optional or may not be immediately available
func IgnoreExternalAddressNotFound(err error) error {
	if IsExternalAddressNotFound(err) {
		return nil
	}
	return err
}

// IsExternalAddressNotFound checks if an error is of type ExternalAddressNotFoundError
func IsExternalAddressNotFound(err error) bool {
	var externalAddrNotFoundErr *ExternalAddressNotFoundError
	return errors.As(err, &externalAddrNotFoundErr)
}

// EvaluateGatewayReadiness checks the readiness status of Gateways and returns those that are not ready
// This is used to determine if routing can proceed or if we need to wait for Gateway provisioning
func EvaluateGatewayReadiness(ctx context.Context, gateways []*gwapiv1.Gateway) []*gwapiv1.Gateway {
	logger := log.FromContext(ctx)
	notReadyGateways := make([]*gwapiv1.Gateway, 0)

	for _, gateway := range gateways {
		ready := IsGatewayReady(gateway)
		logger.Info("Gateway readiness evaluated", "gateway", fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name), "ready", ready)

		// Collect gateways that are not ready for status reporting
		if !ready {
			notReadyGateways = append(notReadyGateways, gateway)
		}
	}

	return notReadyGateways
}

// IsGatewayReady determines if a Gateway is ready based on its status conditions
// A Gateway is considered ready when it has a "Programmed" condition with status True
// The "Programmed" condition indicates that the Gateway has been configured and is operational
func IsGatewayReady(gateway *gwapiv1.Gateway) bool {
	// Check for the standard Gateway API "Programmed" condition
	for _, condition := range gateway.Status.Conditions {
		if condition.Type == string(gwapiv1.GatewayConditionProgrammed) {
			return condition.Status == metav1.ConditionTrue
		}
	}

	// If no Programmed condition is found, Gateway is considered not ready
	return false
}

// EvaluateHTTPRouteReadiness checks the readiness status of HTTPRoutes and returns those that are not ready
// This helps determine if traffic routing is functional or if there are configuration issues
func EvaluateHTTPRouteReadiness(ctx context.Context, routes []*gwapiv1.HTTPRoute) []*gwapiv1.HTTPRoute {
	logger := log.FromContext(ctx)
	notReadyRoutes := make([]*gwapiv1.HTTPRoute, 0)

	for _, route := range routes {
		ready := IsHTTPRouteReady(route)
		logger.Info("HTTPRoute readiness evaluated", "route", fmt.Sprintf("%s/%s", route.Namespace, route.Name), "ready", ready)

		// Collect routes that are not ready for status reporting
		if !ready {
			notReadyRoutes = append(notReadyRoutes, route)
		}
	}

	return notReadyRoutes
}

// IsHTTPRouteReady determines if an HTTPRoute is ready based on its status conditions
// An HTTPRoute is ready only when ALL parent Gateways have accepted it
// This ensures that traffic can flow through all configured paths
func IsHTTPRouteReady(route *gwapiv1.HTTPRoute) bool {
	if route == nil || len(route.Spec.ParentRefs) == 0 {
		return false
	}

	// Check that all parent references have corresponding status entries
	if len(route.Status.RouteStatus.Parents) != len(route.Spec.ParentRefs) {
		// HTTPRoute is ready only when _all_ parents have accepted the route.
		return false
	}

	// Check for any non-ready conditions across all parents
	if cond, missing := nonReadyHTTPRouteTopLevelCondition(route); cond != nil || missing {
		return false
	}

	return true
}

// nonReadyHTTPRouteTopLevelCondition checks for any non-ready conditions in an HTTPRoute
// Returns the first problematic condition found, or indicates if any conditions are missing
// A condition is considered stale if its ObservedGeneration is less than the route's current Generation
func nonReadyHTTPRouteTopLevelCondition(route *gwapiv1.HTTPRoute) (*metav1.Condition, bool) {
	if route == nil {
		return nil, true
	}

	for _, parent := range route.Status.RouteStatus.Parents {
		// Look for the "Accepted" condition which indicates Gateway acceptance
		cond := meta.FindStatusCondition(parent.Conditions, string(gwapiv1.RouteConditionAccepted))
		if cond == nil {
			// Missing condition indicates the route is not ready
			return nil, true
		}
		// Check if condition is stale (based on older generation)
		staleCondition := cond.ObservedGeneration > 0 && cond.ObservedGeneration < route.Generation
		if cond.Status != metav1.ConditionTrue || staleCondition {
			return cond, false
		}
	}

	return nil, false
}

// IsInferencePoolReady checks if an InferencePool has been accepted by all parents
// InferencePools manage collections of inference workloads for load balancing
// They must be accepted by their parent Gateways to be considered operational
func IsInferencePoolReady(pool *igwapi.InferencePool) bool {
	if pool == nil || len(pool.Status.Parents) == 0 {
		return false
	}

	// Check for any non-ready conditions across all parents
	if cond, missing := nonReadyInferencePoolTopLevelCondition(pool); cond != nil || missing {
		return false
	}

	return true
}

// nonReadyInferencePoolTopLevelCondition checks for any non-ready conditions in an InferencePool
// Similar to HTTPRoute validation but uses InferencePool-specific condition types
// Returns the first problematic condition or indicates missing conditions
func nonReadyInferencePoolTopLevelCondition(pool *igwapi.InferencePool) (*metav1.Condition, bool) {
	if pool == nil {
		return nil, true
	}

	for _, parent := range pool.Status.Parents {
		// Look for the "Accepted" condition specific to InferencePools
		cond := meta.FindStatusCondition(parent.Conditions, string(igwapi.InferencePoolConditionAccepted))
		if cond == nil {
			// Missing condition indicates the pool is not ready
			return nil, true
		}
		// Check if condition is stale (based on older generation)
		staleCondition := cond.ObservedGeneration > 0 && cond.ObservedGeneration < pool.Generation
		if cond.Status != metav1.ConditionTrue || staleCondition {
			return cond, false
		}
	}

	return nil, false
}
