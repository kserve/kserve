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
	shards, _, err := packHTTPRouteRules(rules)
	return shards, err
}

// packHTTPRouteRules is shardHTTPRouteRules that also returns, for every
// shard, the sorted indexes of the input rules its rules came from.
func packHTTPRouteRules(input []gwapiv1.HTTPRouteRule) ([][]gwapiv1.HTTPRouteRule, [][]int, error) {
	rules, origins := splitOversizedRules(input)
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
			return nil, nil, fmt.Errorf("%w: %s match the same requests with equal precedence and different actions, "+
				"so they must stay in one HTTPRoute, but together they have %d rules and %d matches (limits %d and %d); "+
				"the stored routes keep serving",
				ErrPreconditionNotMet, describeSourceRules(input, sourceIndexes(group, origins)), len(group), count,
				httpRouteMaxRules, httpRouteMaxTotalMatches)
		}
		if namedOrigins(group, map[int]struct{}{}) {
			return nil, nil, fmt.Errorf("%w: %s must stay in one HTTPRoute but include two parts of one named rule split "+
				"for the %d matches per rule limit; give the rule fewer matches or no name; the stored routes keep serving",
				ErrPreconditionNotMet, describeSourceRules(input, sourceIndexes(group, origins)), httpRouteMaxRuleMatches)
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
	sources := make([][]int, len(shards))
	for s, indexes := range shards {
		// Keep the logical list order within a shard, where it breaks ties.
		slices.Sort(indexes)
		for _, i := range indexes {
			out[s] = append(out[s], rules[i])
		}
		sources[s] = sourceIndexes(indexes, origins)
	}
	return out, sources, nil
}

