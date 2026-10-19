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
	"crypto/sha256"
	"encoding/hex"
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

// httpRouteRuleNameMaxLength is the Gateway API SectionName length limit.
const httpRouteRuleNameMaxLength = 253

// splitOversizedRules splits every rule with more matches than a single rule
// may hold into consecutive rules with the same filters, backends and
// timeouts. Chunks after the first get a derived name on a named rule so rule
// names stay unique within a route; see chunkRuleName.
func splitOversizedRules(rules []gwapiv1.HTTPRouteRule) []gwapiv1.HTTPRouteRule {
	used := make(map[gwapiv1.SectionName]struct{}, len(rules))
	for i := range rules {
		if rules[i].Name != nil {
			used[*rules[i].Name] = struct{}{}
		}
	}
	out := make([]gwapiv1.HTTPRouteRule, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		if len(rule.Matches) <= httpRouteMaxRuleMatches {
			out = append(out, rule)
			continue
		}
		for chunk, start := 0, 0; start < len(rule.Matches); chunk, start = chunk+1, start+httpRouteMaxRuleMatches {
			end := min(start+httpRouteMaxRuleMatches, len(rule.Matches))
			part := *rule.DeepCopy()
			part.Matches = part.Matches[start:end]
			if chunk > 0 && part.Name != nil {
				name := chunkRuleName(*part.Name, chunk, used)
				used[name] = struct{}{}
				part.Name = ptr.To(name)
			}
			out = append(out, part)
		}
	}
	return out
}

// chunkRuleName returns the name of the chunk-th extra rule split from a rule
// named base. The plain form is "<base>-<chunk>". When that is longer than a
// SectionName may be, or already names another rule, base is cut and a short
// hash of the base, chunk and attempt is appended, so the result is valid,
// deterministic and unique among used.
func chunkRuleName(base gwapiv1.SectionName, chunk int, used map[gwapiv1.SectionName]struct{}) gwapiv1.SectionName {
	plain := gwapiv1.SectionName(fmt.Sprintf("%s-%d", base, chunk))
	if _, taken := used[plain]; !taken && len(plain) <= httpRouteRuleNameMaxLength {
		return plain
	}
	for attempt := 0; ; attempt++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%d", base, chunk, attempt)))
		suffix := fmt.Sprintf("-%d-%s", chunk, hex.EncodeToString(sum[:])[:8])
		prefix := string(base)
		if len(prefix)+len(suffix) > httpRouteRuleNameMaxLength {
			prefix = prefix[:httpRouteRuleNameMaxLength-len(suffix)]
		}
		// A SectionName label must start and end with an alphanumeric character.
		prefix = strings.TrimRight(prefix, "-.")
		name := gwapiv1.SectionName(prefix + suffix)
		if _, taken := used[name]; !taken {
			return name
		}
	}
}

// shardHTTPRouteRules packs rules, in order, into groups that each respect the
// Gateway API rule and match limits. It always returns at least one group so
// a route without rules still produces one resource.
func shardHTTPRouteRules(rules []gwapiv1.HTTPRouteRule) [][]gwapiv1.HTTPRouteRule {
	rules = splitOversizedRules(rules)
	shards := [][]gwapiv1.HTTPRouteRule{nil}
	matches := 0
	for i := range rules {
		count := ruleMatchCount(&rules[i])
		last := len(shards) - 1
		if len(shards[last]) > 0 && (len(shards[last]) >= httpRouteMaxRules || matches+count > httpRouteMaxTotalMatches) {
			shards = append(shards, nil)
			last++
			matches = 0
		}
		shards[last] = append(shards[last], rules[i])
		matches += count
	}
	return shards
}

// shardHTTPRoute splits a fully rendered route into the HTTPRoute resources
// that are written to the cluster. The first resource keeps the name, labels
// and annotations of the input. Later resources get stable suffixed names and
// a shard label. Route level fields (parentRefs, hostnames) are copied to all.
//
// Gateway API precedence between matches of equal specificity falls back to
// the oldest route, then to the route name; the first shard is created first
// and sorts first by name, so rule order is preserved on such ties.
func shardHTTPRoute(llmSvc *v1alpha2.LLMInferenceService, route *gwapiv1.HTTPRoute) []*gwapiv1.HTTPRoute {
	groups := shardHTTPRouteRules(route.Spec.Rules)
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
	return routes
}

