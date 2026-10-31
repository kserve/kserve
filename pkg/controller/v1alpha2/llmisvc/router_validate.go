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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// NewInlineRouteDryRun returns the validating webhook's inline route check:
// the rules are split and packed into HTTPRoutes that each fit the Gateway
// API limits, and every one is sent to the API server as a dry-run, so the
// Gateway API CRD validates the nested rules the inline schema leaves out.
// It runs on kubectl apply and, through the rendered spec dry-run in
// combineBaseRefsConfig, on presets, so a bad rule fails before the workload
// rolls. Precedence grouping does not matter for validation, so the rules
// are packed in list order.
//
// Each packed route takes the name of the managed shard at its position.
// When that shard exists it is dry-run updated, otherwise dry-run created,
// so an update of a service never needs spare HTTPRoute quota. A create that
// only fails on quota is skipped: the quota says nothing about the rules,
// and the controller reports it when it writes the shard.
//
// A rejection of the rules is returned as *v1alpha2.InlineRouteRejectedError;
// any other error means the rules could not be checked and is worth a retry.
func NewInlineRouteDryRun(c client.Client) v1alpha2.InlineRouteDryRunFunc {
	return func(ctx context.Context, namespace, name string, spec *gwapiv1.HTTPRouteSpec) error {
		routes, sources := packInlineRouteForValidation(namespace, name, spec)
		for i, route := range routes {
			err := dryRunInlineRouteShard(ctx, c, route)
			switch {
			case err == nil, isQuotaExceeded(err):
				continue
			case meta.IsNoMatchError(err):
				return nil // Gateway API is not installed; nothing to validate against.
			case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
				return &v1alpha2.InlineRouteRejectedError{
					Err: fmt.Errorf("%s: %w", describeSourceRange(sources[i]), apiServerVerdict(err)),
				}
			default:
				return fmt.Errorf("dry-run of %s: %w", describeSourceRange(sources[i]), err)
			}
		}
		return nil
	}
}

// dryRunInlineRouteShard dry-run updates route over the stored HTTPRoute of
// the same name, or dry-run creates it when there is none.
func dryRunInlineRouteShard(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute) error {
	stored := &gwapiv1.HTTPRoute{}
	err := c.Get(ctx, client.ObjectKeyFromObject(route), stored)
	switch {
	case err == nil:
		stored.Spec = route.Spec
		return c.Update(ctx, stored, client.DryRunAll)
	case apierrors.IsNotFound(err):
		return c.Create(ctx, route, client.DryRunAll)
	default:
		return err
	}
}

// isQuotaExceeded reports a ResourceQuota rejection, which the API server
// returns as Forbidden with "exceeded quota" in the message.
func isQuotaExceeded(err error) bool {
	return apierrors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota")
}

// packInlineRouteForValidation packs the rules of spec in list order into
// HTTPRoutes within the Gateway API rule and match limits, keeping the
// chunks of one named rule apart, and returns the source rule indexes of
// each route.
func packInlineRouteForValidation(namespace, name string, spec *gwapiv1.HTTPRouteSpec) ([]*gwapiv1.HTTPRoute, [][]int) {
	rules, origins := splitOversizedRules(spec.Rules)
	var groups [][]int
	current := make([]int, 0, min(len(rules), httpRouteMaxRules))
	matches, names := 0, map[int]struct{}{}
	for i := range rules {
		n := ruleMatchCount(&rules[i])
		_, sameName := names[origins[i]]
		sameName = sameName && rules[i].Name != nil
		if len(current) > 0 && (sameName || len(current) == httpRouteMaxRules || matches+n > httpRouteMaxTotalMatches) {
			groups = append(groups, current)
			current, matches, names = make([]int, 0, min(len(rules)-i, httpRouteMaxRules)), 0, map[int]struct{}{}
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
				Name:      managedHTTPRouteName(name, g),
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
