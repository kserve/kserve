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
	"fmt"
	"regexp"
	"slices"
	"strings"

	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// LoRAModelRoutingStrategy selects how LoRA adapter expansion represents the
// qualified model-name-to-pool index in the generated HTTPRoute. The cluster-wide
// value comes from the inferenceservice-config ConfigMap; a service pins its own
// with AnnotationLoRAModelRoutingStrategy.
type LoRAModelRoutingStrategy string

const (
	// LoRAModelRoutingStrategyExact renders one Exact header match per model
	// identity (base model plus each adapter). Portable Gateway API Core
	// behavior; capacity is bounded by the per-rule match budget.
	LoRAModelRoutingStrategyExact LoRAModelRoutingStrategy = constants.LoRAModelRoutingStrategyExact
	// LoRAModelRoutingStrategyRegex collapses the base model and all adapters
	// into a single anchored RegularExpression header match. Header regex
	// matching is implementation-specific in Gateway API, and the adapter
	// ceiling is set by the provider's RE2 program-size budget, which KServe
	// can neither read nor validate against: Envoy Gateway disables the check
	// (the 4096-character header value limit binds), Istio allows 32768, and
	// Envoy's stock default of 100 fits only a couple of adapters. A proxy that
	// rejects the pattern reports an xDS NACK, not an HTTPRoute condition.
	LoRAModelRoutingStrategyRegex LoRAModelRoutingStrategy = constants.LoRAModelRoutingStrategyRegex
)

// applyLoRAModelRouting expands the model-routing matches of a service's
// generated route for its LoRA adapters, using the configured strategy. It is
// a no-op when there is nothing to expand - no named adapters, or no
// model-routing match to expand them into - so a strategy value that cannot be
// applied never fails a path-only route or an adapter-less service.
func applyLoRAModelRouting(rules []gwapiv1.HTTPRouteRule, llmSvc *v1alpha2.LLMInferenceService, cfg *Config) error {
	if llmSvc.Spec.Model.LoRA == nil {
		return nil
	}
	adapterNames := loraAdapterNames(llmSvc.Spec.Model.LoRA.Adapters)
	if len(adapterNames) == 0 || !hasModelRoutingMatch(rules, cfg.ModelBasedRoutingHeaderName) {
		return nil
	}

	strategy, source := loraRoutingStrategyFor(llmSvc, cfg)
	switch strategy {
	case LoRAModelRoutingStrategyRegex:
		baseModel := ptr.Deref(llmSvc.Spec.Model.Name, llmSvc.GetName())
		if err := applyLoRARegexMatches(rules, llmSvc.Namespace, baseModel, adapterNames, cfg.ModelBasedRoutingHeaderName); err != nil {
			return fmt.Errorf("%w: %w", ErrPreconditionNotMet, err)
		}
		return nil
	case LoRAModelRoutingStrategyExact, "":
		// A loaded Config is never empty (NewIngressConfig defaults it); the
		// zero value keeps directly constructed Configs byte-compatible with
		// the pre-strategy behavior.
		expandLoRAAdapterMatches(rules, llmSvc.Namespace, llmSvc.Spec.Model.LoRA.Adapters, cfg.ModelBasedRoutingHeaderName)
		return nil
	default:
		// Unreachable for values that passed admission (annotation) or config
		// loading (ConfigMap). Kept so an object that bypassed the webhook
		// fails closed instead of silently falling back to Exact.
		return fmt.Errorf("%w: unsupported loraModelRoutingStrategy %q from %s: supported values are %q and %q",
			ErrPreconditionNotMet, strategy, source, LoRAModelRoutingStrategyExact, LoRAModelRoutingStrategyRegex)
	}
}

// loraRoutingStrategyFor resolves one service's strategy - its spec annotation
// when set, otherwise the cluster-wide value cfg already carries canonically -
// and names the source, so a rejected value tells the operator where to fix it.
// The annotation is normalized here because the webhook accepts it
// case-insensitively and the API server stores it as written.
func loraRoutingStrategyFor(llmSvc *v1alpha2.LLMInferenceService, cfg *Config) (LoRAModelRoutingStrategy, string) {
	if v := strings.TrimSpace(llmSvc.Spec.Annotations[AnnotationLoRAModelRoutingStrategy]); v != "" {
		return LoRAModelRoutingStrategy(strings.ToLower(v)), "annotation " + AnnotationLoRAModelRoutingStrategy
	}
	return cfg.LoRAModelRoutingStrategy, "the " + constants.InferenceServiceConfigMapName + " ConfigMap"
}

// hasModelRoutingMatch reports whether any rule carries a model-routing header
// match, i.e. whether LoRA expansion has anything to rewrite.
func hasModelRoutingMatch(rules []gwapiv1.HTTPRouteRule, headerName string) bool {
	for i := range rules {
		for _, match := range rules[i].Matches {
			if isModelBasedRoutingMatch(match, headerName) {
				return true
			}
		}
	}
	return false
}