// listHTTPRouteShards returns the additional (index >= 1) managed HTTPRoutes
// of the named service, sorted by shard index.
func listHTTPRouteShards(ctx context.Context, c client.Reader, namespace, llmSvcName string) ([]*gwapiv1.HTTPRoute, error) {
	list := &gwapiv1.HTTPRouteList{}
	if err := c.List(ctx, list, client.InNamespace(namespace),
		client.MatchingLabels{constants.KubernetesAppNameLabelKey: llmSvcName},
		client.HasLabels{constants.LLMHTTPRouteShardLabelKey}); err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list HTTPRoute shards of %s/%s: %w", namespace, llmSvcName, err)
	}
	routes := make([]*gwapiv1.HTTPRoute, 0, len(list.Items))
	for i := range list.Items {
		if _, err := strconv.Atoi(list.Items[i].Labels[constants.LLMHTTPRouteShardLabelKey]); err != nil {
			continue
		}
		routes = append(routes, &list.Items[i])
	}
	slices.SortFunc(routes, func(a, b *gwapiv1.HTTPRoute) int {
		ai, _ := strconv.Atoi(a.Labels[constants.LLMHTTPRouteShardLabelKey])
		bi, _ := strconv.Atoi(b.Labels[constants.LLMHTTPRouteShardLabelKey])
		return ai - bi
	})
	return routes, nil
}

// managedHTTPRoutes returns every stored managed HTTPRoute of the named
// service: the base route, when present, followed by its additional shards.
func managedHTTPRoutes(ctx context.Context, c client.Reader, namespace, llmSvcName string) ([]*gwapiv1.HTTPRoute, error) {
	var routes []*gwapiv1.HTTPRoute
	base := &gwapiv1.HTTPRoute{}
	key := client.ObjectKey{Namespace: namespace, Name: managedHTTPRouteName(llmSvcName, 0)}
	switch err := c.Get(ctx, key, base); {
	case err == nil:
		routes = append(routes, base)
	case meta.IsNoMatchError(err):
		// Gateway API is not installed, so there are no routes at all.
		return nil, nil
	case !apierrors.IsNotFound(err):
		return nil, fmt.Errorf("failed to get HTTPRoute %s: %w", key, err)
	}
	shards, err := listHTTPRouteShards(ctx, c, namespace, llmSvcName)
	if err != nil {
		return nil, err
	}
	return append(routes, shards...), nil
}

// deleteStaleHTTPRouteShards deletes additional shards controlled by the
// service whose index is not below keep. keep is the number of shards the
// current spec produces; pass 0 to delete every additional shard. The base
// route is handled by the caller.
func (r *LLMISVCReconciler) deleteStaleHTTPRouteShards(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, keep int) error {
	shards, err := listHTTPRouteShards(ctx, r, llmSvc.GetNamespace(), llmSvc.GetName())
	if err != nil {
		return err
	}
	for _, shard := range shards {
		index, _ := strconv.Atoi(shard.Labels[constants.LLMHTTPRouteShardLabelKey])
		if index < keep || !metav1.IsControlledBy(shard, llmSvc) {
			continue
		}
		if err := Delete(ctx, r, llmSvc, shard); err != nil {
			return err
		}
	}
	return nil
}

// httpRouteMaxShardsFor returns how many managed HTTPRoutes the service's
// inline route may use, and where that value came from for status messages.
// The spec annotation wins over the ingress config; anything unusable falls
// back to the config, as the webhook already rejects bad annotation values.
func httpRouteMaxShardsFor(llmSvc *v1alpha2.LLMInferenceService, cfg *Config) (int, string) {
	if raw, ok := llmSvc.Spec.Annotations[AnnotationHTTPRouteMaxShards]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 1 {
			return n, "annotation " + AnnotationHTTPRouteMaxShards
		}
	}
	limit := constants.DefaultHTTPRouteMaxShards
	if cfg != nil && cfg.HTTPRouteMaxShards > 0 {
		limit = cfg.HTTPRouteMaxShards
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
	var rules []gwapiv1.HTTPRouteRule
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
