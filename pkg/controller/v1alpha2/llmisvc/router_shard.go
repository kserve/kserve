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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// Gateway API per-HTTPRoute limits (v1.5): 16 rules, 64 matches per rule and
// 128 matches across all rules.
const (
	httpRouteMaxRules         = 16
	httpRouteMaxRuleMatches   = 64
	httpRouteMaxTotalMatches  = 128
	httpRouteBaseNameSuffix   = "-kserve-route"
	httpRouteShardNamePattern = httpRouteBaseNameSuffix + "-%d"
)

// managedHTTPRouteName returns the name of the index-th managed HTTPRoute of a
// service. Index 0 is the route name used before sharding existed, so a route
// that fits in one resource keeps its name across upgrades.
func managedHTTPRouteName(llmSvcName string, index int) string {
	if index == 0 {
		return kmeta.ChildName(llmSvcName, httpRouteBaseNameSuffix)
	}
	return kmeta.ChildName(llmSvcName, fmt.Sprintf(httpRouteShardNamePattern, index))
}

// ruleMatchCount is the number of matches a rule contributes to the per-route
// total. A rule without matches is defaulted to a single "/" prefix match.
func ruleMatchCount(rule *gwapiv1.HTTPRouteRule) int {
	return max(1, len(rule.Matches))
}

// splitOversizedRules splits every rule with more matches than a single rule
// may hold into consecutive rules with the same name, filters, backends and
// timeouts. origins[i] is the index of the input rule that out[i] came from.
// Chunks keep the rule name, so shardHTTPRouteRules never puts two chunks of
// a named rule in one HTTPRoute, where rule names must be unique; a
// sectionName policy on the rule then applies in every shard.
func splitOversizedRules(rules []gwapiv1.HTTPRouteRule) (out []gwapiv1.HTTPRouteRule, origins []int) {
	out = make([]gwapiv1.HTTPRouteRule, 0, len(rules))
	origins = make([]int, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		if len(rule.Matches) <= httpRouteMaxRuleMatches {
			out = append(out, rule)
			origins = append(origins, i)
			continue
		}
		for start := 0; start < len(rule.Matches); start += httpRouteMaxRuleMatches {
			end := min(start+httpRouteMaxRuleMatches, len(rule.Matches))
			part := *rule.DeepCopy()
			part.Matches = part.Matches[start:end]
			out = append(out, part)
			origins = append(origins, i)
		}
	}
	return out, origins
}

// checkHTTPRouteShardCapacity rejects rules that cannot fit in limit
// HTTPRoutes however they are packed: more rules, after oversized rules are
// split, than limit*16, or more matches than limit*128. It is cheap, so it
// runs before the pairwise grouping of shardHTTPRouteRules.
func checkHTTPRouteShardCapacity(rules []gwapiv1.HTTPRouteRule, limit int, source string) error {
	split, matches := 0, 0
	for i := range rules {
		n := ruleMatchCount(&rules[i])
		matches += n
		split += (n + httpRouteMaxRuleMatches - 1) / httpRouteMaxRuleMatches
	}
	if split > limit*httpRouteMaxRules {
		return httpRouteShardCapError(fmt.Sprintf("%d rules (at most %d per HTTPRoute)", split, httpRouteMaxRules), limit, source)
	}
	if matches > limit*httpRouteMaxTotalMatches {
		return httpRouteShardCapError(fmt.Sprintf("%d matches (at most %d per HTTPRoute)", matches, httpRouteMaxTotalMatches), limit, source)
	}
	return nil
}

// httpRouteShardCapError reports that the route needs more than limit
// HTTPRoutes, naming both places the cap can be raised.
func httpRouteShardCapError(need string, limit int, source string) error {
	return fmt.Errorf("%w: the route needs %s but %s allows %d HTTPRoutes; "+
		"set the %s annotation on this service (up to %d), ask an admin to raise httpRouteMaxShards in the %s ConfigMap, "+
		"or reduce the rules and LoRA adapters; the stored routes keep serving",
		ErrPreconditionNotMet, need, source, limit,
		AnnotationHTTPRouteMaxShards, constants.MaxHTTPRouteMaxShards, constants.InferenceServiceConfigMapName)
}