// loraRegexHeaderValue renders the anchored RE2 pattern matching the base model
// and every LoRA adapter identity:
//
//	^publishers/<namespace>/models/(<base>|<adapter-1>|...)$
//
// The identity prefix comes from fullyQualifiedModelName, so rendering cannot
// drift from validation; the base model and each adapter are escaped with
// regexp.QuoteMeta. Alternatives are sorted and deduplicated, so the pattern is
// deterministic regardless of spec order, and the base model appears once.
func loraRegexHeaderValue(namespace, baseModel string, adapterNames []string) string {
	names := slices.Clone(adapterNames)
	slices.Sort(names)
	names = slices.Compact(names)

	alternatives := make([]string, 0, len(names)+1)
	alternatives = append(alternatives, regexp.QuoteMeta(baseModel))
	for _, name := range names {
		if name == baseModel {
			continue
		}
		alternatives = append(alternatives, regexp.QuoteMeta(name))
	}

	return "^" + regexp.QuoteMeta(fullyQualifiedModelName(namespace, "")) + "(" + strings.Join(alternatives, "|") + ")$"
}

// loraAdapterNames returns the declared adapter served names, unordered.
// Unnamed and empty-named entries are skipped - an empty alternative would
// match the empty model identity.
func loraAdapterNames(adapters []v1alpha2.LLMModelSpec) []string {
	names := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter.Name == nil || *adapter.Name == "" {
			continue
		}
		names = append(names, *adapter.Name)
	}
	return names
}

// applyLoRARegexMatches rewrites every recognized model-routing header match to
// a single RegularExpression match covering the base model and adapterNames.
// Recognition is deliberately narrow: the header name must equal
// the configured model-routing header (ASCII case-insensitively, because HTTP
// header-name matching is case-insensitive in Gateway API) and the match must
// carry the expected generated base-model identity as an Exact value. A match
// on the model-routing header with any other shape fails structural validation
// instead of being silently overwritten; validation runs over the whole rule
// set before any header is rewritten, so a failure never leaves a partially
// transformed spec. Path rules and unrelated headers are never touched.
func applyLoRARegexMatches(rules []gwapiv1.HTTPRouteRule, namespace, baseModel string, adapterNames []string, headerName string) error {
	if len(adapterNames) == 0 {
		return nil // nothing to collapse; a base-only pattern would be misleading
	}

	baseValue := fullyQualifiedModelName(namespace, baseModel)

	// Validate every match before rewriting any, so a failed validation leaves
	// the route untouched.
	var recognized []*gwapiv1.HTTPHeaderMatch
	for i := range rules {
		for j := range rules[i].Matches {
			header, err := recognizeBaseModelHeader(&rules[i].Matches[j], headerName, baseValue)
			if err != nil {
				return fmt.Errorf("LoRA routing strategy %q cannot be applied: rule %q match %d %w",
					LoRAModelRoutingStrategyRegex, ptr.Deref(rules[i].Name, ""), j, err)
			}
			if header != nil {
				recognized = append(recognized, header)
			}
		}
	}

	regexValue := loraRegexHeaderValue(namespace, baseModel, adapterNames)
	for _, header := range recognized {
		header.Type = ptr.To(gwapiv1.HeaderMatchRegularExpression)
		header.Value = regexValue
	}

	return nil
}

// recognizeBaseModelHeader returns the match's model-routing header when it has
// the generated shape, nil when the match carries none, and an error explaining
// why an otherwise-matching header cannot be rewritten. The error omits the rule
// and match position, which the caller supplies.
func recognizeBaseModelHeader(match *gwapiv1.HTTPRouteMatch, headerName, baseValue string) (*gwapiv1.HTTPHeaderMatch, error) {
	var found *gwapiv1.HTTPHeaderMatch
	for h := range match.Headers {
		header := &match.Headers[h]
		if !isModelRoutingHeader(header.Name, headerName) {
			continue
		}
		// Gateway API only evaluates the first of several equivalent header
		// names, so a match carrying more than one is rejected.
		if found != nil {
			return nil, fmt.Errorf("carries multiple case-equivalent %s headers; Gateway API only evaluates the first", headerName)
		}
		headerType := ptr.Deref(header.Type, gwapiv1.HeaderMatchExact)
		if headerType != gwapiv1.HeaderMatchExact || header.Value != baseValue {
			return nil, fmt.Errorf("has an unrecognized %s match (type %s, value %q); expected an Exact match on the base model identity %q",
				header.Name, headerType, header.Value, baseValue)
		}
		found = header
	}
	return found, nil
}
