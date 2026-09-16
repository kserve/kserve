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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestLoraRegexHeaderValue(t *testing.T) {
	t.Parallel()
	t.Run("sorted, factored, anchored", func(t *testing.T) {
		got := loraRegexHeaderValue("ns", "base", []string{"b-adapter", "a-adapter"})
		assert.Equal(t, "^publishers/ns/models/(base|a-adapter|b-adapter)$", got)
	})

	t.Run("deterministic regardless of input order", func(t *testing.T) {
		a := loraRegexHeaderValue("ns", "base", []string{"x", "y", "z"})
		b := loraRegexHeaderValue("ns", "base", []string{"z", "x", "y"})
		assert.Equal(t, a, b)
	})

	t.Run("duplicate adapters collapse", func(t *testing.T) {
		got := loraRegexHeaderValue("ns", "base", []string{"a", "a", "a"})
		assert.Equal(t, "^publishers/ns/models/(base|a)$", got)
	})

	t.Run("base model appears exactly once", func(t *testing.T) {
		got := loraRegexHeaderValue("ns", "base", []string{"base", "a"})
		assert.Equal(t, "^publishers/ns/models/(base|a)$", got)
	})

	t.Run("metacharacters escaped in namespace, base, and adapters", func(t *testing.T) {
		got := loraRegexHeaderValue("ns.prod", "org/base.v1+fp16", []string{"a|b", "c(d)", "e[f]", "g{h}", "i*j?k", "l^m$n", `o\p`})
		re := regexp.MustCompile(got)

		for _, name := range []string{"org/base.v1+fp16", "a|b", "c(d)", "e[f]", "g{h}", "i*j?k", "l^m$n", `o\p`} {
			assert.True(t, re.MatchString("publishers/ns.prod/models/"+name), "expected %q to match", name)
		}
		// The escaped dot must not match an arbitrary character.
		assert.False(t, re.MatchString("publishers/nsXprod/models/org/base.v1+fp16"))
		assert.False(t, re.MatchString("publishers/ns.prod/models/org/baseXv1+fp16"))
		// Alternation metacharacters in names must not split alternatives.
		assert.False(t, re.MatchString("publishers/ns.prod/models/a"))
		assert.False(t, re.MatchString("publishers/ns.prod/models/b"))
	})

	t.Run("anchored full match rejects prefix, suffix, and embedded identities", func(t *testing.T) {
		re := regexp.MustCompile(loraRegexHeaderValue("ns", "base", []string{"adapter-1"}))

		assert.True(t, re.MatchString("publishers/ns/models/base"))
		assert.True(t, re.MatchString("publishers/ns/models/adapter-1"))

		for _, miss := range []string{
			"publishers/ns/models/adapter",      // prefix of a valid name
			"publishers/ns/models/adapter-12",   // valid name is a prefix
			"publishers/ns/models/xadapter-1",   // embedded
			"publishers/other/models/adapter-1", // cross-namespace
			"xpublishers/ns/models/adapter-1",   // leading garbage
			"publishers/ns/models/adapter-1x",   // trailing garbage
			"publishers/ns/models/",             // empty name
			"publishers/ns/models/base adapter-1",
		} {
			assert.False(t, re.MatchString(miss), "expected %q to miss", miss)
		}
	})
}

func regexHeaderMatchValue(t *testing.T, match gwapiv1.HTTPRouteMatch) string {
	t.Helper()
	require.Len(t, match.Headers, 1)
	require.Equal(t, gwapiv1.HeaderMatchRegularExpression, ptr.Deref(match.Headers[0].Type, gwapiv1.HeaderMatchExact))
	return match.Headers[0].Value
}

