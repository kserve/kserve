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

package fixture

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ObjectOption[T client.Object] func(T)

type GatewayOption ObjectOption[*gatewayapiv1.Gateway]

func Gateway(name string, opts ...GatewayOption) *gatewayapiv1.Gateway {
	gw := &gatewayapiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: gatewayapiv1.GatewaySpec{
			GatewayClassName: defaultGatewayClass,
			Listeners:        []gatewayapiv1.Listener{},
			Infrastructure:   &gatewayapiv1.GatewayInfrastructure{},
		},
		Status: gatewayapiv1.GatewayStatus{
			Addresses: []gatewayapiv1.GatewayStatusAddress{},
		},
	}

	for _, opt := range opts {
		opt(gw)
	}

	return gw
}

func InNamespace[T metav1.Object](namespace string) func(T) {
	return func(t T) {
		t.SetNamespace(namespace)
	}
}

func WithClassName(className string) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		gw.Spec.GatewayClassName = gatewayapiv1.ObjectName(className)
	}
}

func WithInfrastructureLabels(key, value string) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		if gw.Spec.Infrastructure.Labels == nil {
			gw.Spec.Infrastructure.Labels = make(map[gatewayapiv1.LabelKey]gatewayapiv1.LabelValue)
		}
		gw.Spec.Infrastructure.Labels[gatewayapiv1.LabelKey(key)] = gatewayapiv1.LabelValue(value)
	}
}

func WithListener(protocol gatewayapiv1.ProtocolType) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		port := gatewayapiv1.PortNumber(80)
		if protocol == gatewayapiv1.HTTPSProtocolType {
			port = 443
		}
		listener := gatewayapiv1.Listener{
			Name:     gatewayapiv1.SectionName("listener"),
			Protocol: protocol,
			Port:     port,
		}
		gw.Spec.Listeners = append(gw.Spec.Listeners, listener)
	}
}

func WithListeners(listeners ...gatewayapiv1.Listener) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		gw.Spec.Listeners = append(gw.Spec.Listeners, listeners...)
	}
}

type (
	HTTPRouteOption ObjectOption[*gatewayapiv1.HTTPRoute]
	ParentRefOption func(*gatewayapiv1.ParentReference)
)

func HTTPRoute(name string, opts ...HTTPRouteOption) *gatewayapiv1.HTTPRoute {
	route := &gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{},
			},
			Hostnames: []gatewayapiv1.Hostname{},
			Rules:     []gatewayapiv1.HTTPRouteRule{},
		},
	}

	for _, opt := range opts {
		opt(route)
	}

	return route
}

func WithParentRef(ref gatewayapiv1.ParentReference) HTTPRouteOption {
	return func(route *gatewayapiv1.HTTPRoute) {
		route.Spec.CommonRouteSpec.ParentRefs = append(route.Spec.CommonRouteSpec.ParentRefs, ref)
	}
}

func WithParentRefs(refs ...gatewayapiv1.ParentReference) HTTPRouteOption {
	return func(route *gatewayapiv1.HTTPRoute) {
		route.Spec.CommonRouteSpec.ParentRefs = refs
	}
}

func WithHostnames(hostnames ...string) HTTPRouteOption {
	return func(route *gatewayapiv1.HTTPRoute) {
		route.Spec.Hostnames = make([]gatewayapiv1.Hostname, len(hostnames))
		for i, hostname := range hostnames {
			route.Spec.Hostnames[i] = gatewayapiv1.Hostname(hostname)
		}
	}
}

func WithAddresses(addresses ...string) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		gw.Status.Addresses = make([]gatewayapiv1.GatewayStatusAddress, len(addresses))
		for i, address := range addresses {
			gw.Status.Addresses[i] = gatewayapiv1.GatewayStatusAddress{
				Value: address,
				// Type is left as nil (defaults to IPAddressType behavior)
			}
		}
	}
}

