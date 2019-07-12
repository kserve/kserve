/*
Copyright 2018 The Knative Authors

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

package resources

import (
	"fmt"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"

	istiov1alpha1 "github.com/knative/pkg/apis/istio/common/v1alpha1"
	"github.com/knative/pkg/apis/istio/v1alpha3"
	"github.com/knative/pkg/kmeta"
	"github.com/knative/pkg/system"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/clusteringress/resources/names"
)

// MakeVirtualService creates an Istio VirtualService as network programming.
// Such VirtualService specifies which Gateways and Hosts that it applies to,
// as well as the routing rules.
func MakeVirtualService(ci *v1alpha1.ClusterIngress, gateways []string) *v1alpha3.VirtualService {
	vs := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.VirtualService(ci),
			Namespace:       system.Namespace(),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ci)},
			Annotations:     ci.ObjectMeta.Annotations,
		},
		Spec: *makeVirtualServiceSpec(ci, gateways),
	}

	// Populate the ClusterIngress labels.
	if vs.Labels == nil {
		vs.Labels = make(map[string]string)
	}
	vs.Labels[networking.IngressLabelKey] = ci.Name

	ingressLabels := ci.Labels
	vs.Labels[serving.RouteLabelKey] = ingressLabels[serving.RouteLabelKey]
	vs.Labels[serving.RouteNamespaceLabelKey] = ingressLabels[serving.RouteNamespaceLabelKey]

	return vs
}

func makeVirtualServiceSpec(ci *v1alpha1.ClusterIngress, gateways []string) *v1alpha3.VirtualServiceSpec {
	spec := v1alpha3.VirtualServiceSpec{
		// We want to connect to two Gateways: the Knative shared
		// Gateway, and the 'mesh' Gateway.  The former provides
		// access from outside of the cluster, and the latter provides
		// access for services from inside the cluster.
		Gateways: append(gateways, "mesh"),
		Hosts:    getHosts(ci),
	}

	for _, rule := range ci.Spec.Rules {
		hosts := rule.Hosts
		for _, p := range rule.HTTP.Paths {
			spec.HTTP = append(spec.HTTP, *makeVirtualServiceRoute(hosts, &p))
		}
	}
	return &spec
}

func makePortSelector(ios intstr.IntOrString) v1alpha3.PortSelector {
	if ios.Type == intstr.Int {
		return v1alpha3.PortSelector{
			Number: uint32(ios.IntValue()),
		}
	}
	return v1alpha3.PortSelector{
		Name: ios.String(),
	}
}

func makeVirtualServiceRoute(hosts []string, http *v1alpha1.HTTPClusterIngressPath) *v1alpha3.HTTPRoute {
	matches := []v1alpha3.HTTPMatchRequest{}
	for _, host := range expandedHosts(hosts) {
		matches = append(matches, makeMatch(host, http.Path))
	}
	weights := []v1alpha3.HTTPRouteDestination{}
	for _, split := range http.Splits {

		var h *v1alpha3.Headers
		// TODO(mattmoor): Switch to Headers when we can have a hard
		// dependency on 1.1, but 1.0.x rejects the unknown fields.
		// if len(split.AppendHeaders) > 0 {
		// 	h = &v1alpha3.Headers{
		// 		Request: &v1alpha3.HeaderOperations{
		// 			Add: split.AppendHeaders,
		// 		},
		// 	}
		// }

		weights = append(weights, v1alpha3.HTTPRouteDestination{
			Destination: v1alpha3.Destination{
				Host: network.GetServiceHostname(
					split.ServiceName, split.ServiceNamespace),
				Port: makePortSelector(split.ServicePort),
			},
			Weight:  split.Percent,
			Headers: h,
		})
	}

	var h *v1alpha3.Headers
	// TODO(mattmoor): Switch to Headers when we can have a hard
	// dependency on 1.1, but 1.0.x rejects the unknown fields.
	// if len(http.AppendHeaders) > 0 {
	// 	h = &v1alpha3.Headers{
	// 		Request: &v1alpha3.HeaderOperations{
	// 			Add: http.AppendHeaders,
	// 		},
	// 	}
	// }

	return &v1alpha3.HTTPRoute{
		Match:   matches,
		Route:   weights,
		Timeout: http.Timeout.Duration.String(),
		Retries: &v1alpha3.HTTPRetry{
			Attempts:      http.Retries.Attempts,
			PerTryTimeout: http.Retries.PerTryTimeout.Duration.String(),
		},
		// TODO(mattmoor): Remove AppendHeaders when 1.1 is a hard dependency.
		// AppendHeaders is deprecated in Istio 1.1 in favor of Headers,
		// however, 1.0.x doesn't support Headers.
		DeprecatedAppendHeaders: http.AppendHeaders,
		Headers:                 h,
		WebsocketUpgrade:        true,
	}
}

func dedup(hosts []string) []string {
	return sets.NewString(hosts...).List()
}

func expandedHosts(hosts []string) []string {
	expanded := []string{}
	allowedSuffixes := []string{
		"",
		"." + network.GetClusterDomainName(),
		".svc." + network.GetClusterDomainName(),
	}
	for _, h := range hosts {
		for _, suffix := range allowedSuffixes {
			if strings.HasSuffix(h, suffix) {
				expanded = append(expanded, strings.TrimSuffix(h, suffix))
			}
		}
	}
	return dedup(expanded)
}

func makeMatch(host string, pathRegExp string) v1alpha3.HTTPMatchRequest {
	match := v1alpha3.HTTPMatchRequest{
		Authority: &istiov1alpha1.StringMatch{
			Regex: hostRegExp(host),
		},
	}
	// Empty pathRegExp is considered match all path. We only need to
	// consider pathRegExp when it's non-empty.
	if pathRegExp != "" {
		match.URI = &istiov1alpha1.StringMatch{
			Regex: pathRegExp,
		}
	}
	return match
}

// Should only match 1..65535, but for simplicity it matches 0-99999.
const portMatch = `(?::\d{1,5})?`

// hostRegExp returns an ECMAScript regular expression to match either host or host:<any port>
func hostRegExp(host string) string {
	return fmt.Sprintf("^%s%s$", regexp.QuoteMeta(host), portMatch)
}

func getHosts(ci *v1alpha1.ClusterIngress) []string {
	hosts := make([]string, 0, len(ci.Spec.Rules))
	for _, rule := range ci.Spec.Rules {
		hosts = append(hosts, rule.Hosts...)
	}
	return dedup(hosts)
}