func TestApplyLoRARegexMatches(t *testing.T) {
	t.Parallel()
	const base = "publishers/ns/models/base-model"
	adapterNames := []string{"adapter-b", "adapter-a"}
	wantPattern := "^publishers/ns/models/(base-model|adapter-a|adapter-b)$"

	t.Run("rewrites every recognized model-routing match", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			ruleWithMatches(
				exactPathWithHeaderMatch("/v1/completions", base),
				exactPathWithHeaderMatch("/v1/completions/", base),
			),
			ruleWithMatches(pathPrefixMatch("/ns/name/v1/completions")),
			ruleWithMatches(headerOnlyMatch(base)),
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName))

		assert.Equal(t, wantPattern, regexHeaderMatchValue(t, rules[0].Matches[0]))
		assert.Equal(t, wantPattern, regexHeaderMatchValue(t, rules[0].Matches[1]))
		// Path matches on transformed rules stay intact.
		assert.Equal(t, "/v1/completions", *rules[0].Matches[0].Path.Value)
		// No match duplication under regex.
		assert.Len(t, rules[0].Matches, 2)
		// Path-only rules untouched.
		assert.Nil(t, rules[1].Matches[0].Headers)
		// Header-only catch-all rule transformed.
		assert.Equal(t, wantPattern, regexHeaderMatchValue(t, rules[2].Matches[0]))
	})

	t.Run("unrelated headers are never rewritten", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{
					{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: "X-Custom", Value: "unrelated"},
				},
			}}},
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName))

		assert.Equal(t, "unrelated", rules[0].Matches[0].Headers[0].Value)
		assert.Equal(t, gwapiv1.HeaderMatchExact, *rules[0].Matches[0].Headers[0].Type)
	})

	t.Run("unrelated header alongside a recognized one is preserved", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{
					{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: gwapiv1.HTTPHeaderName(headerName), Value: base},
					{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: "X-Custom", Value: "unrelated"},
				},
			}}},
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName))

		assert.Equal(t, wantPattern, rules[0].Matches[0].Headers[0].Value)
		assert.Equal(t, gwapiv1.HeaderMatchRegularExpression, *rules[0].Matches[0].Headers[0].Type)
		assert.Equal(t, "unrelated", rules[0].Matches[0].Headers[1].Value)
		assert.Equal(t, gwapiv1.HeaderMatchExact, *rules[0].Matches[0].Headers[1].Type)
	})

	t.Run("nil header type is treated as Exact", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{
					{Name: gwapiv1.HTTPHeaderName(headerName), Value: base},
				},
			}}},
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName))
		assert.Equal(t, wantPattern, rules[0].Matches[0].Headers[0].Value)
	})

	t.Run("case-variant header names are recognized and transformed", func(t *testing.T) {
		// HTTP header-name matching is case-insensitive in Gateway API, so a
		// lowercase spelling of the configured header is the same header.
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{
					{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: "x-gateway-model-name", Value: base},
				},
			}}},
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName))
		assert.Equal(t, wantPattern, rules[0].Matches[0].Headers[0].Value)
		assert.Equal(t, gwapiv1.HeaderMatchRegularExpression, *rules[0].Matches[0].Headers[0].Type)
		// The author's spelling of the name is preserved.
		assert.Equal(t, gwapiv1.HTTPHeaderName("x-gateway-model-name"), rules[0].Matches[0].Headers[0].Name)
	})

	t.Run("case-variant handcrafted regex fails structural validation", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{{
					Type:  ptr.To(gwapiv1.HeaderMatchRegularExpression),
					Name:  "x-gateway-model-name",
					Value: "^publishers/other-ns/models/.*$",
				}},
			}}},
		}

		err := applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName)
		assert.ErrorContains(t, err, "cannot be applied")
	})

	t.Run("duplicate case-equivalent routing headers fail structural validation", func(t *testing.T) {
		// Gateway API only evaluates the first equivalent header name; a second
		// one would be silently dead, so the match is rejected outright.
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{
					{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: gwapiv1.HTTPHeaderName(headerName), Value: base},
					{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: "x-gateway-model-name", Value: base},
				},
			}}},
		}

		err := applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName)
		require.ErrorContains(t, err, "case-equivalent")
	})

	t.Run("validation failure leaves every match untouched (no partial rewrite)", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			ruleWithMatches(
				exactPathWithHeaderMatch("/v1/completions", base),
				headerOnlyMatch("publishers/ns/models/rogue"),
			),
		}

		err := applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName)
		require.Error(t, err)
		// The valid first match must not have been rewritten before the
		// invalid second match was discovered.
		assert.Equal(t, base, rules[0].Matches[0].Headers[0].Value)
		assert.Equal(t, gwapiv1.HeaderMatchExact, *rules[0].Matches[0].Headers[0].Type)
	})

	t.Run("model-routing match with unexpected value fails structural validation", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{
				headerOnlyMatch("publishers/ns/models/some-other-model"),
			}},
		}

		err := applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName)
		require.Error(t, err)
		assert.ErrorContains(t, err, "cannot be applied")
		// The unrecognized match is not silently overwritten.
		assert.Equal(t, "publishers/ns/models/some-other-model", rules[0].Matches[0].Headers[0].Value)
	})

	t.Run("handcrafted regex match fails structural validation", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			{Matches: []gwapiv1.HTTPRouteMatch{{
				Headers: []gwapiv1.HTTPHeaderMatch{{
					Type:  ptr.To(gwapiv1.HeaderMatchRegularExpression),
					Name:  gwapiv1.HTTPHeaderName(headerName),
					Value: "^publishers/ns/models/.*$",
				}},
			}}},
		}

		err := applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, headerName)
		assert.ErrorContains(t, err, "cannot be applied")
	})

	t.Run("no adapters is a no-op", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			ruleWithMatches(exactPathWithHeaderMatch("/v1/completions", base)),
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", nil, headerName))

		assert.Equal(t, base, rules[0].Matches[0].Headers[0].Value)
		assert.Equal(t, gwapiv1.HeaderMatchExact, *rules[0].Matches[0].Headers[0].Type)
	})

	t.Run("empty header name is a no-op", func(t *testing.T) {
		rules := []gwapiv1.HTTPRouteRule{
			ruleWithMatches(exactPathWithHeaderMatch("/v1/completions", base)),
		}

		require.NoError(t, applyLoRARegexMatches(rules, "ns", "base-model", adapterNames, ""))
		assert.Equal(t, base, rules[0].Matches[0].Headers[0].Value)
	})
}

