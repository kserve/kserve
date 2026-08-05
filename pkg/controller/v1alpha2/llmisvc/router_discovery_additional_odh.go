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
	"cmp"
	"context"
	"fmt"
	"slices"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// gatewayWithPaths pairs a gateway reference with the URL paths discovered for it.
type gatewayWithPaths struct {
	ref   gwapiv1.ObjectReference
	paths []string
}

// discoverAdditionalURLs discovers OpenShift Routes that provide external
// access to Gateways lacking external URLs in their status addresses.
func (r *LLMISVCReconciler) discoverAdditionalURLs(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, discovered []DiscoveredURL) ([]DiscoveredURL, error) {
	gateways := gatewaysWithoutExternalURLs(discovered)
	if len(gateways) == 0 {
		return nil, nil
	}

	existingURLs := make(map[string]bool, len(discovered))
	for _, d := range discovered {
		existingURLs[d.URL.String()] = true
	}

	var additional []DiscoveredURL

	for _, gw := range gateways {
		routeURLs, err := r.discoverRouteURLs(ctx, llmSvc, gw)
		if err != nil {
			return nil, fmt.Errorf("failed to discover Routes for gateway %s/%s: %w",
				ptr.Deref(gw.ref.Namespace, ""), gw.ref.Name, err)
		}

		for _, u := range routeURLs {
			if !existingURLs[u.URL.String()] {
				additional = append(additional, u)
				existingURLs[u.URL.String()] = true
			}
		}
	}

	return additional, nil
}

// discoverRouteURLs finds OpenShift Routes that front a gateway's services
// and returns DiscoveredURLs with Route origin - one per Route per path.
func (r *LLMISVCReconciler) discoverRouteURLs(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, gw gatewayWithPaths) ([]DiscoveredURL, error) {
	ns := string(ptr.Deref(gw.ref.Namespace, ""))
	gwName := string(gw.ref.Name)

	gwServices, err := r.gatewayServices(ctx, ns, gwName)
	if err != nil {
		return nil, err
	}
	if len(gwServices) == 0 {
		return nil, nil
	}

	routeList := &routev1.RouteList{}
	if err := r.List(ctx, routeList, client.InNamespace(ns)); err != nil {
		if meta.IsNoMatchError(err) {
			log.FromContext(ctx).V(1).Info("OpenShift Route CRD (route.openshift.io/v1) not available, skipping Route discovery")

			return nil, nil
		}

		return nil, fmt.Errorf("failed to list Routes in %s: %w", ns, err)
	}

	var urls []DiscoveredURL
	for i := range routeList.Items {
		route := &routeList.Items[i]

		// Kind defaults to "Service" when empty per OpenShift API
		if (route.Spec.To.Kind != "" && route.Spec.To.Kind != "Service") || gwServices[route.Spec.To.Name] == nil {
			continue
		}

		host := admittedHost(route)
		if host == "" {
			continue
		}

		// Routes with path-based routing are incompatible with gateway URL
		// discovery. HAProxy uses different regex patterns for rewrite-target
		// "/" vs other values, making reliable reverse-computation of the
		// client-facing URL fragile. In practice, Routes fronting gateway
		// services are host-only; path-based variants are rare and likely
		// misconfigured for this use case.
		if route.Spec.Path != "" {
			r.Eventf(llmSvc, corev1.EventTypeWarning, "RoutePathIncompatible",
				"Route %s/%s has path-based routing (path=%q) incompatible with gateway URL discovery - skipping",
				route.Namespace, route.Name, route.Spec.Path)

			continue
		}

		if mismatch := checkRoutePortMismatch(route, gwServices); mismatch != "" {
			r.Eventf(llmSvc, corev1.EventTypeWarning, "RoutePortMismatch",
				"Route %s/%s has a port mismatch: %s",
				route.Namespace, route.Name, mismatch)
		}

		if mismatch := checkRouteTLSMismatch(route, gwServices); mismatch != "" {
			r.Eventf(llmSvc, corev1.EventTypeWarning, "RouteTLSMismatch",
				"Route %s/%s has a TLS mismatch: %s",
				route.Namespace, route.Name, mismatch)
		}

		origin := &gwapiv1.ObjectReference{
			Group:     gwapiv1.Group(routev1.GroupName),
			Kind:      "Route",
			Name:      gwapiv1.ObjectName(route.Name),
			Namespace: ptr.To(gwapiv1.Namespace(route.Namespace)),
		}

		for _, gwPath := range gw.paths {
			u := routeURL(route, host)
			u.Path = gwPath

			urls = append(urls, DiscoveredURL{URL: u, Origin: origin})
		}
	}

	return urls, nil
}