// sourceIndexes maps indexes of split rules back to the sorted, deduplicated
// indexes of the input rules they came from.
func sourceIndexes(indexes, origins []int) []int {
	out := make([]int, 0, len(indexes))
	for _, i := range indexes {
		out = append(out, origins[i])
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// maxDescribedRules caps how many rules a condition message lists.
const maxDescribedRules = 5

// describeSourceRules names input rules for a condition message, with the
// rule name when there is one, listing at most maxDescribedRules so a big
// group does not put hundreds of indexes into a condition.
func describeSourceRules(rules []gwapiv1.HTTPRouteRule, indexes []int) string {
	parts := make([]string, 0, min(len(indexes), maxDescribedRules)+1)
	for _, i := range indexes[:min(len(indexes), maxDescribedRules)] {
		if name := ptr.Deref(rules[i].Name, ""); name != "" {
			parts = append(parts, fmt.Sprintf("%d (%s)", i, name))
		} else {
			parts = append(parts, strconv.Itoa(i))
		}
	}
	if extra := len(indexes) - maxDescribedRules; extra > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", extra))
	}
	noun := "rules "
	if len(indexes) == 1 {
		noun = "rule "
	}
	return noun + strings.Join(parts, ", ")
}

// describeSourceRange names the input rules a shard holds as index ranges,
// such as "inline route rules 0-15, 18".
func describeSourceRange(indexes []int) string {
	var parts []string
	for start := 0; start < len(indexes); {
		end := start
		for end+1 < len(indexes) && indexes[end+1] == indexes[end]+1 {
			end++
		}
		if end == start {
			parts = append(parts, strconv.Itoa(indexes[start]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", indexes[start], indexes[end]))
		}
		start = end + 1
	}
	if len(indexes) == 1 {
		return "inline route rule " + parts[0]
	}
	return "inline route rules " + strings.Join(parts, ", ")
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
// everything but its matches. The name is part of the action, because a
// policy can attach to a rule by sectionName, so two overlapping rules with
// the same backend but different names must keep their list order. Chunks of
// one split rule share its name, so LoRA chunks still split freely.
func ruleActionKey(rule *gwapiv1.HTTPRouteRule) string {
	action := rule.DeepCopy()
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
// header and query param matches. It is coarser than the spec in two places
// where implementations tie matches the spec orders:
//   - a PathPrefix value is compared without its trailing "/", which the spec
//     ignores for prefix matching and Istio trims, so /tie and /tie/ tie;
//   - a method counts as one more header, as Envoy Gateway turns the method
//     into a header match, so /tie + GET ties /tie + one header.
//
// A coarser key only groups more rules together, which is always safe.
//
// For RegularExpression paths the spec leaves precedence to the
// implementation. The key counts the pattern length, so two overlapping
// regexes of different lengths may land in different HTTPRoutes. That keeps
// the winner only because the gateway ranks a longer regex first, as Istio
// (rank, then length) and Envoy Gateway (type, then length) do. Neither
// ordering is a promise of the spec, so a gateway that ranks regexes another
// way can change the winner once such rules are sharded.
func precedenceKey(m gwapiv1.HTTPRouteMatch) string {
	pathType, pathValue := matchPath(m)
	headers := len(m.Headers)
	if m.Method != nil {
		headers++
	}
	return fmt.Sprintf("%s/%d/%d/%d", pathType, len(pathValue), headers, len(m.QueryParams))
}

// matchPath returns the defaulted path type and value of a match, with the
// trailing "/" of a PathPrefix value other than "/" trimmed; see
// precedenceKey.
func matchPath(m gwapiv1.HTTPRouteMatch) (gwapiv1.PathMatchType, string) {
	pathType, pathValue := gwapiv1.PathMatchPathPrefix, "/"
	if m.Path != nil {
		pathType = ptr.Deref(m.Path.Type, gwapiv1.PathMatchPathPrefix)
		pathValue = ptr.Deref(m.Path.Value, "/")
	}
	if pathType == gwapiv1.PathMatchPathPrefix && len(pathValue) > 1 {
		pathValue = strings.TrimSuffix(pathValue, "/")
	}
	return pathType, pathValue
}

// matchFacts holds the parts of a match that matchesMayOverlap compares,
// built once per match so the pairwise comparison does not rebuild the
// header and query maps for every pair.
type matchFacts struct {
	pathType gwapiv1.PathMatchType
	path     string
	method   string
	headers  map[string]string
	query    map[string]string
}

func newMatchFacts(m gwapiv1.HTTPRouteMatch) matchFacts {
	f := matchFacts{method: string(ptr.Deref(m.Method, ""))}
	f.pathType, f.path = matchPath(m)
	f.headers = headerExactValues(m.Headers)
	f.query = queryExactValues(m.QueryParams)
	return f
}

// matchesMayOverlap reports whether two matches with the same precedence key
// could match one request. It is conservative: only exact or prefix paths
// with different values (after the trailing "/" trim of precedenceKey), two
// different methods, or an Exact header or query param with the same name
// and a different value, count as disjoint. A method against a header match
// is assumed to overlap. Regular expressions are assumed
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
	if a.method != "" && b.method != "" && a.method != b.method {
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
	routes, _, err := shardHTTPRouteWithSources(llmSvc, route)
	return routes, err
}

// shardHTTPRouteWithSources is shardHTTPRoute that also returns, for every
// shard, the indexes of the input route rules it holds.
func shardHTTPRouteWithSources(llmSvc *v1alpha2.LLMInferenceService, route *gwapiv1.HTTPRoute) ([]*gwapiv1.HTTPRoute, [][]int, error) {
	groups, sources, err := packHTTPRouteRules(route.Spec.Rules)
	if err != nil {
		return nil, nil, err
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
	return routes, sources, nil
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

// reconcileHTTPRouteShards writes the shards of the inline route and then
// deletes the shards the route no longer needs.
//
// Kubernetes has no atomic write across several HTTPRoutes, so the whole
// set is validated first: a shard the API server would reject fails the
// reconcile before any shard is written, and the stored routes keep
// serving. This cannot stop a transient mix of old and new shards while
// the writes below run.
//
// That dry-run is also the defaults dry-run the comparison needs, so each
// shard costs one dry-run per reconcile and only the shards that differ are
// written.
func (r *LLMISVCReconciler) reconcileHTTPRouteShards(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shards []*gwapiv1.HTTPRoute, sources [][]int) error {
	stored, err := managedHTTPRoutes(ctx, r, llmSvc)
	if err != nil {
		return err
	}
	writes, err := r.prepareHTTPRouteShards(ctx, llmSvc, shards, sources)
	if err != nil {
		return err
	}
	for _, i := range httpRouteShardWriteOrder(stored, shards) {
		if err := r.writeHTTPRouteShard(ctx, llmSvc, shards[i], writes[i]); err != nil {
			return err
		}
	}
	// Only after every current shard is written, so traffic covered by a
	// shrinking route never loses its rules in between.
	return r.deleteStaleHTTPRouteShards(ctx, llmSvc, len(shards))
}

// httpRouteShardWrite is what prepareHTTPRouteShards decided for one shard.
type httpRouteShardWrite int

const (
	httpRouteShardUnchanged httpRouteShardWrite = iota
	httpRouteShardCreate
	httpRouteShardUpdate
)

// prepareHTTPRouteShards sends every desired shard to the API server as a
// dry-run create or update, so the Gateway API CRD validates the whole set
// before the first real write. The dry-run update is the one PrepareUpdate
// uses for server defaults, so each shard is dry-run once per reconcile;
// shards that would update are left carrying the dry-run result for
// writeHTTPRouteShard.
func (r *LLMISVCReconciler) prepareHTTPRouteShards(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shards []*gwapiv1.HTTPRoute, sources [][]int) ([]httpRouteShardWrite, error) {
	writes := make([]httpRouteShardWrite, len(shards))
	for s, shard := range shards {
		rejected := func(err error) error {
			return fmt.Errorf("failed to reconcile HTTPRoute %s/%s (%s): rejected on dry-run, "+
				"no HTTPRoute was written and the stored routes keep serving: %w",
				shard.GetNamespace(), shard.GetName(), describeSourceRange(sources[s]), apiServerVerdict(err))
		}
		curr := &gwapiv1.HTTPRoute{}
		switch err := r.Get(ctx, client.ObjectKeyFromObject(shard), curr); {
		case apierrors.IsNotFound(err):
			if err := r.Create(ctx, shard.DeepCopy(), client.DryRunAll); err != nil {
				return nil, rejected(err)
			}
			writes[s] = httpRouteShardCreate
		case err != nil:
			return nil, fmt.Errorf("failed to get HTTPRoute %s/%s: %w", shard.GetNamespace(), shard.GetName(), err)
		case !metav1.IsControlledBy(curr, llmSvc):
			return nil, fmt.Errorf("failed to reconcile HTTPRoute %s/%s: it is not controlled by %s/%s",
				shard.GetNamespace(), shard.GetName(), llmSvc.GetNamespace(), llmSvc.GetName())
		default:
			changed, err := PrepareUpdate(ctx, r, llmSvc, curr, shard, semanticHTTPRouteIsEqual)
			if err != nil {
				return nil, rejected(err)
			}
			if changed {
				writes[s] = httpRouteShardUpdate
			}
		}
	}
	return writes, nil
}

// writeHTTPRouteShard performs the write prepareHTTPRouteShards decided on.
func (r *LLMISVCReconciler) writeHTTPRouteShard(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shard *gwapiv1.HTTPRoute, write httpRouteShardWrite) error {
	var err error
	switch write {
	case httpRouteShardCreate:
		err = Create(ctx, r, llmSvc, shard)
	case httpRouteShardUpdate:
		err = CommitUpdate(ctx, r, llmSvc, shard)
	default:
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to reconcile HTTPRoute %s/%s: %w", shard.GetNamespace(), shard.GetName(), apiServerVerdict(err))
	}
	return nil
}

// httpRouteShardWriteOrder returns the indexes of desired in the order they
// should be written, so every match that is served before and after the
// update stays in at least one stored route the whole time.
//
// A match needs ordering only when no route holds it throughout: no stored
// route that the update leaves alone (a stale shard is deleted only after
// every write) and no shard that has it both before and after. For such a
// match, a shard that gains it is written before every shard that loses it.
// The writes are then sorted topologically, preferring higher shards first,
// which also is the order when nothing moves. Only when the constraints form
// a cycle, as when two shards swap rules, is higher shards first used as a
// fallback, which may briefly drop the rules that move back.
func httpRouteShardWriteOrder(stored, desired []*gwapiv1.HTTPRoute) []int {
	n := len(desired)
	descending := make([]int, n)
	for i := range desired {
		descending[i] = n - 1 - i
	}

	storedByName := make(map[string]map[string]struct{}, len(stored))
	for _, route := range stored {
		storedByName[route.GetName()] = routeMatchKeys(map[string][]gwapiv1.HTTPRouteRule{"": route.Spec.Rules})
	}
	desiredNames := make(map[string]struct{}, n)
	before := make([]map[string]struct{}, n)
	after := make([]map[string]struct{}, n)
	for i, route := range desired {
		desiredNames[route.GetName()] = struct{}{}
		before[i] = storedByName[route.GetName()]
		after[i] = routeMatchKeys(map[string][]gwapiv1.HTTPRouteRule{"": route.Spec.Rules})
	}

	stable := map[string]struct{}{}
	for name, keys := range storedByName {
		if _, ok := desiredNames[name]; !ok {
			maps.Copy(stable, keys)
		}
	}
	for i := range desired {
		for k := range before[i] {
			if _, ok := after[i][k]; ok {
				stable[k] = struct{}{}
			}
		}
	}

	// edges[j][i] means shard j is written before shard i.
	edges := make([]map[int]struct{}, n)
	indegree := make([]int, n)
	for i := range n {
		edges[i] = map[int]struct{}{}
	}
	for loser := range desired {
		for k := range before[loser] {
			if _, ok := after[loser][k]; ok {
				continue
			}
			if _, ok := stable[k]; ok {
				continue
			}
			for gainer := range desired {
				if _, gains := after[gainer][k]; !gains || gainer == loser {
					continue
				}
				if _, dup := edges[gainer][loser]; !dup {
					edges[gainer][loser] = struct{}{}
					indegree[loser]++
				}
				break
			}
		}
	}

	order := make([]int, 0, n)
	done := make([]bool, n)
	for len(order) < n {
		next := -1
		for i := n - 1; i >= 0; i-- {
			if !done[i] && indegree[i] == 0 {
				next = i
				break
			}
		}
		if next < 0 {
			return descending // a cycle: shards swap rules
		}
		done[next] = true
		order = append(order, next)
		for i := range edges[next] {
			indegree[i]--
		}
	}

	state := make(map[string][]gwapiv1.HTTPRouteRule, len(stored))
	for _, route := range stored {
		state[route.GetName()] = route.Spec.Rules
	}
	kept := map[string]struct{}{}
	all := routeMatchKeys(map[string][]gwapiv1.HTTPRouteRule{"": flattenRules(desired)})
	for k := range routeMatchKeys(state) {
		if _, ok := all[k]; ok {
			kept[k] = struct{}{}
		}
	}
	if !writeOrderKeepsMatches(state, desired, order, kept) {
		return descending
	}
	return order
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

// mergeManagedHTTPRouteShards returns routes with the managed shards of
// llmSvc's inline route replaced by one route holding all their rules, in
// shard order, at the position of the first shard. The shards share
// metadata, parentRefs and hostnames, so the merged route reads the same as
// the logical route would. Other routes are returned unchanged.
func mergeManagedHTTPRouteShards(llmSvc *v1alpha2.LLMInferenceService, routes []*gwapiv1.HTTPRoute) []*gwapiv1.HTTPRoute {
	out := make([]*gwapiv1.HTTPRoute, 0, len(routes))
	var shards []*gwapiv1.HTTPRoute
	first := -1
	for _, route := range routes {
		if metav1.IsControlledBy(route, llmSvc) && managedHTTPRouteIndex(llmSvc.GetName(), route.GetName()) >= 0 {
			if first < 0 {
				first = len(out)
				out = append(out, nil)
			}
			shards = append(shards, route)
			continue
		}
		out = append(out, route)
	}
	if first < 0 {
		return out
	}
	slices.SortStableFunc(shards, func(a, b *gwapiv1.HTTPRoute) int {
		return managedHTTPRouteIndex(llmSvc.GetName(), a.GetName()) - managedHTTPRouteIndex(llmSvc.GetName(), b.GetName())
	})
	if len(shards) == 1 {
		out[first] = shards[0]
		return out
	}
	merged := shards[0].DeepCopy()
	merged.Spec.Rules = flattenRules(shards)
	out[first] = merged
	return out
}

// dedupeDiscoveredURLs drops repeated addresses, keeping the first of each.
// Two gateways that expose the same URL are different addresses, so the
// key includes the origin gateway.
func dedupeDiscoveredURLs(urls []DiscoveredURL) []DiscoveredURL {
	seen := make(map[string]struct{}, len(urls))
	out := urls[:0]
	for _, u := range urls {
		key := u.URL.String()
		if o := u.Origin; o != nil {
			key += "\x00" + string(o.Group) + "/" + string(o.Kind) + "/" + string(ptr.Deref(o.Namespace, "")) + "/" + string(o.Name)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, u)
	}
	return out
}