func TestLoraAdapterNames(t *testing.T) {
	t.Parallel()
	names := loraAdapterNames([]v1alpha2.LLMModelSpec{
		{Name: ptr.To("b")},
		{Name: nil},
		{Name: ptr.To("")}, // an empty alternative would match the empty identity
		{Name: ptr.To("a")},
	})
	assert.Equal(t, []string{"b", "a"}, names, "unnamed and empty-named entries are skipped, order is preserved")
	assert.Empty(t, loraAdapterNames(nil))
}

// TestApplyLoRAModelRouting covers the strategy gate: which strategy runs, and
// that a strategy value is only ever consulted when there are adapters to
// expand and a model-routing match to expand them into.
func TestApplyLoRAModelRouting(t *testing.T) {
	t.Parallel()
	const base = "publishers/ns/models/base-model"
	adapters := &v1alpha2.LoRASpec{Adapters: []v1alpha2.LLMModelSpec{{Name: ptr.To("adapter-b")}, {Name: ptr.To("adapter-a")}}}
	unnamed := &v1alpha2.LoRASpec{Adapters: []v1alpha2.LLMModelSpec{{Name: nil}, {Name: ptr.To("")}}}
	modelRule := func() []gwapiv1.HTTPRouteRule {
		return []gwapiv1.HTTPRouteRule{ruleWithMatches(exactPathWithHeaderMatch("/v1/completions", base))}
	}

	for _, tt := range []struct {
		name        string
		strategy    LoRAModelRoutingStrategy
		header      string
		lora        *v1alpha2.LoRASpec
		rules       []gwapiv1.HTTPRouteRule
		annotations map[string]string
		wantErr     bool
		wantErrFrom string
		wantMatches int
		wantType    gwapiv1.HeaderMatchType
	}{
		{name: "regex collapses adapters into one match", strategy: LoRAModelRoutingStrategyRegex, header: headerName, lora: adapters, rules: modelRule(), wantMatches: 1, wantType: gwapiv1.HeaderMatchRegularExpression},
		{name: "exact expands one match per adapter", strategy: LoRAModelRoutingStrategyExact, header: headerName, lora: adapters, rules: modelRule(), wantMatches: 3, wantType: gwapiv1.HeaderMatchExact},
		{name: "zero value behaves as exact", strategy: "", header: headerName, lora: adapters, rules: modelRule(), wantMatches: 3, wantType: gwapiv1.HeaderMatchExact},
		{name: "unsupported strategy is a precondition failure", strategy: "bogus", header: headerName, lora: adapters, rules: modelRule(), wantErr: true},
		{name: "unsupported strategy is ignored on a path-only route", strategy: "bogus", header: headerName, lora: adapters, rules: []gwapiv1.HTTPRouteRule{ruleWithMatches(pathPrefixMatch("/ns/svc/v1/completions"))}, wantMatches: 1},
		{name: "unsupported strategy is ignored without LoRA", strategy: "bogus", header: headerName, lora: nil, rules: modelRule(), wantMatches: 1, wantType: gwapiv1.HeaderMatchExact},
		{name: "unsupported strategy is ignored when no adapter is named", strategy: "bogus", header: headerName, lora: unnamed, rules: modelRule(), wantMatches: 1, wantType: gwapiv1.HeaderMatchExact},
		{name: "unconfigured header disables expansion", strategy: LoRAModelRoutingStrategyRegex, header: "", lora: adapters, rules: modelRule(), wantMatches: 1, wantType: gwapiv1.HeaderMatchExact},
		{name: "regex is ignored without LoRA", strategy: LoRAModelRoutingStrategyRegex, header: headerName, lora: nil, rules: modelRule(), wantMatches: 1, wantType: gwapiv1.HeaderMatchExact},
		{name: "annotation overrides the cluster strategy", strategy: LoRAModelRoutingStrategyExact, header: headerName, lora: adapters, rules: modelRule(), annotations: map[string]string{AnnotationLoRAModelRoutingStrategy: "regex"}, wantMatches: 1, wantType: gwapiv1.HeaderMatchRegularExpression},
		{name: "annotation pins exact under a cluster regex", strategy: LoRAModelRoutingStrategyRegex, header: headerName, lora: adapters, rules: modelRule(), annotations: map[string]string{AnnotationLoRAModelRoutingStrategy: "exact"}, wantMatches: 3, wantType: gwapiv1.HeaderMatchExact},
		{name: "annotation value is trimmed and case-insensitive", strategy: LoRAModelRoutingStrategyExact, header: headerName, lora: adapters, rules: modelRule(), annotations: map[string]string{AnnotationLoRAModelRoutingStrategy: " Regex "}, wantMatches: 1, wantType: gwapiv1.HeaderMatchRegularExpression},
		{name: "empty annotation defers to the cluster strategy", strategy: LoRAModelRoutingStrategyRegex, header: headerName, lora: adapters, rules: modelRule(), annotations: map[string]string{AnnotationLoRAModelRoutingStrategy: ""}, wantMatches: 1, wantType: gwapiv1.HeaderMatchRegularExpression},
		{name: "unsupported annotation is a precondition failure naming the annotation", strategy: LoRAModelRoutingStrategyExact, header: headerName, lora: adapters, rules: modelRule(), annotations: map[string]string{AnnotationLoRAModelRoutingStrategy: "bogus"}, wantErr: true, wantErrFrom: "annotation " + AnnotationLoRAModelRoutingStrategy},
		{name: "unsupported cluster strategy names the ConfigMap", strategy: "bogus", header: headerName, lora: adapters, rules: modelRule(), wantErr: true, wantErrFrom: "ConfigMap"},
		{name: "regex rejects an unrecognized model match", strategy: LoRAModelRoutingStrategyRegex, header: headerName, lora: adapters, rules: []gwapiv1.HTTPRouteRule{ruleWithMatches(headerOnlyMatch("publishers/ns/models/other"))}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model:        v1alpha2.LLMModelSpec{Name: ptr.To("base-model"), LoRA: tt.lora},
					WorkloadSpec: v1alpha2.WorkloadSpec{Annotations: tt.annotations},
				},
			}
			cfg := &Config{LoRAModelRoutingStrategy: tt.strategy, ModelBasedRoutingHeaderName: tt.header}
			var before *gwapiv1.HTTPHeaderMatch
			if headers := tt.rules[0].Matches[0].Headers; len(headers) > 0 {
				before = ptr.To(headers[0])
			}

			err := applyLoRAModelRouting(tt.rules, svc, cfg)

			if tt.wantErr {
				require.ErrorIs(t, err, ErrPreconditionNotMet)
				if tt.wantErrFrom != "" {
					assert.ErrorContains(t, err, tt.wantErrFrom)
				}
				assert.Len(t, tt.rules[0].Matches, 1)
				assert.Equal(t, *before, tt.rules[0].Matches[0].Headers[0], "a failed transform must leave the route untouched")
				return
			}
			require.NoError(t, err)
			assert.Len(t, tt.rules[0].Matches, tt.wantMatches)
			if before != nil {
				assert.Equal(t, tt.wantType, ptr.Deref(tt.rules[0].Matches[0].Headers[0].Type, gwapiv1.HeaderMatchExact))
			}
		})
	}
}

