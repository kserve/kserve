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
)

func TestParseModelBasedRoutingMode(t *testing.T) {
	tests := []struct {
		input string
		want  ModelBasedRoutingMode
	}{
		{"enabled", ModelBasedRoutingEnabled},
		{"forced", ModelBasedRoutingForced},
		{"disabled", ModelBasedRoutingDisabled},
		{"FORCED", ModelBasedRoutingForced},
		{"Disabled", ModelBasedRoutingDisabled},
		{"ENABLED", ModelBasedRoutingEnabled},
		{"unknown", ModelBasedRoutingEnabled},
		{"", ModelBasedRoutingEnabled},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseModelBasedRoutingMode(tt.input))
		})
	}
}

func TestStripModelBasedRoutingRules(t *testing.T) {
	const hdr = "X-Gateway-Model-Name"

	modelMatch := func(path, headerValue string) gwapiv1.HTTPRouteMatch {
		return gwapiv1.HTTPRouteMatch{
			Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchExact),
				Value: ptr.To(path),
			},
			Headers: []gwapiv1.HTTPHeaderMatch{
				{
					Type:  ptr.To(gwapiv1.HeaderMatchExact),
					Name:  hdr,
					Value: headerValue,
				},
			},
		}
	}

	pathOnlyMatch := func(path string) gwapiv1.HTTPRouteMatch {
		return gwapiv1.HTTPRouteMatch{
			Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
				Value: ptr.To(path),
			},
		}
	}

	tests := []struct {
		name       string
		rules      []gwapiv1.HTTPRouteRule
		headerName string
		wantFn     func(t *testing.T, rules []gwapiv1.HTTPRouteRule)
	}{
		{
			name: "strips consolidated model-routing rule with all endpoint matches",
			rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{
					modelMatch("/v1/completions", "publishers/ns/models/m1"),
					modelMatch("/v1/completions/", "publishers/ns/models/m1"),
					modelMatch("/v1/chat/completions", "publishers/ns/models/m1"),
					modelMatch("/v1/chat/completions/", "publishers/ns/models/m1"),
					modelMatch("/v1/responses", "publishers/ns/models/m1"),
					modelMatch("/v1/responses/", "publishers/ns/models/m1"),
					modelMatch("/v1/messages", "publishers/ns/models/m1"),
					modelMatch("/v1/messages/", "publishers/ns/models/m1"),
				}},
			},
			headerName: hdr,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Empty(t, rules)
			},
		},
		{
			name: "keeps path-only rules untouched",
			rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{
					pathOnlyMatch("/ns/name/v1/completions"),
				}},
			},
			headerName: hdr,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules, 1)
				assert.Len(t, rules[0].Matches, 1)
			},
		},
		{
			name: "mixed rule: removes model-routing matches, keeps path matches",
			rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{
					pathOnlyMatch("/ns/name/v1/completions"),
					modelMatch("/v1/completions", "publishers/ns/models/m1"),
				}},
			},
			headerName: hdr,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules, 1)
				assert.Len(t, rules[0].Matches, 1)
				assert.Nil(t, rules[0].Matches[0].Headers)
			},
		},
		{
			name: "strips messages model-routing rule alongside other endpoints",
			rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{
					pathOnlyMatch("/ns/name/v1/messages"),
				}},
				{Matches: []gwapiv1.HTTPRouteMatch{
					modelMatch("/v1/messages", "publishers/ns/models/m1"),
					modelMatch("/v1/messages/", "publishers/ns/models/m1"),
				}},
			},
			headerName: hdr,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules, 1)
				assert.Len(t, rules[0].Matches, 1)
				assert.Nil(t, rules[0].Matches[0].Headers)
			},
		},
		{
			name: "empty header name: returns rules unchanged",
			rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{
					modelMatch("/v1/completions", "publishers/ns/models/m1"),
				}},
			},
			headerName: "",
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules, 1)
				assert.Len(t, rules[0].Matches, 1)
			},
		},
		{
			name: "no model-routing matches: returns all rules",
			rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{
					pathOnlyMatch("/ns/name/v1/completions"),
				}},
				{Matches: []gwapiv1.HTTPRouteMatch{
					pathOnlyMatch("/ns/name/health"),
				}},
			},
			headerName: hdr,
			wantFn: func(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
				assert.Len(t, rules, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripModelBasedRoutingRules(tt.rules, tt.headerName)
			tt.wantFn(t, got)
		})
	}
}
