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
	"strings"

	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

type RouteRuleNaming string

const (
	RouteRuleNamingPreset RouteRuleNaming = constants.RouteRuleNamingPreset
	// RouteRuleNamingUnique exists for Istio before 1.29.9, 1.30.6 and 1.31.2,
	// which keys the InferencePool endpoint picker by bare rule name across every
	// HTTPRoute on a gateway, so services sharing preset names get scheduled by
	// each other's endpoint picker.
	RouteRuleNamingUnique RouteRuleNaming = constants.RouteRuleNamingUnique
)

// maxRuleNameLength is the MaxLength of gwapiv1.SectionName.
const maxRuleNameLength = 253

// applyRouteRuleNaming hashes the service name rather than the model name, since
// canary members and other services in a namespace can share a model name.
// Unnamed rules stay unnamed: Istio already derives a per-route name for them.
// A name already ending in the suffix is kept, so a spec copied from a rendered
// route is not suffixed twice; a spec holding both "x" and "x-<suffix>" then
// ends up with two rules of that name, as can two over-long names that are equal
// once cut to fit.
func applyRouteRuleNaming(rules []gwapiv1.HTTPRouteRule, llmSvc *v1alpha2.LLMInferenceService, cfg *Config) error {
	naming, source := routeRuleNamingFor(llmSvc, cfg)
	switch naming {
	case RouteRuleNamingPreset, "":
		// "" only in Configs built without NewIngressConfig, which defaults it.
		return nil
	case RouteRuleNamingUnique:
		suffix := "-" + utils.ShortHash(llmSvc.GetNamespace()+"/"+llmSvc.GetName())
		for i := range rules {
			if name := ptr.Deref(rules[i].Name, ""); name != "" {
				rules[i].Name = ptr.To(uniqueRuleName(name, suffix))
			}
		}
		return nil
	default:
		// A ConfigMap typo, or an annotation that bypassed the webhook. Falling
		// back to preset would quietly bring the collision back, so the route is
		// left as it is.
		return fmt.Errorf("%w: unsupported routeRuleNaming %q from %s: supported values are %q and %q",
			ErrPreconditionNotMet, naming, source, RouteRuleNamingPreset, RouteRuleNamingUnique)
	}
}

// uniqueRuleName drops trailing dots and hyphens after a cut, as a dot right
// before the suffix's hyphen breaks the SectionName pattern.
func uniqueRuleName(name gwapiv1.SectionName, suffix string) gwapiv1.SectionName {
	stem := string(name)
	if strings.HasSuffix(stem, suffix) {
		return name
	}
	if budget := maxRuleNameLength - len(suffix); len(stem) > budget {
		stem = strings.TrimRight(stem[:budget], ".-")
	}
	return gwapiv1.SectionName(stem + suffix)
}

// routeRuleNamingFor normalizes the annotation because the webhook accepts it
// case-insensitively and the API server stores it as written. The source goes
// into the error, so an operator knows where to fix a rejected value.
func routeRuleNamingFor(llmSvc *v1alpha2.LLMInferenceService, cfg *Config) (RouteRuleNaming, string) {
	if v := strings.TrimSpace(llmSvc.Spec.Annotations[AnnotationRouteRuleNaming]); v != "" {
		return RouteRuleNaming(strings.ToLower(v)), "annotation " + AnnotationRouteRuleNaming
	}
	return cfg.RouteRuleNaming, "the " + constants.InferenceServiceConfigMapName + " ConfigMap"
}