// shardHTTPRouteRules packs rules into groups that each respect the Gateway
// API rule and match limits. It always returns at least one group so a route
// without rules still produces one resource.
//
// Rules in different HTTPRoutes are ordered by Gateway API match precedence
// and, on equal precedence, by route age and name, which implementations
// break differently (Istio compares its own generated names). So rules whose
// matches have the same precedence key, could match the same request and do
// different things are kept together in one shard, where list order still
// decides; see orderDependentRuleGroups. Such a group that does not fit in
// one HTTPRoute is rejected rather than split. Rules with the same action
// never need to stay together, so LoRA model-routing chunks split freely.
func shardHTTPRouteRules(rules []gwapiv1.HTTPRouteRule) ([][]gwapiv1.HTTPRouteRule, error) {
	rules, origins := splitOversizedRules(rules)
	groups := orderDependentRuleGroups(rules)
	// namedOrigins returns the named source rules a set of rules came from,
	// and whether a source shows up twice.
	namedOrigins := func(indexes []int, into map[int]struct{}) bool {
		for _, i := range indexes {
			if rules[i].Name == nil {
				continue
			}
			if _, dup := into[origins[i]]; dup {
				return true
			}
			into[origins[i]] = struct{}{}
		}
		return false
	}

	var shards [][]int
	current, matches, currentOrigins := []int(nil), 0, map[int]struct{}{}
	for _, group := range groups {
		count := 0
		for _, i := range group {
			count += ruleMatchCount(&rules[i])
		}
		if len(group) > httpRouteMaxRules || count > httpRouteMaxTotalMatches {
			return nil, fmt.Errorf("%w: rules %s match the same requests with equal precedence and different actions, "+
				"so they must stay in one HTTPRoute, but together they have %d rules and %d matches (limits %d and %d)",
				ErrPreconditionNotMet, ruleIndexList(group), len(group), count, httpRouteMaxRules, httpRouteMaxTotalMatches)
		}
		if namedOrigins(group, map[int]struct{}{}) {
			return nil, fmt.Errorf("%w: rules %s must stay in one HTTPRoute but include two parts of one named rule split "+
				"for the %d matches per rule limit; give the rule fewer matches or no name",
				ErrPreconditionNotMet, ruleIndexList(group), httpRouteMaxRuleMatches)
		}
		probe := maps.Clone(currentOrigins)
		sharesRule := namedOrigins(group, probe)
		if len(current) > 0 && (sharesRule || len(current)+len(group) > httpRouteMaxRules || matches+count > httpRouteMaxTotalMatches) {
			shards = append(shards, current)
			current, matches, currentOrigins = nil, 0, map[int]struct{}{}
		}
		current = append(current, group...)
		matches += count
		namedOrigins(group, currentOrigins)
	}
	shards = append(shards, current)

	out := make([][]gwapiv1.HTTPRouteRule, len(shards))
	for s, indexes := range shards {
		// Keep the logical list order within a shard, where it breaks ties.
		slices.Sort(indexes)
		for _, i := range indexes {
			out[s] = append(out[s], rules[i])
		}
	}
	return out, nil
}

