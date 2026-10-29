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
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// Reasons for the PerModelPathsDropped condition. They are part of the API.
const (
	reasonSetOnService                = "SetOnService"
	reasonSetOnAllGateways            = "SetOnAllGateways"
	reasonDisabledOnService           = "DisabledOnService"
	reasonUnrecognizedValue           = "UnrecognizedValue"
	reasonNotSetOnAllGateways         = "NotSetOnAllGateways"
	reasonModelBasedRoutingNotEnabled = "ModelBasedRoutingNotEnabled"
	reasonNoModelRoutingMatches       = "NoModelRoutingMatches"
)

// perModelPathsDecision is what AnnotationModelBasedRoutingOnly decided for a
// managed route. The zero value means the annotation isn't configured, which
// clears the PerModelPathsDropped condition.
type perModelPathsDecision struct {
	configured bool
	drop       bool
	reason     string
	message    string
}

func (d perModelPathsDecision) markOn(llmSvc *v1alpha2.LLMInferenceService) {
	switch {
	case !d.configured:
		llmSvc.MarkPerModelPathsDroppedUnset()
	case d.drop:
		llmSvc.MarkPerModelPathsDropped(d.reason, "%s", d.message)
	default:
		llmSvc.MarkPerModelPathsKept(d.reason, "%s", d.message)
	}
}

func dropPerModelPaths(reason, message string) perModelPathsDecision {
	return perModelPathsDecision{configured: true, drop: true, reason: reason, message: message}
}

func keepPerModelPaths(reason, message string) perModelPathsDecision {
	return perModelPathsDecision{configured: true, reason: reason, message: message}
}

// applyModelBasedRoutingOnly resolves AnnotationModelBasedRoutingOnly for the
// managed route and, when it applies, strips the per-model path matches from
// route in place. It runs after the model-routing transform, so a route whose
// model-routing matches were stripped keeps its paths.
func (r *LLMISVCReconciler) applyModelBasedRoutingOnly(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, cfg *Config, route *gwapiv1.HTTPRoute) (perModelPathsDecision, error) {
	decision, err := r.resolveModelBasedRoutingOnly(ctx, llmSvc, route)
	if err != nil || !decision.drop {
		return decision, err
	}
	if !hasModelRoutingMatch(route.Spec.Rules, cfg.ModelBasedRoutingHeaderName) {
		// The configured spec having model-routing matches that the rendered
		// route lacks means the model-routing transform stripped them.
		if cfg.ModelBasedRoutingHeaderName == "" || hasModelRoutingMatch(llmSvc.Spec.Router.Route.HTTP.Spec.Rules, cfg.ModelBasedRoutingHeaderName) {
			return keepPerModelPaths(reasonModelBasedRoutingNotEnabled, AnnotationModelBasedRoutingOnly+" is set, but model-based routing is not enabled for the service; per-model path matches are kept"), nil
		}
		return keepPerModelPaths(reasonNoModelRoutingMatches, AnnotationModelBasedRoutingOnly+" is set, but the route has no model-routing matches to fall back on; per-model path matches are kept"), nil
	}
	route.Spec.Rules = stripPerModelPathMatches(route.Spec.Rules, perModelPathPrefixes(llmSvc), cfg.ModelBasedRoutingHeaderName)
	return decision, nil
}

// resolveModelBasedRoutingOnly decides whether route drops its per-model path
// matches, before checking there is a model-routing tree to fall back on. An
// explicit service value wins, then the parent Gateways decide (see
// AnnotationModelBasedRoutingOnly). A service "false" reads the Gateways only
// to report an override; a failed read there leaves it unreported.
func (r *LLMISVCReconciler) resolveModelBasedRoutingOnly(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, route *gwapiv1.HTTPRoute) (perModelPathsDecision, error) {
	value := llmSvc.Spec.Annotations[AnnotationModelBasedRoutingOnly]
	onService, recognized := parseModelBasedRoutingOnly(value)
	if recognized && onService {
		return dropPerModelPaths(reasonSetOnService, AnnotationModelBasedRoutingOnly+" is true on the service or its preset; per-model path matches are dropped"), nil
	}

	parents := r.readParentGateways(ctx, route)
	if recognized {
		if len(parents.set) == 0 {
			return perModelPathsDecision{}, nil
		}
		return keepPerModelPaths(reasonDisabledOnService, fmt.Sprintf(
			"%s is false on the service or its preset, overriding parent Gateways %s; per-model path matches are kept",
			AnnotationModelBasedRoutingOnly, strings.Join(parents.set, ", "))), nil
	}

	switch {
	case len(parents.unset) == 0 && len(parents.unreadable) > 0:
		return perModelPathsDecision{}, fmt.Errorf("failed to get parent Gateways %s: %w", strings.Join(parents.unreadable, ", "), parents.err)
	case len(parents.set) > 0 && len(parents.unset) == 0:
		return dropPerModelPaths(reasonSetOnAllGateways, AnnotationModelBasedRoutingOnly+" is true on every parent Gateway; per-model path matches are dropped"), nil
	case value != "":
		return keepPerModelPaths(reasonUnrecognizedValue, fmt.Sprintf(
			"%s has the unrecognized value %q on the service or its preset; per-model path matches are kept", AnnotationModelBasedRoutingOnly, value)), nil
	case len(parents.set) > 0:
		return keepPerModelPaths(reasonNotSetOnAllGateways, fmt.Sprintf(
			"%s is not true on parent Gateways %s; per-model path matches are kept",
			AnnotationModelBasedRoutingOnly, strings.Join(parents.unset, ", "))), nil
	}
	return perModelPathsDecision{}, nil
}

