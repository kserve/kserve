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
	"net"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/constants"
)

var wildcardHostname = constants.GetEnvOrDefault("GATEWAY_API_WILDCARD_HOSTNAME", "inference")

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

// DiscoverGatewayServiceHost attempts to find the cluster-local hostname
// for the Service backing a Gateway.
// Returns empty string if no backing service is found (not an error).
func DiscoverGatewayServiceHost(ctx context.Context, c client.Client, gateway *gwapiv1.Gateway) (string, error) {
	logger := log.FromContext(ctx)

	// Look for Service with known gateway label first
	svcList := &corev1.ServiceList{}
	if err := c.List(ctx, svcList,
		client.InNamespace(gateway.Namespace),
		client.MatchingLabels{
			"gateway.networking.k8s.io/gateway-name": gateway.Name,
		},
	); err != nil {
		return "", fmt.Errorf("failed to list services for gateway %s/%s: %w", gateway.Namespace, gateway.Name, err)
	}
	if len(svcList.Items) > 0 {
		if len(svcList.Items) > 1 {
			logger.Info("Multiple services found with gateway label, using first alphabetically",
				"gateway", gateway.Name, "count", len(svcList.Items))
			slices.SortFunc(svcList.Items, func(a, b corev1.Service) int {
				return cmp.Compare(a.Name, b.Name)
			})
		}
		svc := &svcList.Items[0]
		host := network.GetServiceHostname(svc.Name, svc.Namespace)
		logger.V(1).Info("Discovered gateway service via label", "gateway", gateway.Name, "service", svc.Name, "host", host)
		return host, nil
	}

	// Fallback: Look for Service with the same name as Gateway
	svc := &corev1.Service{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: gateway.Namespace,
		Name:      gateway.Name,
	}, svc)
	if err == nil {
		host := network.GetServiceHostname(svc.Name, svc.Namespace)
		logger.V(1).Info("Discovered gateway service via name match", "gateway", gateway.Name, "service", svc.Name, "host", host)
		return host, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get service %s/%s: %w", gateway.Namespace, gateway.Name, err)
	}

	// No backing service found - not an error - we are guessing here
	logger.V(1).Info("No backing service found for gateway", "gateway", fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name))
	return "", nil
}

// DiscoverURLs extracts accessible URLs from an HTTPRoute by examining its gateways
// It constructs URLs based on gateway listeners and addresses, and also discovers
// internal URLs from backing services
func DiscoverURLs(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute, preferredUrlScheme string) ([]*apis.URL, error) {
	var urls []*apis.URL

	gateways, err := DiscoverGateways(ctx, c, route)
	if err != nil {
		return nil, fmt.Errorf("failed to discover gateways: %w", err)
	}

	for _, g := range gateways {
		listeners, err := selectListeners(g.gateway, g.parentRef.SectionName, preferredUrlScheme)
		if err != nil {
			return nil, fmt.Errorf("failed to select listeners for gateway %s/%s: %w", g.gateway.Namespace, g.gateway.Name, err)
		}

		path := extractRoutePath(route)
		addresses := g.gateway.Status.Addresses

		// Discover external URLs from Gateway status addresses (if available)
		if len(addresses) > 0 {
			for _, listener := range listeners {
				scheme, err := resolveScheme(listener)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve scheme for gateway %s/%s listener %s: %w",
						g.gateway.Namespace, g.gateway.Name, listener.Name, err)
				}

				hostnames := extractHostnamesForListener(route, listener, addresses)
				gatewayURLs, err := combineIntoURLs(hostnames, scheme, listener.Port, path)
				if err != nil {
					return nil, fmt.Errorf("failed to combine URLs for Gateway %s/%s: %w", g.gateway.Namespace, g.gateway.Name, err)
				}
				urls = append(urls, gatewayURLs...)
			}
		}

		// Discover internal URL from Gateway backing service
		internalHost, err := DiscoverGatewayServiceHost(ctx, c, g.gateway)
		if err != nil {
			return nil, fmt.Errorf("failed to discover gateway service host for %s/%s: %w", g.gateway.Namespace, g.gateway.Name, err)
		}
		if internalHost != "" {
			// Use preferred (first) listener's scheme and port for the internal URL.
			// Internal services typically expose a single protocol, so generating one
			// URL (matching the preferred scheme) is sufficient. External URLs are
			// generated for all listeners because clients may need either protocol.
			listener := listeners[0]
			internalURLs, err := combineIntoURLs([]string{internalHost}, schemeForProtocol(listener.Protocol), listener.Port, path)
			if err != nil {
				return nil, fmt.Errorf("failed to build internal URL for Gateway %s/%s: %w", g.gateway.Namespace, g.gateway.Name, err)
			}
			urls = append(urls, internalURLs...)
		}
	}

	// Error only if no URLs discovered at all (neither external nor internal)
	if len(urls) == 0 && len(gateways) > 0 {
		g := gateways[0]
		return nil, &NoURLsDiscoveredError{
			GatewayNamespace: g.gateway.Namespace,
			GatewayName:      g.gateway.Name,
		}
	}

	return urls, nil
}