func ruleIndexList(group []int) string {
	parts := make([]string, len(group))
	for i, idx := range group {
		parts[i] = strconv.Itoa(idx)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// orderDependentRuleGroups partitions rule indexes into groups that must share
// an HTTPRoute, ordered by their first rule. Two rules are joined when they
// have different actions and a pair of their matches has the same precedence
// key and may overlap; joins are transitive.
func orderDependentRuleGroups(rules []gwapiv1.HTTPRouteRule) [][]int {
	parent := make([]int, len(rules))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}

	actions := make([]string, len(rules))
	type entry struct {
		rule  int
		match matchFacts
	}
	buckets := map[string][]entry{}
	var keys []string
	for i := range rules {
		actions[i] = ruleActionKey(&rules[i])
		ms := rules[i].Matches
		if len(ms) == 0 {
			ms = []gwapiv1.HTTPRouteMatch{{}}
		}
		for _, m := range ms {
			k := precedenceKey(m)
			if _, ok := buckets[k]; !ok {
				keys = append(keys, k)
			}
			buckets[k] = append(buckets[k], entry{rule: i, match: newMatchFacts(m)})
		}
	}
	for _, k := range keys {
		bucket := buckets[k]
		for x := range bucket {
			for y := x + 1; y < len(bucket); y++ {
				a, b := bucket[x], bucket[y]
				if a.rule == b.rule || actions[a.rule] == actions[b.rule] || find(a.rule) == find(b.rule) {
					continue
				}
				if a.match.mayOverlap(&b.match) {
					parent[find(b.rule)] = find(a.rule)
				}
			}
		}
	}

	byRoot := map[int][]int{}
	var roots []int
	for i := range rules {
		r := find(i)
		if _, ok := byRoot[r]; !ok {
			roots = append(roots, r)
		}
		byRoot[r] = append(byRoot[r], i)
	}
	groups := make([][]int, 0, len(roots))
	for _, r := range roots {
		groups = append(groups, byRoot[r])
	}
	slices.SortFunc(groups, func(a, b []int) int { return a[0] - b[0] })
	return groups
}

// ruleActionKey identifies what a rule does with a request it matches:
// everything but its name and matches.
func ruleActionKey(rule *gwapiv1.HTTPRouteRule) string {
	action := rule.DeepCopy()
	action.Name = nil
	action.Matches = nil
	data, err := json.Marshal(action)
	if err != nil {
		// Unreachable for API types; treat the rule as unique.
		return fmt.Sprintf("%p", rule)
	}
	return string(data)
}

// precedenceKey is the part of a match that Gateway API compares to decide
// precedence across routes: path type and length, method, and the number of
// header and query param matches.
func precedenceKey(m gwapiv1.HTTPRouteMatch) string {
	pathType, pathValue := gwapiv1.PathMatchPathPrefix, "/"
	if m.Path != nil {
		pathType = ptr.Deref(m.Path.Type, gwapiv1.PathMatchPathPrefix)
		pathValue = ptr.Deref(m.Path.Value, "/")
	}
	return fmt.Sprintf("%s/%d/%s/%d/%d", pathType, len(pathValue), ptr.Deref(m.Method, ""), len(m.Headers), len(m.QueryParams))
}

// matchFacts holds the parts of a match that matchesMayOverlap compares,
// built once per match so the pairwise comparison does not rebuild the
// header and query maps for every pair.
type matchFacts struct {
	pathType gwapiv1.PathMatchType
	path     string
	headers  map[string]string
	query    map[string]string
}

func newMatchFacts(m gwapiv1.HTTPRouteMatch) matchFacts {
	f := matchFacts{pathType: gwapiv1.PathMatchPathPrefix, path: "/"}
	if m.Path != nil {
		f.pathType = ptr.Deref(m.Path.Type, gwapiv1.PathMatchPathPrefix)
		f.path = ptr.Deref(m.Path.Value, "/")
	}
	f.headers = headerExactValues(m.Headers)
	f.query = queryExactValues(m.QueryParams)
	return f
}

// matchesMayOverlap reports whether two matches with the same precedence key
// could match one request. It is conservative: only exact or prefix paths
// with different values, or an Exact header or query param with the same name
// and a different value, count as disjoint. Regular expressions are assumed
// to overlap anything.
func matchesMayOverlap(a, b gwapiv1.HTTPRouteMatch) bool {
	fa, fb := newMatchFacts(a), newMatchFacts(b)
	return fa.mayOverlap(&fb)
}

func (a *matchFacts) mayOverlap(b *matchFacts) bool {
	// Same key, so same type and length: two exact or prefix values of equal
	// length only overlap when they are equal.
	if a.pathType != gwapiv1.PathMatchRegularExpression && a.path != b.path {
		return false
	}
	if exactValuesDiffer(a.headers, b.headers) {
		return false
	}
	if exactValuesDiffer(a.query, b.query) {
		return false
	}
	return true
}

func headerExactValues(hs []gwapiv1.HTTPHeaderMatch) map[string]string {
	out := map[string]string{}
	for _, h := range hs {
		if ptr.Deref(h.Type, gwapiv1.HeaderMatchExact) == gwapiv1.HeaderMatchExact {
			out[strings.ToLower(string(h.Name))] = h.Value
		}
	}
	return out
}

func queryExactValues(qs []gwapiv1.HTTPQueryParamMatch) map[string]string {
	out := map[string]string{}
	for _, q := range qs {
		if ptr.Deref(q.Type, gwapiv1.QueryParamMatchExact) == gwapiv1.QueryParamMatchExact {
			out[string(q.Name)] = q.Value
		}
	}
	return out
}

func exactValuesDiffer(a, b map[string]string) bool {
	for name, va := range a {
		if vb, ok := b[name]; ok && va != vb {
			return true
		}
	}
	return false
}

// shardHTTPRoute splits a fully rendered route into the HTTPRoute resources
// that are written to the cluster. The first resource keeps the name, labels
// and annotations of the input. Later resources get stable suffixed names and
// a shard label. Route level fields (parentRefs, hostnames) are copied to all.
// Precedence across the resources is preserved by shardHTTPRouteRules keeping
// order-dependent rules together, not by resource names or creation order.
func shardHTTPRoute(llmSvc *v1alpha2.LLMInferenceService, route *gwapiv1.HTTPRoute) ([]*gwapiv1.HTTPRoute, error) {
	groups, err := shardHTTPRouteRules(route.Spec.Rules)
	if err != nil {
		return nil, err
	}
	routes := make([]*gwapiv1.HTTPRoute, 0, len(groups))
	for i, rules := range groups {
		shard := route.DeepCopy()
		shard.Spec.Rules = rules
		if i > 0 {
			shard.Name = managedHTTPRouteName(llmSvc.GetName(), i)
			shard.Labels = maps.Clone(route.Labels)
			if shard.Labels == nil {
				shard.Labels = map[string]string{}
			}
			shard.Labels[constants.LLMHTTPRouteShardLabelKey] = strconv.Itoa(i)
		}
		routes = append(routes, shard)
	}
	return routes, nil
}

// managedHTTPRoutes returns the stored HTTPRoutes that hold the inline route
// of owner, in shard order: the routes at the names the controller gives its
// shards (see managedHTTPRouteName) that owner controls. Routes are found by
// name and controller ownership, never by labels, so an unrelated HTTPRoute
// carrying the same labels is ignored. Consumers should use
// status.router.gateways[].httpRoutes rather than names or labels.
func managedHTTPRoutes(ctx context.Context, c client.Reader, owner metav1.Object) ([]*gwapiv1.HTTPRoute, error) {
	var routes []*gwapiv1.HTTPRoute
	for i := range constants.MaxHTTPRouteMaxShards {
		route := &gwapiv1.HTTPRoute{}
		key := client.ObjectKey{Namespace: owner.GetNamespace(), Name: managedHTTPRouteName(owner.GetName(), i)}
		switch err := c.Get(ctx, key, route); {
		case err == nil:
			if metav1.IsControlledBy(route, owner) {
				routes = append(routes, route)
			}
		case meta.IsNoMatchError(err):
			// Gateway API is not installed, so there are no routes at all.
			return nil, nil
		case !apierrors.IsNotFound(err):
			return nil, fmt.Errorf("failed to get HTTPRoute %s: %w", key, err)
		}
	}
	return routes, nil
}

// deleteStaleHTTPRouteShards deletes the additional shards controlled by the
// service whose index is not below keep. keep is the number of shards the
// current spec produces; pass 0 to delete every additional shard. The base
// route is handled by the caller.
func (r *LLMISVCReconciler) deleteStaleHTTPRouteShards(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, keep int) error {
	routes, err := managedHTTPRoutes(ctx, r, llmSvc)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route.GetName() == managedHTTPRouteName(llmSvc.GetName(), 0) {
			continue
		}
		index := managedHTTPRouteIndex(llmSvc.GetName(), route.GetName())
		if index < max(keep, 1) {
			continue
		}
		if err := Delete(ctx, r, llmSvc, route); err != nil {
			return err
		}
	}
	return nil
}