// parseModelBasedRoutingOnly reads an AnnotationModelBasedRoutingOnly value
// with strconv.ParseBool. An empty or unparsable value isn't recognized.
func parseModelBasedRoutingOnly(value string) (enabled, recognized bool) {
	if value == "" {
		return false, false
	}
	enabled, err := strconv.ParseBool(value)
	return enabled, err == nil
}

// parentGateways sorts a route's parents by their AnnotationModelBasedRoutingOnly
// setting. Names are namespace/name, sorted and deduplicated, since they end up
// in the condition message, and a change there is a status write.
type parentGateways struct {
	set, unset, unreadable []string
	// err is the first lookup error behind unreadable.
	err error
}

// readParentGateways reads every parent of the rendered route rather than
// spec.router.gateway.refs, so the default ingress Gateway counts too, and only
// the Gateways themselves, so a GatewayClass problem cannot flip the answer.
// Every parent is read, so the outcome doesn't depend on parentRef order; a
// parent that isn't a Gateway counts as unset.
func (r *LLMISVCReconciler) readParentGateways(ctx context.Context, route *gwapiv1.HTTPRoute) parentGateways {
	var parents parentGateways
	for _, ref := range route.Spec.ParentRefs {
		key := types.NamespacedName{
			Namespace: string(ptr.Deref(ref.Namespace, gwapiv1.Namespace(route.Namespace))),
			Name:      string(ref.Name),
		}
		if ptr.Deref(ref.Group, gwapiv1.GroupName) != gwapiv1.GroupName || ptr.Deref(ref.Kind, "Gateway") != "Gateway" {
			parents.unset = append(parents.unset, key.String())
			continue
		}
		gateway := &gwapiv1.Gateway{}
		if err := r.Get(ctx, key, gateway); err != nil {
			parents.unreadable = append(parents.unreadable, key.String())
			if parents.err == nil {
				parents.err = err
			}
			continue
		}
		if enabled, recognized := parseModelBasedRoutingOnly(gateway.Annotations[AnnotationModelBasedRoutingOnly]); recognized && enabled {
			parents.set = append(parents.set, key.String())
		} else {
			parents.unset = append(parents.unset, key.String())
		}
	}
	for _, names := range []*[]string{&parents.set, &parents.unset, &parents.unreadable} {
		slices.Sort(*names)
		*names = slices.Compact(*names)
	}
	return parents
}

// perModelPathPrefixes returns the path prefixes the router route template
// exposes llmSvc under: /{namespace}/{name} and the publisher-qualified
// /publishers/{namespace}/models/{model}.
func perModelPathPrefixes(llmSvc *v1alpha2.LLMInferenceService) []string {
	return []string{
		"/" + llmSvc.GetNamespace() + "/" + llmSvc.GetName(),
		"/" + fullyQualifiedModelName(llmSvc.GetNamespace(), resolveModelName(llmSvc)),
	}
}

// stripPerModelPathMatches removes matches that route by a path under one of
// prefixes. Model-routing matches and paths outside the prefixes are kept, so
// user rules survive. A rule with an empty match list applies to every request,
// so it is kept; any other rule is dropped once stripping has emptied it.
func stripPerModelPathMatches(rules []gwapiv1.HTTPRouteRule, prefixes []string, headerName string) []gwapiv1.HTTPRouteRule {
	if len(prefixes) == 0 {
		return rules
	}
	var filtered []gwapiv1.HTTPRouteRule
	for i := range rules {
		if len(rules[i].Matches) == 0 {
			filtered = append(filtered, rules[i])
			continue
		}
		var kept []gwapiv1.HTTPRouteMatch
		for _, match := range rules[i].Matches {
			if !isPerModelPathMatch(match, prefixes, headerName) {
				kept = append(kept, match)
			}
		}
		if len(kept) > 0 {
			rules[i].Matches = kept
			filtered = append(filtered, rules[i])
		}
	}
	return filtered
}

// isPerModelPathMatch reports whether match selects the model by a path under
// one of prefixes rather than by the model-routing header. Prefixes compare by
// path segment, so /ns/name does not claim /ns/name-other. A regular-expression
// path is a pattern rather than a path and is never claimed.
func isPerModelPathMatch(match gwapiv1.HTTPRouteMatch, prefixes []string, headerName string) bool {
	if match.Path == nil || match.Path.Value == nil || isModelBasedRoutingMatch(match, headerName) {
		return false
	}
	if ptr.Deref(match.Path.Type, gwapiv1.PathMatchPathPrefix) == gwapiv1.PathMatchRegularExpression {
		return false
	}
	path := *match.Path.Value
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
