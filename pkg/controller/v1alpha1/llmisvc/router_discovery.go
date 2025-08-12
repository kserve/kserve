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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type resolvedGateway struct {
	gateway      *gwapiv1.Gateway
	gatewayClass *gwapiv1.GatewayClass
	parentRef    gwapiv1.ParentReference
}

// e.g. given a route with ParentRefs [{Name: "gw-a"}, {Name: "gw-b"}], it will return two entries
// resolving Gateways "gw-a" and "gw-b" (in their respective namespaces or the route's namespace),
// along with each Gateway's GatewayClass, preserving the original ParentReference.
func discoverGateways(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute) ([]resolvedGateway, error) {
	gateways := make([]resolvedGateway, 0)
	for _, parentRef := range route.Spec.ParentRefs {
		ns := ptr.Deref(parentRef.Namespace, gwapiv1.Namespace(route.Namespace))
		gwNS, gwName := string(ns), string(parentRef.Name)

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

// e.g. with route hostnames ["api.example.com"], listener HTTPS:443, path "/v1", and Gateway address "1.2.3.4",
// it will return ["https://api.example.com/v1"]; if no hostnames, it will fall back to ["https://1.2.3.4/v1"].
func DiscoverURLs(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute) ([]*apis.URL, error) {
	var urls []*apis.URL

	gateways, err := discoverGateways(ctx, c, route)
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

// e.g. if route.Spec.Rules[0].Matches[0].Path.Value = "/chat", it will return "/chat"; if unset, it will return "/".
func extractRoutePath(route *gwapiv1.HTTPRoute) string {
	if len(route.Spec.Rules) > 0 && len(route.Spec.Rules[0].Matches) > 0 {
		// TODO how do we deal with regexp
		return ptr.Deref(route.Spec.Rules[0].Matches[0].Path.Value, "/")
	}
	return "/"
}

// Will return the first listener in the gateway that matches given a section name
// e.g. if sectionName is "kserve-route", it will return the first listener that matches "kserve-route"
func selectListener(gateway *gwapiv1.Gateway, sectionName *gwapiv1.SectionName) *gwapiv1.Listener {
	if sectionName != nil {
		for _, listener := range gateway.Spec.Listeners {
			if listener.Name == *sectionName {
				return &listener
			}
		}
	}

	return &gateway.Spec.Listeners[0]
}

// e.g. if listener.Protocol is "HTTPS", it will return "https", defaulting to "http"
func extractSchemeFromListener(listener *gwapiv1.Listener) string {
	if listener.Protocol == gwapiv1.HTTPSProtocolType {
		return "https"
	}
	return "http"
}

// e.g. if route.Spec.Hostnames is ["foo.com", "bar.com"], it will return ["foo.com", "bar.com"]
func extractRouteHostnames(route *gwapiv1.HTTPRoute) []string {
	var hostnames []string
	for _, h := range route.Spec.Hostnames {
		host := string(h)
		if host != "" && host != "*" {
			hostnames = append(hostnames, host)
		}
	}
	return hostnames
}

// e.g. if addresses is [{"Value": "192.168.1.1"}, {"Value": "192.168.1.2"}], it will return ["192.168.1.1", "192.168.1.2"]
func extractAddressValues(addresses []gwapiv1.GatewayStatusAddress) []string {
	var values []string
	for _, addr := range addresses {
		if addr.Value != "" {
			values = append(values, addr.Value)
		}
	}
	return values
}

// e.g. hostnames ["a.com","b.com"], scheme "http", port 80, path "/p" will return ["http://a.com/p","http://b.com/p"].
// e.g. hostnames ["a.com"], scheme "https", port 8443, path "/" will return ["https://a.com:8443/"].
func combineIntoURLs(hostnames []string, scheme string, port gwapiv1.PortNumber, path string) ([]*apis.URL, error) {
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

// e.g. ("a.com", 8080) will return "a.com:8080"; ("a.com", nil) or ("a.com", 0) will return "a.com".
func joinHostPort(host string, port *gwapiv1.PortNumber) string {
	if port != nil && *port != 0 {
		return net.JoinHostPort(host, fmt.Sprint(*port))
	}
	return host
}

type ExternalAddressNotFoundError struct {
	GatewayNamespace string
	GatewayName      string
}

// e.g. (&ExternalAddressNotFoundError{GatewayNamespace: "ns", GatewayName: "gw"}).Error()
// will return "Gateway ns/gw has no external address found".
func (e *ExternalAddressNotFoundError) Error() string {
	return fmt.Sprintf("Gateway %s/%s has no external address found", e.GatewayNamespace, e.GatewayName)
}

// e.g. if err is of type *ExternalAddressNotFoundError, it will return nil; otherwise it will return err unchanged.
func IgnoreExternalAddressNotFound(err error) error {
	if IsExternalAddressNotFound(err) {
		return nil
	}
	return err
}

// e.g. will return true for &ExternalAddressNotFoundError{...}; will return false for other error types or nil.
func IsExternalAddressNotFound(err error) bool {
	var externalAddrNotFoundErr *ExternalAddressNotFoundError
	return errors.As(err, &externalAddrNotFoundErr)
}