// managedHTTPRouteIndex returns the shard index of a managed route name, or
// -1 when name is not one.
func managedHTTPRouteIndex(llmSvcName, name string) int {
	for i := range constants.MaxHTTPRouteMaxShards {
		if managedHTTPRouteName(llmSvcName, i) == name {
			return i
		}
	}
	return -1
}

// httpRouteMaxShardsFor returns how many managed HTTPRoutes the service's
// inline route may use, and where that value came from for status messages.
// The spec annotation wins over the ingress config; anything unusable falls
// back to the config, as the webhook already rejects bad annotation values.
func httpRouteMaxShardsFor(llmSvc *v1alpha2.LLMInferenceService, cfg *Config) (int, string) {
	if raw, ok := llmSvc.Spec.Annotations[AnnotationHTTPRouteMaxShards]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 1 {
			// Clamped as managedHTTPRoutes only looks at shards below the
			// bound, so a value past the webhook could leave shards nothing
			// cleans up.
			return min(n, constants.MaxHTTPRouteMaxShards), "annotation " + AnnotationHTTPRouteMaxShards
		}
	}
	limit := constants.DefaultHTTPRouteMaxShards
	if cfg != nil && cfg.HTTPRouteMaxShards > 0 {
		limit = min(cfg.HTTPRouteMaxShards, constants.MaxHTTPRouteMaxShards)
	}
	return limit, "httpRouteMaxShards in the " + constants.InferenceServiceConfigMapName + " ConfigMap"
}

