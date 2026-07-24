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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func TestSemanticHTTPRouteIsEqual_Labels(t *testing.T) {
	baseLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentRouter,
		constants.KubernetesAppNameLabelKey:   "my-llm",
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}

	withGroup := func(labels map[string]string, group string) map[string]string {
		out := make(map[string]string, len(labels)+1)
		for k, v := range labels {
			out[k] = v
		}
		out[constants.LLMRoutingGroupLabelKey] = group
		return out
	}

	withExtra := func(labels map[string]string, key, val string) map[string]string {
		out := make(map[string]string, len(labels)+1)
		for k, v := range labels {
			out[k] = v
		}
		out[key] = val
		return out
	}

	route := func(labels map[string]string) *gwapiv1.HTTPRoute {
		return &gwapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
		}
	}

	tests := []struct {
		name     string
		expected *gwapiv1.HTTPRoute
		current  *gwapiv1.HTTPRoute
		wantEq   bool
	}{
		{
			name:     "extra non-controller label on current - no update",
			expected: route(baseLabels),
			current:  route(withExtra(baseLabels, "app.kubernetes.io/managed-by", "argocd")),
			wantEq:   true,
		},
		{
			name:     "stale routing-group on current, absent in expected - update",
			expected: route(baseLabels),
			current:  route(withGroup(baseLabels, "old-group")),
			wantEq:   false,
		},
		{
			name:     "routing-group value changed - update",
			expected: route(withGroup(baseLabels, "new-group")),
			current:  route(withGroup(baseLabels, "old-group")),
			wantEq:   false,
		},
		{
			name:     "expected has routing-group, current missing - update",
			expected: route(withGroup(baseLabels, "llama-70b")),
			current:  route(baseLabels),
			wantEq:   false,
		},
		{
			name:     "identical labels - no update",
			expected: route(withGroup(baseLabels, "llama-70b")),
			current:  route(withGroup(baseLabels, "llama-70b")),
			wantEq:   true,
		},
		{
			name:     "no labels on either - no update",
			expected: route(nil),
			current:  route(nil),
			wantEq:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEq, semanticHTTPRouteIsEqual(tt.expected, tt.current))
		})
	}
}

func TestSemanticHTTPRouteIsEqual_GroupedRouteRules(t *testing.T) {
	groupLabels := map[string]string{
		constants.LLMRoutingGroupLabelKey: "llama-70b",
	}

	makeRule := func(path string, weight int32) gwapiv1.HTTPRouteRule {
		return gwapiv1.HTTPRouteRule{
			Matches: []gwapiv1.HTTPRouteMatch{
				{Path: &gwapiv1.HTTPPathMatch{Value: &path}},
			},
			BackendRefs: []gwapiv1.HTTPBackendRef{
				{BackendRef: gwapiv1.BackendRef{Weight: &weight}},
			},
		}
	}

	tests := []struct {
		name     string
		expected *gwapiv1.HTTPRoute
		current  *gwapiv1.HTTPRoute
		wantEq   bool
	}{
		{
			name: "grouped route - identical rules - no update",
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: groupLabels},
				Spec:       gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{makeRule("/v1/chat", 9)}},
			},
			current: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: groupLabels},
				Spec:       gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{makeRule("/v1/chat", 9)}},
			},
			wantEq: true,
		},
		{
			name: "grouped route - stale backendRef in current (extra rule) - update",
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: groupLabels},
				Spec:       gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{makeRule("/v1/chat", 9)}},
			},
			current: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: groupLabels},
				Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{
					makeRule("/v1/chat", 9),
					makeRule("/v1/chat", 1),
				}},
			},
			wantEq: false,
		},
		{
			name: "grouped route - weight changed - update",
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: groupLabels},
				Spec:       gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{makeRule("/v1/chat", 5)}},
			},
			current: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: groupLabels},
				Spec:       gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{makeRule("/v1/chat", 9)}},
			},
			wantEq: false,
		},
		{
			name: "non-grouped route - extra rule tolerated by DeepDerivative - no update",
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}},
				Spec:       gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{makeRule("/v1/chat", 9)}},
			},
			current: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}},
				Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{
					makeRule("/v1/chat", 9),
					makeRule("/v1/chat", 1),
				}},
			},
			wantEq: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEq, semanticHTTPRouteIsEqual(tt.expected, tt.current))
		})
	}
}
