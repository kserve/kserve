/*
Copyright 2025 The KServe Authors.

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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const headerName = "X-Gateway-Model-Name"

func exactPathWithHeaderMatch(path, headerValue string) gwapiv1.HTTPRouteMatch {
	return gwapiv1.HTTPRouteMatch{
		Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(gwapiv1.PathMatchExact),
			Value: ptr.To(path),
		},
		Headers: []gwapiv1.HTTPHeaderMatch{
			{
				Type:  ptr.To(gwapiv1.HeaderMatchExact),
				Name:  gwapiv1.HTTPHeaderName(headerName),
				Value: headerValue,
			},
		},
	}
}

func headerOnlyMatch(headerValue string) gwapiv1.HTTPRouteMatch {
	return gwapiv1.HTTPRouteMatch{
		Headers: []gwapiv1.HTTPHeaderMatch{
			{
				Type:  ptr.To(gwapiv1.HeaderMatchExact),
				Name:  gwapiv1.HTTPHeaderName(headerName),
				Value: headerValue,
			},
		},
	}
}

func pathPrefixMatch(path string) gwapiv1.HTTPRouteMatch {
	return gwapiv1.HTTPRouteMatch{
		Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
			Value: ptr.To(path),
		},
	}
}

func ruleWithMatches(matches ...gwapiv1.HTTPRouteMatch) gwapiv1.HTTPRouteRule {
	return gwapiv1.HTTPRouteRule{Matches: matches}
}

func TestExpandLoRAAdapterMatches(t *testing.T) {
	adapters := []v1alpha2.LLMModelSpec{
		{Name: ptr.To("adapter-a")},
		{Name: ptr.To("adapter-b")},
	}

	tests := []struct {
		name       string
		rules      []gwapiv1.HTTPRouteRule
		namespace  string
		adapters   []v1alpha2.LLMModelSpec
		headerName string
		wantFn     func(t *testing.T, rules []gwapiv1.HTTPRouteRule)
	}{
		{
			name: "expands model-routing matches for each adapter",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(
					exactPathWithHeaderMatch("/v1/completions","publishers/ns/models/base-model"),
					exactPathWithHeaderMatch("/v1/completions/","publishers/ns/models/base-model"),
				),
			},
			namespace:  "ns",
			adapters:   adapters,
			headerName: headerName,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				// 2 base matches + 2 matches × 2 adapters = 6 total
				// Order: base matches first, then for each base match: adapter-a, adapter-b
				assert.Len(t, rules[0].Matches, 6)
				// match[0] /v1/completions adapter-a (from base match 0)
				assert.Equal(t, "publishers/ns/models/adapter-a", rules[0].Matches[2].Headers[0].Value)
				assert.Equal(t, "/v1/completions", *rules[0].Matches[2].Path.Value)
				// match[1] /v1/completions adapter-b (from base match 0)
				assert.Equal(t, "publishers/ns/models/adapter-b", rules[0].Matches[3].Headers[0].Value)
				assert.Equal(t, "/v1/completions", *rules[0].Matches[3].Path.Value)
				// match[2] /v1/completions/ adapter-a (from base match 1)
				assert.Equal(t, "publishers/ns/models/adapter-a", rules[0].Matches[4].Headers[0].Value)
				assert.Equal(t, "/v1/completions/", *rules[0].Matches[4].Path.Value)
				// match[3] /v1/completions/ adapter-b (from base match 1)
				assert.Equal(t, "publishers/ns/models/adapter-b", rules[0].Matches[5].Headers[0].Value)
				assert.Equal(t, "/v1/completions/", *rules[0].Matches[5].Path.Value)
			},
		},
		{
			name: "no adapters - rules unchanged",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(
					exactPathWithHeaderMatch("/v1/completions","publishers/ns/models/base-model"),
				),
			},
			namespace:  "ns",
			adapters:   nil,
			headerName: headerName,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules[0].Matches, 1)
			},
		},
		{
			name: "empty header name - rules unchanged",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(
					exactPathWithHeaderMatch("/v1/completions","publishers/ns/models/base-model"),
				),
			},
			namespace:  "ns",
			adapters:   adapters,
			headerName: "",
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules[0].Matches, 1)
			},
		},
		{
			name: "path-only rules are untouched",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(pathPrefixMatch("/ns/name/v1/completions")),
				ruleWithMatches(
					exactPathWithHeaderMatch("/v1/completions","publishers/ns/models/base-model"),
				),
			},
			namespace:  "ns",
			adapters:   adapters,
			headerName: headerName,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules[0].Matches, 1, "path-only rule should be untouched")
				assert.Len(t, rules[1].Matches, 3, "model-routing rule should be expanded")
			},
		},
		{
			name: "catch-all header-only match gets adapter copies",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(
					headerOnlyMatch("publishers/ns/models/base-model"),
				),
			},
			namespace:  "ns",
			adapters:   adapters,
			headerName: headerName,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				// 1 base + 2 adapters = 3
				assert.Len(t, rules[0].Matches, 3)
				assert.Equal(t, "publishers/ns/models/adapter-a", rules[0].Matches[1].Headers[0].Value)
				assert.Equal(t, "publishers/ns/models/adapter-b", rules[0].Matches[2].Headers[0].Value)
			},
		},
		{
			name: "adapter with nil name is skipped",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(
					exactPathWithHeaderMatch("/v1/completions","publishers/ns/models/base-model"),
				),
			},
			namespace: "ns",
			adapters: []v1alpha2.LLMModelSpec{
				{Name: nil},
				{Name: ptr.To("valid-adapter")},
			},
			headerName: headerName,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				// 1 base + 1 valid adapter = 2
				assert.Len(t, rules[0].Matches, 2)
				assert.Equal(t, "publishers/ns/models/valid-adapter", rules[0].Matches[1].Headers[0].Value)
			},
		},
		{
			name: "uses correct namespace in header value",
			rules: []gwapiv1.HTTPRouteRule{
				ruleWithMatches(
					exactPathWithHeaderMatch("/v1/completions","publishers/prod/models/base-model"),
				),
			},
			namespace:  "prod",
			adapters:   []v1alpha2.LLMModelSpec{{Name: ptr.To("lora-1")}},
			headerName: headerName,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Equal(t, "publishers/prod/models/lora-1", rules[0].Matches[1].Headers[0].Value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandLoRAAdapterMatches(tt.rules, tt.namespace, tt.adapters, tt.headerName)
			tt.wantFn(t, tt.rules)
		})
	}
}