// extractHostnamesForListener determines the hostnames to use for URL generation
func extractHostnamesForListener(route *gwapiv1.HTTPRoute, listener *gwapiv1.Listener, addresses []gwapiv1.GatewayStatusAddress) []string {
	hostnames := extractRouteHostnames(route)
	// If Hostname is set in the spec, use the Hostname specified.
	// Using the LoadBalancer addresses in `Gateway.Status.Addresses` will return 404 in those cases.
	if len(hostnames) == 0 && listener.Hostname != nil && *listener.Hostname != "" {
		if host, isWildcard := strings.CutPrefix(string(*listener.Hostname), "*."); isWildcard {
			// Hostnames that are prefixed with a wildcard label (`*.`) are interpreted
			// as a suffix match. That means that a match for `*.example.com` would match
			// both `test.example.com`, and `foo.test.example.com`, but not `example.com`.
			hostnames = append(hostnames, fmt.Sprintf("%s.%s", wildcardHostname, host))
		} else {
			hostnames = []string{host}
		}
	}
	if len(hostnames) == 0 {
		hostnames = extractAddressValues(addresses)
	}
	return hostnames
}

// extractRoutePath selects the base URL path from an HTTPRoute's rules.
// It assumes paths form a hierarchy (e.g. /ns/name, /ns/name/v1/completions)
// where the shallowest path is the logical base URL to advertise in status.
// Paths from rules with a Service backend take priority over others (e.g. InferencePool).
// Among candidates, the path with the fewest "/" segments is chosen, with
// string length as a tiebreaker.
// Note: this only handles PathPrefix matches correctly; Exact and RegularExpression
// match types are not distinguished.
func extractRoutePath(route *gwapiv1.HTTPRoute) string {
	var servicePaths, otherPaths []string
	for _, rule := range route.Spec.Rules {
		svc := hasServiceBackend(rule)
		for _, match := range rule.Matches {
			if match.Path == nil {
				continue
			}
			p := ptr.Deref(match.Path.Value, "/")
			if svc {
				servicePaths = append(servicePaths, p)
			} else {
				otherPaths = append(otherPaths, p)
			}
		}
	}

	// TODO how do we deal with regexp
	// TODO how do we intelligently handle multiple rules
	paths := servicePaths
	if len(paths) == 0 {
		paths = otherPaths
	}
	if len(paths) == 0 {
		return "/"
	}

	return slices.MinFunc(paths, func(a, b string) int {
		if d := strings.Count(a, "/") - strings.Count(b, "/"); d != 0 {
			return d
		}
		return len(a) - len(b)
	})
}

// hasServiceBackend returns true if the rule has at least one backendRef with Kind "Service"
// or with no Kind set (defaults to Service per Gateway API spec).
func hasServiceBackend(rule gwapiv1.HTTPRouteRule) bool {
	for _, ref := range rule.BackendRefs {
		kind := ptr.Deref(ref.Kind, gwapiv1.Kind("Service"))
		if kind == "Service" {
			return true
		}
	}
	return false
}