// TestLoraRegexRealisticCapacityFixture binds the published 100-adapter
// capacity claim to the versioned realistic-v1 naming fixture. The golden pattern is
// produced by an independent generator (not the production renderer) and
// reviewed like any golden file; regenerating it is an explicit action.
func TestLoraRegexRealisticCapacityFixture(t *testing.T) {
	t.Parallel()
	const (
		fixtureNamespace = "lora-capacity"
		fixtureBaseModel = "llama-3-1-8b-instruct"
	)

	raw, err := os.ReadFile(filepath.Join("testdata", "lora-adapter-names-realistic-v1.txt"))
	require.NoError(t, err)
	names := strings.Fields(strings.TrimSpace(string(raw)))
	require.Len(t, names, 100, "the realistic-v1 fixture claims exactly 100 adapters")

	golden, err := os.ReadFile(filepath.Join("testdata", "lora-regex-realistic-v1.golden"))
	require.NoError(t, err)
	wantPattern := strings.TrimSuffix(string(golden), "\n")

	got := loraRegexHeaderValue(fixtureNamespace, fixtureBaseModel, names)
	assert.Equal(t, wantPattern, got, "rendered pattern must match the independently generated golden")

	// 100 adapters plus the base model: 101 model identities total.
	alternation := got[strings.Index(got, "(")+1 : strings.LastIndex(got, ")")]
	assert.Len(t, strings.Split(alternation, "|"), 101,
		"pattern must carry exactly 101 model-identity alternatives")
	re := regexp.MustCompile(got)
	assert.True(t, re.MatchString("publishers/"+fixtureNamespace+"/models/"+fixtureBaseModel))
	for _, name := range names {
		assert.True(t, re.MatchString("publishers/"+fixtureNamespace+"/models/"+name), "adapter %q must route", name)
	}
	assert.False(t, re.MatchString("publishers/"+fixtureNamespace+"/models/unknown-adapter"))
	assert.False(t, re.MatchString("publishers/other-ns/models/"+fixtureBaseModel))

	assert.LessOrEqual(t, utf8.RuneCountInString(got), 4096,
		"realistic-v1 fixture must fit the supported header-value budget (got %d runes)", utf8.RuneCountInString(got))
}

// TestLoraRegexEscapesEveryPrintableASCII checks each printable ASCII
// character, used as an adapter name, matches itself literally and
// nothing else.
func TestLoraRegexEscapesEveryPrintableASCII(t *testing.T) {
	t.Parallel()
	for b := byte(0x20); b < 0x7f; b++ {
		name := string([]byte{'a', b, 'z'})
		pattern := loraRegexHeaderValue("ns", "base", []string{name})
		re, err := regexp.Compile(pattern)
		require.NoError(t, err, "pattern must compile for adapter name %q", name)

		assert.True(t, re.MatchString("publishers/ns/models/"+name),
			"adapter %q must match its own identity", name)
		assert.True(t, re.MatchString("publishers/ns/models/base"))
		if name != "abz" {
			assert.False(t, re.MatchString("publishers/ns/models/abz"),
				"escaped %q must not match a different literal", name)
		}
		assert.False(t, re.MatchString("publishers/ns/models/"+name+"x"))
		assert.False(t, re.MatchString("publishers/ns/models/x"+name))
	}
}