// apiServerVerdict returns the API server's own field errors when err carries
// an Invalid status, rather than a wrapper such as Update's "failed to get
// defaults for", so the condition message shows what to fix.
func apiServerVerdict(err error) error {
	var invalid *apierrors.StatusError
	if apierrors.IsInvalid(err) && errors.As(err, &invalid) {
		return invalid
	}
	return err
}

// dryRunHTTPRouteShards sends every desired shard to the API server as a
// dry-run create or update, so the Gateway API CRD validates the whole set
// before the first real write.
func (r *LLMISVCReconciler) dryRunHTTPRouteShards(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shards []*gwapiv1.HTTPRoute) error {
	for _, shard := range shards {
		probe := shard.DeepCopy()
		curr := &gwapiv1.HTTPRoute{}
		err := r.Get(ctx, client.ObjectKeyFromObject(probe), curr)
		switch {
		case apierrors.IsNotFound(err):
			err = r.Create(ctx, probe, client.DryRunAll)
		case err != nil:
			return fmt.Errorf("failed to get HTTPRoute %s/%s: %w", probe.GetNamespace(), probe.GetName(), err)
		case !metav1.IsControlledBy(curr, llmSvc):
			return fmt.Errorf("failed to reconcile HTTPRoute %s/%s: it is not controlled by %s/%s",
				probe.GetNamespace(), probe.GetName(), llmSvc.GetNamespace(), llmSvc.GetName())
		default:
			probe.SetResourceVersion(curr.GetResourceVersion())
			err = r.Update(ctx, probe, client.DryRunAll)
		}
		if err != nil {
			return fmt.Errorf("failed to reconcile HTTPRoute %s/%s: rejected on dry-run, no HTTPRoute was written: %w",
				probe.GetNamespace(), probe.GetName(), apiServerVerdict(err))
		}
	}
	return nil
}