// schemeForProtocol returns the URL scheme for a Gateway API protocol.
// Returns empty string for protocols that don't support HTTP routing.
func schemeForProtocol(protocol gwapiv1.ProtocolType) string {
	switch protocol {
	case gwapiv1.HTTPProtocolType, gwapiv1.HTTPSProtocolType:
		return strings.ToLower(string(protocol))
	case gwapiv1.TLSProtocolType:
		return "https"
	default:
		return ""
	}
}

// selectListeners returns the applicable listeners for URL generation.
// - If sectionName is provided, returns only that specific listener
// - Otherwise, returns ALL HTTP-capable listeners sorted by: preferredScheme first, then HTTPS, then HTTP
func selectListeners(gateway *gwapiv1.Gateway, sectionName *gwapiv1.SectionName, preferredScheme string) ([]*gwapiv1.Listener, error) {
	// If sectionName provided, find exact match (single listener)
	if sectionName != nil {
		for i := range gateway.Spec.Listeners {
			if gateway.Spec.Listeners[i].Name == *sectionName {
				return []*gwapiv1.Listener{&gateway.Spec.Listeners[i]}, nil
			}
		}
		return nil, fmt.Errorf("listener %q not found in gateway %s/%s", *sectionName, gateway.Namespace, gateway.Name)
	}

	// Collect all HTTP-capable listeners
	var listeners []*gwapiv1.Listener
	for i := range gateway.Spec.Listeners {
		if scheme := schemeForProtocol(gateway.Spec.Listeners[i].Protocol); scheme != "" {
			listeners = append(listeners, &gateway.Spec.Listeners[i])
		}
	}
	if len(listeners) == 0 {
		return nil, fmt.Errorf("no HTTP-capable listener found in gateway %s/%s", gateway.Namespace, gateway.Name)
	}

	// Sort: preferredScheme first, then HTTPS before HTTP
	precedence := func(l *gwapiv1.Listener) int {
		scheme := schemeForProtocol(l.Protocol)
		if scheme == preferredScheme {
			return 0
		}
		if scheme == "https" {
			return 1
		}
		return 2
	}
	slices.SortFunc(listeners, func(a, b *gwapiv1.Listener) int {
		return precedence(a) - precedence(b)
	})

	return listeners, nil
}