func WithHostnameAddresses(addresses ...string) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		gw.Status.Addresses = make([]gatewayapiv1.GatewayStatusAddress, len(addresses))
		for i, address := range addresses {
			gw.Status.Addresses[i] = gatewayapiv1.GatewayStatusAddress{
				Type:  ptr.To(gatewayapiv1.HostnameAddressType),
				Value: address,
			}
		}
	}
}

func WithMixedAddresses(addresses ...gatewayapiv1.GatewayStatusAddress) GatewayOption {
	return func(gw *gatewayapiv1.Gateway) {
		gw.Status.Addresses = addresses
	}
}

func IPAddress(value string) gatewayapiv1.GatewayStatusAddress {
	return gatewayapiv1.GatewayStatusAddress{
		Type:  ptr.To(gatewayapiv1.IPAddressType),
		Value: value,
	}
}

func HostnameAddress(value string) gatewayapiv1.GatewayStatusAddress {
	return gatewayapiv1.GatewayStatusAddress{
		Type:  ptr.To(gatewayapiv1.HostnameAddressType),
		Value: value,
	}
}

func WithPath(path string) HTTPRouteOption {
	return func(route *gatewayapiv1.HTTPRoute) {
		rule := gatewayapiv1.HTTPRouteRule{
			Matches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Value: ptr.To(path),
					},
				},
			},
		}
		route.Spec.Rules = append(route.Spec.Rules, rule)
	}
}

func WithRules(rules ...gatewayapiv1.HTTPRouteRule) HTTPRouteOption {
	return func(route *gatewayapiv1.HTTPRoute) {
		route.Spec.Rules = rules
	}
}

func GatewayRef(name string, opts ...ParentRefOption) gatewayapiv1.ParentReference {
	ref := gatewayapiv1.ParentReference{
		Name:  gatewayapiv1.ObjectName(name),
		Group: ptr.To(gatewayapiv1.Group("gateway.networking.k8s.io")),
		Kind:  ptr.To(gatewayapiv1.Kind("Gateway")),
	}
	for _, opt := range opts {
		opt(&ref)
	}
	return ref
}

func RefInNamespace(namespace string) ParentRefOption {
	return func(ref *gatewayapiv1.ParentReference) {
		ref.Namespace = ptr.To(gatewayapiv1.Namespace(namespace))
	}
}

func GatewayRefWithoutNamespace(name string) gatewayapiv1.ParentReference {
	return gatewayapiv1.ParentReference{
		Name:  gatewayapiv1.ObjectName(name),
		Group: ptr.To(gatewayapiv1.Group("gateway.networking.k8s.io")),
		Kind:  ptr.To(gatewayapiv1.Kind("Gateway")),
		// Namespace intentionally omitted
	}
}

func HTTPSGateway(name, namespace string, addresses ...string) *gatewayapiv1.Gateway {
	return Gateway(name,
		InNamespace[*gatewayapiv1.Gateway](namespace),
		WithListener(gatewayapiv1.HTTPSProtocolType),
		WithAddresses(addresses...),
	)
}

func HTTPGateway(name, namespace string, addresses ...string) *gatewayapiv1.Gateway {
	return Gateway(name,
		InNamespace[*gatewayapiv1.Gateway](namespace),
		WithListener(gatewayapiv1.HTTPProtocolType),
		WithAddresses(addresses...),
	)
}

type (
	HTTPRouteRuleOption  func(*gatewayapiv1.HTTPRouteRule)
	HTTPBackendRefOption func(*gatewayapiv1.HTTPBackendRef)
)

func WithHTTPRouteRule(rule gatewayapiv1.HTTPRouteRule) HTTPRouteOption {
	return func(route *gatewayapiv1.HTTPRoute) {
		route.Spec.Rules = append(route.Spec.Rules, rule)
	}
}

func HTTPRouteRule(opts ...HTTPRouteRuleOption) gatewayapiv1.HTTPRouteRule {
	rule := gatewayapiv1.HTTPRouteRule{
		Matches:     []gatewayapiv1.HTTPRouteMatch{},
		BackendRefs: []gatewayapiv1.HTTPBackendRef{},
	}

	for _, opt := range opts {
		opt(&rule)
	}

	return rule
}