// httpRouteShardWriteOrder returns the indexes of desired in the order they
// should be written. When rules move between shards, writing the shard that
// gains them before the one that loses them keeps every match that is served
// before and after the update in at least one stored route the whole time.
// Higher shards first covers rules moving forward (a rule added early in the
// list pushes later rules up); lower shards first covers rules moving back.
// When neither order is gap free, higher shards first is used.
func httpRouteShardWriteOrder(stored, desired []*gwapiv1.HTTPRoute) []int {
	descending := make([]int, len(desired))
	ascending := make([]int, len(desired))
	for i := range desired {
		descending[i] = len(desired) - 1 - i
		ascending[i] = i
	}
	state := make(map[string][]gwapiv1.HTTPRouteRule, len(stored))
	for _, route := range stored {
		state[route.GetName()] = route.Spec.Rules
	}
	before := routeMatchKeys(state)
	after := routeMatchKeys(map[string][]gwapiv1.HTTPRouteRule{"": flattenRules(desired)})
	kept := map[string]struct{}{}
	for k := range before {
		if _, ok := after[k]; ok {
			kept[k] = struct{}{}
		}
	}
	for _, order := range [][]int{descending, ascending} {
		if writeOrderKeepsMatches(state, desired, order, kept) {
			return order
		}
	}
	return descending
}

func writeOrderKeepsMatches(stored map[string][]gwapiv1.HTTPRouteRule, desired []*gwapiv1.HTTPRoute, order []int, kept map[string]struct{}) bool {
	state := maps.Clone(stored)
	for _, i := range order {
		state[desired[i].GetName()] = desired[i].Spec.Rules
		served := routeMatchKeys(state)
		for k := range kept {
			if _, ok := served[k]; !ok {
				return false
			}
		}
	}
	return true
}

func flattenRules(routes []*gwapiv1.HTTPRoute) []gwapiv1.HTTPRouteRule {
	n := 0
	for _, route := range routes {
		n += len(route.Spec.Rules)
	}
	rules := make([]gwapiv1.HTTPRouteRule, 0, n)
	for _, route := range routes {
		rules = append(rules, route.Spec.Rules...)
	}
	return rules
}

// routeMatchKeys returns a key for every match the routes serve. Matches are
// normalized with the Gateway API defaults so a stored, defaulted route and a
// freshly rendered one compare equal.
func routeMatchKeys(routes map[string][]gwapiv1.HTTPRouteRule) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, rules := range routes {
		for i := range rules {
			if len(rules[i].Matches) == 0 {
				keys[matchKey(gwapiv1.HTTPRouteMatch{})] = struct{}{}
			}
			for _, m := range rules[i].Matches {
				keys[matchKey(m)] = struct{}{}
			}
		}
	}
	return keys
}

func matchKey(m gwapiv1.HTTPRouteMatch) string {
	pathType, pathValue := gwapiv1.PathMatchPathPrefix, "/"
	if m.Path != nil {
		pathType = ptr.Deref(m.Path.Type, gwapiv1.PathMatchPathPrefix)
		pathValue = ptr.Deref(m.Path.Value, "/")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s", pathType, pathValue, ptr.Deref(m.Method, ""))
	headers := make([]string, 0, len(m.Headers))
	for _, h := range m.Headers {
		headers = append(headers, fmt.Sprintf("%s=%s:%s", ptr.Deref(h.Type, gwapiv1.HeaderMatchExact), strings.ToLower(string(h.Name)), h.Value))
	}
	slices.Sort(headers)
	query := make([]string, 0, len(m.QueryParams))
	for _, q := range m.QueryParams {
		query = append(query, fmt.Sprintf("%s=%s:%s", ptr.Deref(q.Type, gwapiv1.QueryParamMatchExact), q.Name, q.Value))
	}
	slices.Sort(query)
	fmt.Fprintf(&b, " h[%s] q[%s]", strings.Join(headers, ","), strings.Join(query, ","))
	return b.String()
}