// resolveScheme returns the URL scheme derived from the listener's protocol.
func resolveScheme(listener *gwapiv1.Listener) (string, error) {
	scheme := schemeForProtocol(listener.Protocol)
	if scheme == "" {
		return "", fmt.Errorf("listener %q uses unsupported protocol %s for HTTP routing", listener.Name, listener.Protocol)
	}
	return scheme, nil
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
			urlStr = fmt.Sprintf("%s://%s%s", scheme, joinHostPort(hostname, nil), path)
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
// Returns just the host if port is nil or zero (with IPv6 bracket wrapping)
func joinHostPort(host string, port *gwapiv1.PortNumber) string {
	if port != nil && *port != 0 {
		return net.JoinHostPort(host, strconv.Itoa(int(*port)))
	}
	// IPv6 addresses need brackets in URLs even without an explicit port
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// NoURLsDiscoveredError indicates that no URLs could be discovered for a Gateway
// (neither external addresses nor internal backing service found)
type NoURLsDiscoveredError struct {
	GatewayNamespace string
	GatewayName      string
}

func (e *NoURLsDiscoveredError) Error() string {
	return fmt.Sprintf("no URLs discovered for Gateway %s/%s (no external addresses and no backing service found)", e.GatewayNamespace, e.GatewayName)
}

// IgnoreNoURLsDiscovered converts NoURLsDiscoveredError to nil
// This is useful when URLs may not be immediately available during provisioning
func IgnoreNoURLsDiscovered(err error) error {
	if IsNoURLsDiscovered(err) {
		return nil
	}
	return err
}

// IsNoURLsDiscovered checks if an error is of type NoURLsDiscoveredError
func IsNoURLsDiscovered(err error) bool {
	var noURLsErr *NoURLsDiscoveredError
	return errors.As(err, &noURLsErr)
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

	// Check that every spec ParentRef has at least one corresponding status entry.
	// Multiple controllers may write separate status entries for the same ParentRef,
	// so we only require that at least one entry exists per spec ref.
	for _, specRef := range route.Spec.ParentRefs {
		if !hasMatchingParentStatus(specRef, route.Status.Parents, gwapiv1.Namespace(route.Namespace)) {
			return false
		}
	}

	return areGatewayConditionsReady(route)
}

// hasMatchingParentStatus checks whether at least one RouteParentStatus entry
// corresponds to the given spec ParentReference and was written by a gateway
// controller (i.e., has the Accepted condition set). Policy or extension
// controllers may also write status entries for the same ParentRef but without
// setting the Accepted condition, so those entries alone are not sufficient.
func hasMatchingParentStatus(specRef gwapiv1.ParentReference, parents []gwapiv1.RouteParentStatus, defaultNS gwapiv1.Namespace) bool {
	for i := range parents {
		if parentRefMatches(specRef, parents[i].ParentRef, defaultNS) &&
			meta.FindStatusCondition(parents[i].Conditions, string(gwapiv1.RouteConditionAccepted)) != nil {
			return true
		}
	}
	return false
}

// parentRefMatches returns true when two ParentReferences identify the same parent,
// applying Gateway API defaulting rules for optional fields.
// defaultNS is used when Namespace is omitted (nil) from a ParentReference, which
// per Gateway API spec defaults to the route's own namespace.
func parentRefMatches(a, b gwapiv1.ParentReference, defaultNS gwapiv1.Namespace) bool {
	return a.Name == b.Name &&
		ptr.Deref(a.Group, gwapiv1.GroupName) == ptr.Deref(b.Group, gwapiv1.GroupName) &&
		ptr.Deref(a.Kind, "Gateway") == ptr.Deref(b.Kind, "Gateway") &&
		ptr.Deref(a.Namespace, defaultNS) == ptr.Deref(b.Namespace, defaultNS) &&
		ptr.Deref(a.SectionName, "") == ptr.Deref(b.SectionName, "") &&
		ptr.Deref(a.Port, 0) == ptr.Deref(b.Port, 0)
}

// areGatewayConditionsReady reports whether all gateway controller entries in the
// HTTPRoute's status have Accepted and ResolvedRefs set to True and up-to-date.
// Status entries without the Accepted condition (e.g. from policy controllers) are skipped.
// A condition is stale when its ObservedGeneration is less than the route's Generation.
func areGatewayConditionsReady(route *gwapiv1.HTTPRoute) bool {
	if route == nil || len(route.Status.Parents) == 0 {
		return false
	}
	for _, parent := range route.Status.Parents {
		acceptedCond := meta.FindStatusCondition(parent.Conditions, string(gwapiv1.RouteConditionAccepted))
		if acceptedCond == nil {
			continue
		}
		stale := acceptedCond.ObservedGeneration > 0 && acceptedCond.ObservedGeneration < route.Generation
		if acceptedCond.Status != metav1.ConditionTrue || stale {
			return false
		}
		resolvedRefCond := meta.FindStatusCondition(parent.Conditions, string(gwapiv1.RouteConditionResolvedRefs))
		if resolvedRefCond == nil {
			return false
		}
		stale = resolvedRefCond.ObservedGeneration > 0 && resolvedRefCond.ObservedGeneration < route.Generation
		if resolvedRefCond.Status != metav1.ConditionTrue || stale {
			return false
		}
	}
	return true
}

// findNonReadyGatewayCondition returns the first non-ready or stale condition from a
// gateway controller entry in the HTTPRoute's status, or nil if none found.
// Status entries without the Accepted condition (e.g. from policy controllers) are skipped.
func findNonReadyGatewayCondition(route *gwapiv1.HTTPRoute) *metav1.Condition {
	if route == nil {
		return nil
	}
	for _, parent := range route.Status.Parents {
		acceptedCond := meta.FindStatusCondition(parent.Conditions, string(gwapiv1.RouteConditionAccepted))
		if acceptedCond == nil {
			continue
		}
		stale := acceptedCond.ObservedGeneration > 0 && acceptedCond.ObservedGeneration < route.Generation
		if acceptedCond.Status != metav1.ConditionTrue || stale {
			return acceptedCond
		}
		resolvedRefCond := meta.FindStatusCondition(parent.Conditions, string(gwapiv1.RouteConditionResolvedRefs))
		if resolvedRefCond == nil {
			// ResolvedRefs not yet reported — areGatewayConditionsReady treats this as not-ready,
			// so surface a synthesized condition for diagnostics.
			return &metav1.Condition{
				Type:    string(gwapiv1.RouteConditionResolvedRefs),
				Status:  metav1.ConditionUnknown,
				Reason:  "ConditionMissing",
				Message: "ResolvedRefs condition not yet reported by gateway controller",
			}
		}
		stale = resolvedRefCond.ObservedGeneration > 0 && resolvedRefCond.ObservedGeneration < route.Generation
		if resolvedRefCond.Status != metav1.ConditionTrue || stale {
			return resolvedRefCond
		}
	}
	return nil
}

// IsInferencePoolReady checks if an InferencePool has been accepted by all parents
// InferencePools manage collections of inference workloads for load balancing
// They must be accepted by their parent Gateways to be considered operational
func IsInferencePoolReady(pool *igwapi.InferencePool) bool {
	if pool == nil {
		return false
	}

	// If no parents have been set, consider the pool ready if it exists and has a valid spec
	// This handles cases where no Gateway controller is populating the status
	if len(pool.Status.Parents) == 0 {
		// Pool is ready if it exists with a valid selector and target ports
		return len(pool.Spec.Selector.MatchLabels) > 0 && len(pool.Spec.TargetPorts) > 0
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

// IsInferencePoolV1Alpha2Supported checks if an HTTPRoute has been accepted by the Gateway, and it's using v1alpha2
// InferencePool.
func IsInferencePoolV1Alpha2Supported(route *gwapiv1.HTTPRoute) metav1.ConditionStatus {
	if isHTTPRouteUsingInferencePool(route, constants.InferencePoolV1Alpha2APIGroupName) {
		return isBackendSupported(route)
	}
	return metav1.ConditionUnknown
}

// IsInferencePoolV1Supported checks if an HTTPRoute has been accepted by the Gateway, and it's using v1 InferencePool.
func IsInferencePoolV1Supported(route *gwapiv1.HTTPRoute) metav1.ConditionStatus {
	if isHTTPRouteUsingInferencePool(route, constants.InferencePoolV1APIGroupName) {
		return isBackendSupported(route)
	}
	return metav1.ConditionUnknown
}

// isBackendSupported only returns false if we're absolutely sure the backend is unsupported
func isBackendSupported(route *gwapiv1.HTTPRoute) metav1.ConditionStatus {
	if route == nil {
		return metav1.ConditionUnknown
	}

	// Check the first parent's status (TODO: filter to our specific gateway)
	if len(route.Status.Parents) > 0 {
		parent := route.Status.Parents[0]
		cond := meta.FindStatusCondition(parent.Conditions, string(gwapiv1.RouteConditionResolvedRefs))
		if cond == nil {
			return metav1.ConditionUnknown
		}
		if cond.Status == metav1.ConditionFalse && cond.Reason == string(gwapiv1.RouteReasonInvalidKind) {
			return metav1.ConditionFalse
		}
		return metav1.ConditionTrue
	}

	return metav1.ConditionUnknown
}

func isHTTPRouteUsingInferencePool(route *gwapiv1.HTTPRoute, group string) bool {
	if route == nil {
		return false
	}

	for _, r := range route.Spec.Rules {
		for _, b := range r.BackendRefs {
			if b.Group != nil && string(*b.Group) == group &&
				b.Kind != nil && *b.Kind == "InferencePool" {
				return true
			}
		}
	}

	return false
}
