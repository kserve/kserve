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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ObjectOption[T client.Object] func(T)

type GatewayOption ObjectOption[*gwapiv1.Gateway]

func Gateway(name string, opts ...GatewayOption) *gwapiv1.Gateway {
	gw := &gwapiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: gwapiv1.GatewaySpec{
			Listeners:      []gwapiv1.Listener{},
			Infrastructure: &gwapiv1.GatewayInfrastructure{},
		},
		Status: gwapiv1.GatewayStatus{
			Addresses: []gwapiv1.GatewayStatusAddress{},
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
	return func(gw *gwapiv1.Gateway) {
		gw.Spec.GatewayClassName = gwapiv1.ObjectName(className)
	}
}

func WithInfrastructureLabels(key, value string) GatewayOption {
	return func(gw *gwapiv1.Gateway) {
		if gw.Spec.Infrastructure.Labels == nil {
			gw.Spec.Infrastructure.Labels = make(map[gwapiv1.LabelKey]gwapiv1.LabelValue)
		}
		gw.Spec.Infrastructure.Labels[gwapiv1.LabelKey(key)] = gwapiv1.LabelValue(value)
	}
}

func WithListener(protocol gwapiv1.ProtocolType) GatewayOption {
	return func(gw *gwapiv1.Gateway) {
		port := gwapiv1.PortNumber(80)
		if protocol == gwapiv1.HTTPSProtocolType {
			port = 443
		}
		listener := gwapiv1.Listener{
			Name:     gwapiv1.SectionName("listener"),
			Protocol: protocol,
			Port:     port,
		}
		gw.Spec.Listeners = append(gw.Spec.Listeners, listener)
	}
}

func WithListeners(listeners ...gwapiv1.Listener) GatewayOption {
	return func(gw *gwapiv1.Gateway) {
		gw.Spec.Listeners = append(gw.Spec.Listeners, listeners...)
	}
}

type (
	HTTPRouteOption ObjectOption[*gwapiv1.HTTPRoute]
	ParentRefOption func(*gwapiv1.ParentReference)
)

func HTTPRoute(name string, opts ...HTTPRouteOption) *gwapiv1.HTTPRoute {
	route := &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{},
			},
			Hostnames: []gwapiv1.Hostname{},
			Rules:     []gwapiv1.HTTPRouteRule{},
		},
	}

	for _, opt := range opts {
		opt(route)
	}

	return route
}

func WithParentRef(ref gwapiv1.ParentReference) HTTPRouteOption {
	return func(route *gwapiv1.HTTPRoute) {
		route.Spec.CommonRouteSpec.ParentRefs = append(route.Spec.CommonRouteSpec.ParentRefs, ref)
	}
}

func WithParentRefs(refs ...gwapiv1.ParentReference) HTTPRouteOption {
	return func(route *gwapiv1.HTTPRoute) {
		route.Spec.CommonRouteSpec.ParentRefs = refs
	}
}

func WithHostnames(hostnames ...string) HTTPRouteOption {
	return func(route *gwapiv1.HTTPRoute) {
		route.Spec.Hostnames = make([]gwapiv1.Hostname, len(hostnames))
		for i, hostname := range hostnames {
			route.Spec.Hostnames[i] = gwapiv1.Hostname(hostname)
		}
	}
}

func WithAddresses(addresses ...string) GatewayOption {
	return func(gw *gwapiv1.Gateway) {
		gw.Status.Addresses = make([]gwapiv1.GatewayStatusAddress, len(addresses))
		for i, address := range addresses {
			gw.Status.Addresses[i] = gwapiv1.GatewayStatusAddress{
				Value: address,
				// Type is left as nil (defaults to IPAddressType behavior)
			}
		}
	}
}

func WithHostnameAddresses(addresses ...string) GatewayOption {
	return func(gw *gwapiv1.Gateway) {
		gw.Status.Addresses = make([]gwapiv1.GatewayStatusAddress, len(addresses))
		for i, address := range addresses {
			gw.Status.Addresses[i] = gwapiv1.GatewayStatusAddress{
				Type:  ptr.To(gwapiv1.HostnameAddressType),
				Value: address,
			}
		}
	}
}

func WithMixedAddresses(addresses ...gwapiv1.GatewayStatusAddress) GatewayOption {
	return func(gw *gwapiv1.Gateway) {
		gw.Status.Addresses = addresses
	}
}

func IPAddress(value string) gwapiv1.GatewayStatusAddress {
	return gwapiv1.GatewayStatusAddress{
		Type:  ptr.To(gwapiv1.IPAddressType),
		Value: value,
	}
}

func HostnameAddress(value string) gwapiv1.GatewayStatusAddress {
	return gwapiv1.GatewayStatusAddress{
		Type:  ptr.To(gwapiv1.HostnameAddressType),
		Value: value,
	}
}

func WithPath(path string) HTTPRouteOption {
	return func(route *gwapiv1.HTTPRoute) {
		rule := gwapiv1.HTTPRouteRule{
			Matches: []gwapiv1.HTTPRouteMatch{
				{
					Path: &gwapiv1.HTTPPathMatch{
						Value: ptr.To(path),
					},
				},
			},
		}
		route.Spec.Rules = append(route.Spec.Rules, rule)
	}
}

func WithRules(rules ...gwapiv1.HTTPRouteRule) HTTPRouteOption {
	return func(route *gwapiv1.HTTPRoute) {
		route.Spec.Rules = rules
	}
}

func GatewayRef(name string, opts ...ParentRefOption) gwapiv1.ParentReference {
	ref := gwapiv1.ParentReference{
		Name:  gwapiv1.ObjectName(name),
		Group: ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
		Kind:  ptr.To(gwapiv1.Kind("Gateway")),
	}
	for _, opt := range opts {
		opt(&ref)
	}
	return ref
}

