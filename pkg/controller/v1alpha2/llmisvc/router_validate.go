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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const httpRouteValidateNamePattern = "-kserve-route-validate-%d"

// NewInlineRouteDryRun returns the validating webhook's inline route check:
// the rules are split and packed into HTTPRoutes that each fit the Gateway
// API limits, and every one is sent to the API server as a dry-run create,
// so the Gateway API CRD validates the nested rules the inline schema leaves
// out. It runs on kubectl apply and, through the rendered spec dry-run in
// combineBaseRefsConfig, on presets, so a bad rule fails before the workload
// rolls. Precedence grouping does not matter for validation, so the rules
// are packed in list order.
func NewInlineRouteDryRun(c client.Client) v1alpha2.InlineRouteDryRunFunc {
	return func(ctx context.Context, namespace, name string, spec *gwapiv1.HTTPRouteSpec) error {
		routes, sources := packInlineRouteForValidation(namespace, name, spec)
		for i, route := range routes {
			if err := c.Create(ctx, route, client.DryRunAll); err != nil {
				if meta.IsNoMatchError(err) {
					return nil // Gateway API is not installed; nothing to validate against.
				}
				return fmt.Errorf("%s: %w", describeSourceRange(sources[i]), apiServerVerdict(err))
			}
		}
		return nil
	}
}

// packInlineRouteForValidation packs the rules of spec in list order into
// HTTPRoutes within the Gateway API rule and match limits, keeping the
// chunks of one named rule apart, and returns the source rule indexes of
// each route.
func packInlineRouteForValidation(namespace, name string, spec *gwapiv1.HTTPRouteSpec) ([]*gwapiv1.HTTPRoute, [][]int) {
	rules, origins := splitOversizedRules(spec.Rules)
	var groups [][]int
	var current []int
	matches, names := 0, map[int]struct{}{}
	for i := range rules {
		n := ruleMatchCount(&rules[i])
		_, sameName := names[origins[i]]
		sameName = sameName && rules[i].Name != nil
		if len(current) > 0 && (sameName || len(current) == httpRouteMaxRules || matches+n > httpRouteMaxTotalMatches) {
			groups = append(groups, current)
			current, matches, names = nil, 0, map[int]struct{}{}
		}
		current = append(current, i)
		matches += n
		names[origins[i]] = struct{}{}
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}

	routes := make([]*gwapiv1.HTTPRoute, 0, len(groups))
	sources := make([][]int, 0, len(groups))
	for g, group := range groups {
		route := &gwapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kmeta.ChildName(name, fmt.Sprintf(httpRouteValidateNamePattern, g)),
				Namespace: namespace,
			},
			Spec: *spec.DeepCopy(),
		}
		route.Spec.Rules = make([]gwapiv1.HTTPRouteRule, 0, len(group))
		for _, i := range group {
			route.Spec.Rules = append(route.Spec.Rules, *rules[i].DeepCopy())
		}
		routes = append(routes, route)
		sources = append(sources, sourceIndexes(group, origins))
	}
	return routes, sources
}
