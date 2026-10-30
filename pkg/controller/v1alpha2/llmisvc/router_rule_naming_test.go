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
	"cmp"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// sectionNamePattern is the schema pattern of gwapiv1.SectionName.
var sectionNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// The suffixes are literal sha256 prefixes of "<namespace>/<name>": a different
// hash would rename every rule of every unique route on upgrade.
func TestExpectedHTTPRouteRuleNaming(t *testing.T) {
	t.Parallel()
	named := func(name, path string) gwapiv1.HTTPRouteRule {
		rule := ruleWithMatches(pathPrefixMatch(path))
		rule.Name = ptr.To(gwapiv1.SectionName(name))
		return rule
	}
	rules := func(suffix string) []gwapiv1.HTTPRouteRule {
		return []gwapiv1.HTTPRouteRule{
			named("v1-completions-path"+suffix, "/ns/svc/v1/completions"),
			ruleWithMatches(pathPrefixMatch("/ns/svc/v1/models")),
			named("v1-chat-completions-publisher-path"+suffix, "/publishers/ns/models/m/v1/chat/completions"),
		}
	}
	presetRules := func() []gwapiv1.HTTPRouteRule { return rules("") }
	uniqueRules := func(hash string) []gwapiv1.HTTPRouteRule { return rules("-" + hash) }
	// 253 characters, the SectionName maximum; cutting it to make room for the
	// suffix lands right after the dot.
	longName := strings.Repeat("a", 243) + "." + strings.Repeat("b", 9)

	for _, tt := range []struct {
		name        string
		namespace   string // "ns" when empty
		svcName     string // "svc" when empty
		naming      RouteRuleNaming
		annotations map[string]string
		rules       []gwapiv1.HTTPRouteRule // presetRules() when nil
		want        []gwapiv1.HTTPRouteRule
		wantErrFrom string
	}{
		{name: "zero value keeps the preset names", want: presetRules()},
		{name: "preset keeps the preset names", naming: RouteRuleNamingPreset, want: presetRules()},
		{name: "unique from the ConfigMap suffixes named rules only", naming: RouteRuleNamingUnique, want: uniqueRules("76372607")},
		{name: "unique from the annotation", naming: RouteRuleNamingPreset, annotations: map[string]string{AnnotationRouteRuleNaming: "unique"}, want: uniqueRules("76372607")},
		{name: "annotation pins preset under a cluster-wide unique", naming: RouteRuleNamingUnique, annotations: map[string]string{AnnotationRouteRuleNaming: "preset"}, want: presetRules()},
		{name: "annotation value is trimmed and case-insensitive", naming: RouteRuleNamingPreset, annotations: map[string]string{AnnotationRouteRuleNaming: " Unique "}, want: uniqueRules("76372607")},
		{name: "empty annotation defers to the ConfigMap", naming: RouteRuleNamingUnique, annotations: map[string]string{AnnotationRouteRuleNaming: ""}, want: uniqueRules("76372607")},
		{name: "same service name in another namespace gets another suffix", namespace: "other-ns", naming: RouteRuleNamingUnique, want: uniqueRules("a39f440c")},
		{name: "another service in the same namespace gets another suffix", svcName: "other-svc", naming: RouteRuleNamingUnique, want: uniqueRules("6ac037dd")},
		{name: "a name already carrying the suffix is not suffixed again", naming: RouteRuleNamingUnique, rules: uniqueRules("76372607"), want: uniqueRules("76372607")},
		{
			name: "an empty name stays empty", naming: RouteRuleNamingUnique,
			rules: []gwapiv1.HTTPRouteRule{named("", "/ns/svc/v1/completions")},
			want:  []gwapiv1.HTTPRouteRule{named("", "/ns/svc/v1/completions")},
		},
		{
			name: "an over-long name is cut to fit a SectionName", naming: RouteRuleNamingUnique,
			rules: []gwapiv1.HTTPRouteRule{named(longName, "/ns/svc/v1/completions")},
			want:  []gwapiv1.HTTPRouteRule{named(strings.Repeat("a", 243)+"-76372607", "/ns/svc/v1/completions")},
		},
		{name: "unsupported annotation fails closed naming the annotation", naming: RouteRuleNamingPreset, annotations: map[string]string{AnnotationRouteRuleNaming: "bogus"}, wantErrFrom: "annotation " + AnnotationRouteRuleNaming},
		{name: "unsupported cluster value fails closed naming the ConfigMap", naming: "bogus", wantErrFrom: "ConfigMap"},
		{name: "annotation pins preset under an unsupported cluster value", naming: "bogus", annotations: map[string]string{AnnotationRouteRuleNaming: "preset"}, want: presetRules()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			render := func(rules []gwapiv1.HTTPRouteRule) (*gwapiv1.HTTPRoute, error) {
				svc := &v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: cmp.Or(tt.svcName, "svc"), Namespace: cmp.Or(tt.namespace, "ns")},
					Spec: v1alpha2.LLMInferenceServiceSpec{
						WorkloadSpec: v1alpha2.WorkloadSpec{Annotations: tt.annotations},
						Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{
							HTTP: &v1alpha2.HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{Rules: rules}},
						}},
					},
				}
				return (&LLMISVCReconciler{}).expectedHTTPRoute(t.Context(), svc, &Config{RouteRuleNaming: tt.naming})
			}
			input := tt.rules
			if input == nil {
				input = presetRules()
			}

			got, err := render(input)

			if tt.wantErrFrom != "" {
				require.ErrorIs(t, err, ErrPreconditionNotMet)
				assert.ErrorContains(t, err, tt.wantErrFrom)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Spec.Rules)
			for _, rule := range got.Spec.Rules {
				if name := string(ptr.Deref(rule.Name, "")); name != "" {
					assert.Regexp(t, sectionNamePattern, name)
					assert.LessOrEqual(t, len(name), 253, "SectionName maximum length")
				}
			}

			again, err := render(got.Spec.Rules)
			require.NoError(t, err)
			assert.Equal(t, got.Spec.Rules, again.Spec.Rules, "rendering a rendered route again must not rename it")
		})
	}
}