func RefInNamespace(namespace string) ParentRefOption {
	return func(ref *gwapiv1.ParentReference) {
		ref.Namespace = ptr.To(gwapiv1.Namespace(namespace))
	}
}

func GatewayRefWithoutNamespace(name string) gwapiv1.ParentReference {
	return gwapiv1.ParentReference{
		Name:  gwapiv1.ObjectName(name),
		Group: ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
		Kind:  ptr.To(gwapiv1.Kind("Gateway")),
		// Namespace intentionally omitted
	}
}

func HTTPSGateway(name, namespace string, addresses ...string) *gwapiv1.Gateway {
	return Gateway(name,
		InNamespace[*gwapiv1.Gateway](namespace),
		WithListener(gwapiv1.HTTPSProtocolType),
		WithAddresses(addresses...),
	)
}

func HTTPGateway(name, namespace string, addresses ...string) *gwapiv1.Gateway {
	return Gateway(name,
		InNamespace[*gwapiv1.Gateway](namespace),
		WithListener(gwapiv1.HTTPProtocolType),
		WithAddresses(addresses...),
	)
}

type (
	HTTPRouteRuleOption  func(*gwapiv1.HTTPRouteRule)
	HTTPBackendRefOption func(*gwapiv1.HTTPBackendRef)
)

func WithHTTPRouteRule(rule gwapiv1.HTTPRouteRule) HTTPRouteOption {
	return func(route *gwapiv1.HTTPRoute) {
		route.Spec.Rules = append(route.Spec.Rules, rule)
	}
}

func HTTPRouteRule(opts ...HTTPRouteRuleOption) gwapiv1.HTTPRouteRule {
	rule := gwapiv1.HTTPRouteRule{
		Matches:     []gwapiv1.HTTPRouteMatch{},
		BackendRefs: []gwapiv1.HTTPBackendRef{},
	}

	for _, opt := range opts {
		opt(&rule)
	}

	return rule
}

func WithMatches(matches ...gwapiv1.HTTPRouteMatch) HTTPRouteRuleOption {
	return func(rule *gwapiv1.HTTPRouteRule) {
		rule.Matches = append(rule.Matches, matches...)
	}
}

func WithBackendRefs(refs ...gwapiv1.HTTPBackendRef) HTTPRouteRuleOption {
	return func(rule *gwapiv1.HTTPRouteRule) {
		rule.BackendRefs = append(rule.BackendRefs, refs...)
	}
}

func WithTimeouts(backendTimeout, requestTimeout string) HTTPRouteRuleOption {
	return func(rule *gwapiv1.HTTPRouteRule) {
		rule.Timeouts = &gwapiv1.HTTPRouteTimeouts{
			BackendRequest: ptr.To(gwapiv1.Duration(backendTimeout)),
			Request:        ptr.To(gwapiv1.Duration(requestTimeout)),
		}
	}
}

func WithFilters(filters ...gwapiv1.HTTPRouteFilter) HTTPRouteRuleOption {
	return func(rule *gwapiv1.HTTPRouteRule) {
		rule.Filters = append(rule.Filters, filters...)
	}
}

func WithHTTPRule(ruleOpts ...HTTPRouteRuleOption) HTTPRouteOption {
	return WithHTTPRouteRule(HTTPRouteRule(ruleOpts...))
}

func Matches(matches ...gwapiv1.HTTPRouteMatch) HTTPRouteRuleOption {
	return WithMatches(matches...)
}

func BackendRefs(refs ...gwapiv1.HTTPBackendRef) HTTPRouteRuleOption {
	return WithBackendRefs(refs...)
}

func Timeouts(backendTimeout, requestTimeout string) HTTPRouteRuleOption {
	return WithTimeouts(backendTimeout, requestTimeout)
}

func Filters(filters ...gwapiv1.HTTPRouteFilter) HTTPRouteRuleOption {
	return WithFilters(filters...)
}

func PathPrefixMatch(path string) gwapiv1.HTTPRouteMatch {
	return gwapiv1.HTTPRouteMatch{
		Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
			Value: ptr.To(path),
		},
	}
}

func ServiceRef(name string, port int32, weight int32) gwapiv1.HTTPBackendRef {
	return gwapiv1.HTTPBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Kind: ptr.To(gwapiv1.Kind("Service")),
				Name: gwapiv1.ObjectName(name),
				Port: ptr.To(gwapiv1.PortNumber(port)),
			},
			Weight: ptr.To(weight),
		},
	}
}

func HTTPRouteRuleWithBackendAndTimeouts(backendName string, backendPort int32, path string, backendTimeout, requestTimeout string) gwapiv1.HTTPRouteRule {
	return gwapiv1.HTTPRouteRule{
		BackendRefs: []gwapiv1.HTTPBackendRef{
			ServiceRef(backendName, backendPort, 1),
		},
		Matches: []gwapiv1.HTTPRouteMatch{
			PathPrefixMatch(path),
		},
		Timeouts: &gwapiv1.HTTPRouteTimeouts{
			BackendRequest: ptr.To(gwapiv1.Duration(backendTimeout)),
			Request:        ptr.To(gwapiv1.Duration(requestTimeout)),
		},
	}
}

func GatewayParentRef(name, namespace string) gwapiv1.ParentReference {
	return gwapiv1.ParentReference{
		Group:     ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
		Kind:      ptr.To(gwapiv1.Kind("Gateway")),
		Name:      gwapiv1.ObjectName(name),
		Namespace: ptr.To(gwapiv1.Namespace(namespace)),
	}
}