// gatewayServices returns all Services in the namespace whose pod selector
// routes traffic to the given gateway's pods, keyed by service name.
// Uses Spec.Selector (not metadata labels) because operator-created services
// (e.g. <gw>-data-science-gateway-class) may lack the gateway label on their
// metadata but still select the same pods via the pod selector.
func (r *LLMISVCReconciler) gatewayServices(ctx context.Context, ns, gwName string) (map[string]*corev1.Service, error) {
	svcList := &corev1.ServiceList{}
	if err := r.List(ctx, svcList, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("failed to list Services in %s: %w", ns, err)
	}

	services := make(map[string]*corev1.Service)
	for i := range svcList.Items {
		if svcList.Items[i].Spec.Selector[gatewayNameLabel] == gwName {
			services[svcList.Items[i].Name] = &svcList.Items[i]
		}
	}

	return services, nil
}

// checkRoutePortMismatch checks whether the Route's target port matches
// any port on the target Service. Returns a human-readable reason if there
// is a mismatch, or empty string if the port is valid or unspecified.
func checkRoutePortMismatch(route *routev1.Route, gwServices map[string]*corev1.Service) string {
	if route.Spec.Port == nil {
		return ""
	}

	svc, ok := gwServices[route.Spec.To.Name]
	if !ok {
		return ""
	}

	if _, ok := findServicePort(route.Spec.Port.TargetPort, svc); ok {
		return ""
	}

	return fmt.Sprintf("target port %s does not match any port on Service %s/%s",
		route.Spec.Port.TargetPort.String(), svc.Namespace, svc.Name)
}

// checkRouteTLSMismatch checks whether the Route's TLS termination mode is
// compatible with the target Service's declared appProtocol. Returns a
// human-readable reason if there is a mismatch, or empty string if compatible
// or if the protocol cannot be determined (best-effort, no false positives).
//
// edge/no-TLS Routes send plain HTTP to the backend; passthrough/reencrypt
// Routes send TLS. If the Service port declares an appProtocol that
// contradicts the Route's termination mode, the backend connection will fail.
func checkRouteTLSMismatch(route *routev1.Route, gwServices map[string]*corev1.Service) string {
	svc, ok := gwServices[route.Spec.To.Name]
	if !ok {
		return ""
	}

	appProto := resolveAppProtocol(route, svc)
	if appProto == "" {
		return ""
	}

	backendExpectsTLS := appProto == "https"
	backendExpectsHTTP := appProto == "http" || appProto == "kubernetes.io/h2c"

	routeSendsHTTP := route.Spec.TLS == nil ||
		route.Spec.TLS.Termination == routev1.TLSTerminationEdge
	routeSendsTLS := route.Spec.TLS != nil &&
		(route.Spec.TLS.Termination == routev1.TLSTerminationPassthrough ||
			route.Spec.TLS.Termination == routev1.TLSTerminationReencrypt)

	termination := "none"
	if route.Spec.TLS != nil {
		termination = string(route.Spec.TLS.Termination)
	}

	if routeSendsHTTP && backendExpectsTLS {
		return fmt.Sprintf("termination %q sends plain HTTP to backend, "+
			"but Service %s/%s declares appProtocol %q",
			termination, svc.Namespace, svc.Name, appProto)
	}

	if routeSendsTLS && backendExpectsHTTP {
		return fmt.Sprintf("termination %q sends TLS to backend, "+
			"but Service %s/%s declares appProtocol %q",
			termination, svc.Namespace, svc.Name, appProto)
	}

	return ""
}