func WithMatches(matches ...gatewayapiv1.HTTPRouteMatch) HTTPRouteRuleOption {
	return func(rule *gatewayapiv1.HTTPRouteRule) {
		rule.Matches = append(rule.Matches, matches...)
	}
}

func WithBackendRefs(refs ...gatewayapiv1.HTTPBackendRef) HTTPRouteRuleOption {
	return func(rule *gatewayapiv1.HTTPRouteRule) {
		rule.BackendRefs = append(rule.BackendRefs, refs...)
	}
}

func WithTimeouts(backendTimeout, requestTimeout string) HTTPRouteRuleOption {
	return func(rule *gatewayapiv1.HTTPRouteRule) {
		rule.Timeouts = &gatewayapiv1.HTTPRouteTimeouts{
			BackendRequest: ptr.To(gatewayapiv1.Duration(backendTimeout)),
			Request:        ptr.To(gatewayapiv1.Duration(requestTimeout)),
		}
	}
}

func WithFilters(filters ...gatewayapiv1.HTTPRouteFilter) HTTPRouteRuleOption {
	return func(rule *gatewayapiv1.HTTPRouteRule) {
		rule.Filters = append(rule.Filters, filters...)
	}
}

func WithHTTPRule(ruleOpts ...HTTPRouteRuleOption) HTTPRouteOption {
	return WithHTTPRouteRule(HTTPRouteRule(ruleOpts...))
}

func Matches(matches ...gatewayapiv1.HTTPRouteMatch) HTTPRouteRuleOption {
	return WithMatches(matches...)
}

func BackendRefs(refs ...gatewayapiv1.HTTPBackendRef) HTTPRouteRuleOption {
	return WithBackendRefs(refs...)
}

func Timeouts(backendTimeout, requestTimeout string) HTTPRouteRuleOption {
	return WithTimeouts(backendTimeout, requestTimeout)
}

func Filters(filters ...gatewayapiv1.HTTPRouteFilter) HTTPRouteRuleOption {
	return WithFilters(filters...)
}

func PathPrefixMatch(path string) gatewayapiv1.HTTPRouteMatch {
	return gatewayapiv1.HTTPRouteMatch{
		Path: &gatewayapiv1.HTTPPathMatch{
			Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
			Value: ptr.To(path),
		},
	}
}

func ServiceRef(name string, port int32, weight int32) gatewayapiv1.HTTPBackendRef {
	return gatewayapiv1.HTTPBackendRef{
		BackendRef: gatewayapiv1.BackendRef{
			BackendObjectReference: gatewayapiv1.BackendObjectReference{
				Kind: ptr.To(gatewayapiv1.Kind("Service")),
				Name: gatewayapiv1.ObjectName(name),
				Port: ptr.To(gatewayapiv1.PortNumber(port)),
			},
			Weight: ptr.To(weight),
		},
	}
}

func HTTPRouteRuleWithBackendAndTimeouts(backendName string, backendPort int32, path string, backendTimeout, requestTimeout string) gatewayapiv1.HTTPRouteRule {
	return gatewayapiv1.HTTPRouteRule{
		BackendRefs: []gatewayapiv1.HTTPBackendRef{
			ServiceRef(backendName, backendPort, 1),
		},
		Matches: []gatewayapiv1.HTTPRouteMatch{
			PathPrefixMatch(path),
		},
		Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
			BackendRequest: ptr.To(gatewayapiv1.Duration(backendTimeout)),
			Request:        ptr.To(gatewayapiv1.Duration(requestTimeout)),
		},
	}
}

func GatewayParentRef(name, namespace string) gatewayapiv1.ParentReference {
	return gatewayapiv1.ParentReference{
		Group:     ptr.To(gatewayapiv1.Group("gateway.networking.k8s.io")),
		Kind:      ptr.To(gatewayapiv1.Kind("Gateway")),
		Name:      gatewayapiv1.ObjectName(name),
		Namespace: ptr.To(gatewayapiv1.Namespace(namespace)),
	}
}