// resolveAppProtocol returns the appProtocol of the Service port targeted by
// the Route. If the Route specifies a target port, it matches by name or
// number. Otherwise it uses the first port. Returns empty string if the
// port has no appProtocol set or cannot be resolved.
func resolveAppProtocol(route *routev1.Route, svc *corev1.Service) string {
	if len(svc.Spec.Ports) == 0 {
		return ""
	}

	if route.Spec.Port == nil {
		return ptr.Deref(svc.Spec.Ports[0].AppProtocol, "")
	}

	if p, ok := findServicePort(route.Spec.Port.TargetPort, svc); ok {
		return ptr.Deref(p.AppProtocol, "")
	}

	return ""
}

// findServicePort matches a Route target port against a Service's ports by
// name or number. For string-typed targets (e.g. "https" or "443"), it tries
// name match first, then numeric match if the string parses to a valid port.
func findServicePort(tp intstr.IntOrString, svc *corev1.Service) (corev1.ServicePort, bool) {
	name, num := targetPortNameAndNumber(tp)

	for _, p := range svc.Spec.Ports {
		if name != "" && p.Name == name {
			return p, true
		}
		if num > 0 && (p.Port == num || p.TargetPort.IntValue() == int(num)) {
			return p, true
		}
	}

	return corev1.ServicePort{}, false
}

func targetPortNameAndNumber(tp intstr.IntOrString) (string, int32) {
	if tp.Type == intstr.Int {
		return "", tp.IntVal
	}

	if n := tp.IntValue(); n > 0 && n <= 65535 {
		return "", int32(n)
	}

	return tp.StrVal, 0
}

// routeURL builds a URL from a Route's host, deriving the scheme from its TLS config.
func routeURL(route *routev1.Route, host string) *apis.URL {
	if route.Spec.TLS == nil {
		return apis.HTTP(host)
	}

	return apis.HTTPS(host)
}

// admittedHost returns the host of an admitted Route, or empty string if not admitted.
func admittedHost(route *routev1.Route) string {
	for _, ingress := range route.Status.Ingress {
		for _, cond := range ingress.Conditions {
			if cond.Type == routev1.RouteAdmitted && cond.Status == corev1.ConditionTrue {
				return ingress.Host
			}
		}
	}

	return ""
}

// gatewaysWithoutExternalURLs returns gateways that have no external URLs
// in their discovered addresses, paired with the unique paths for each.
func gatewaysWithoutExternalURLs(discovered []DiscoveredURL) []gatewayWithPaths {
	type gwKey struct{ ns, name string }
	pathsByGW := make(map[gwKey]map[string]bool)
	hasExternal := make(map[gwKey]bool)
	orderSeen := make(map[gwKey]gwapiv1.ObjectReference)

	for _, d := range discovered {
		if d.Origin == nil || d.Origin.Kind != "Gateway" {
			continue
		}

		key := gwKey{
			ns:   string(ptr.Deref(d.Origin.Namespace, "")),
			name: string(d.Origin.Name),
		}

		if _, ok := orderSeen[key]; !ok {
			orderSeen[key] = *d.Origin
		}

		if IsExternalURL(d.URL) {
			hasExternal[key] = true
		}

		if pathsByGW[key] == nil {
			pathsByGW[key] = make(map[string]bool)
		}
		pathsByGW[key][d.URL.Path] = true
	}

	var result []gatewayWithPaths
	for key, ref := range orderSeen {
		if hasExternal[key] {
			continue
		}

		paths := make([]string, 0, len(pathsByGW[key]))
		for p := range pathsByGW[key] {
			paths = append(paths, p)
		}
		slices.Sort(paths)

		result = append(result, gatewayWithPaths{ref: ref, paths: paths})
	}

	slices.SortFunc(result, func(a, b gatewayWithPaths) int {
		if c := cmp.Compare(string(ptr.Deref(a.ref.Namespace, "")), string(ptr.Deref(b.ref.Namespace, ""))); c != 0 {
			return c
		}

		return cmp.Compare(string(a.ref.Name), string(b.ref.Name))
	})

	return result
}
