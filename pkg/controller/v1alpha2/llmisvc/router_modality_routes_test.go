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

package llmisvc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"

	kservetesting "github.com/kserve/kserve/pkg/testing"
)

// The default router route preset must expose the modality endpoints served by
// vLLM-Omni (TTS + image generation) through the InferencePool, mirroring the
// llm-d diffusion-serving multi-endpoint HTTPRoute paths with a 600s timeout.
// Without these rules audio/image requests fall through to the catch-all
// Service rule and bypass EPP scheduling entirely.
func TestRouterRoutePresetExposesModalityEndpoints(t *testing.T) {
	path := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvcconfig", "config-llm-router-route.yaml")
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	content := string(data)

	for _, want := range []string{
		"v1/audio/speech",
		"v1/audio/transcriptions",
		"v1/images/generations",
	} {
		require.Contains(t, content, want, "router route preset must route %s to the InferencePool", want)
	}

	// The preset embeds a Gateway API HTTPRoute, which caps rules at 16, matches
	// per rule at 64, and total matches at 128. Exceeding any of them rejects
	// the preset at CRD validation time; guard it here without needing envtest.
	preset := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, yaml.Unmarshal(data, preset))
	rules := preset.Spec.Router.Route.HTTP.Spec.Rules
	require.LessOrEqual(t, len(rules), 16, "HTTPRoute rules exceed Gateway API max")
	total := 0
	for _, r := range rules {
		require.LessOrEqual(t, len(r.Matches), 64, "rule %q matches exceed Gateway API max", r.Name)
		total += len(r.Matches)
	}
	require.LessOrEqual(t, total, 128, "HTTPRoute total matches exceed Gateway API max")
}

// Non-omni services must not carry the modality rules their runtime can never
// serve: the rules spend Gateway API rule and match budget that LoRA adapter
// expansion needs (each adapter duplicates every model-routing match), and
// text services get no benefit from them. The controller strips them at render
// time until the route preset itself is restructured (kserve/kserve#6360).
func TestStripModalityRouteRules(t *testing.T) {
	poolRule := func(name string, matches ...gwapiv1.HTTPRouteMatch) gwapiv1.HTTPRouteRule {
		return gwapiv1.HTTPRouteRule{
			Name: ptr.To(gwapiv1.SectionName(name)),
			BackendRefs: []gwapiv1.HTTPBackendRef{{
				BackendRef: gwapiv1.BackendRef{
					BackendObjectReference: gwapiv1.BackendObjectReference{
						Kind: ptr.To(gwapiv1.Kind("InferencePool")),
						Name: "svc-inference-pool",
					},
				},
			}},
			Matches: matches,
		}
	}
	prefix := func(value string) gwapiv1.HTTPRouteMatch {
		return gwapiv1.HTTPRouteMatch{Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
			Value: ptr.To(value),
		}}
	}
	exact := func(value string) gwapiv1.HTTPRouteMatch {
		return gwapiv1.HTTPRouteMatch{Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(gwapiv1.PathMatchExact),
			Value: ptr.To(value),
		}}
	}

	tests := []struct {
		name        string
		rules       []gwapiv1.HTTPRouteRule
		want        []string // kept rule names, in order
		wantMatches map[string]int
	}{
		{
			name: "namespaced and publisher modality prefix rules are dropped",
			rules: []gwapiv1.HTTPRouteRule{
				poolRule("v1-completions-path", prefix("/ns/svc/v1/completions")),
				poolRule("v1-audio-path", prefix("/ns/svc/v1/audio")),
				poolRule("v1-images-path", prefix("/ns/svc/v1/images")),
				poolRule("v1-audio-publisher-path", prefix("/publishers/ns/models/m/v1/audio")),
				poolRule("v1-images-publisher-path", prefix("/publishers/ns/models/m/v1/images")),
			},
			want: []string{"v1-completions-path"},
		},
		{
			name: "modality matches are pruned from the mixed model-routing rule",
			rules: []gwapiv1.HTTPRouteRule{
				poolRule("v1-model-routing",
					exact("/v1/completions"),
					exact("/v1/audio/speech"),
					exact("/v1/audio/speech/"),
					exact("/v1/audio/transcriptions"),
					exact("/v1/images/generations"),
					exact("/v1/chat/completions"),
				),
			},
			want:        []string{"v1-model-routing"},
			wantMatches: map[string]int{"v1-model-routing": 2},
		},
		{
			name: "catch-all and header-only rules survive untouched",
			rules: []gwapiv1.HTTPRouteRule{
				poolRule("v1-catch-all-path", prefix("/ns/svc")),
				poolRule("v1-publisher-path-catch-all", prefix("/publishers/ns/models/m")),
				{
					Name: ptr.To(gwapiv1.SectionName("v1-catch-all-model-routing")),
					Matches: []gwapiv1.HTTPRouteMatch{{Headers: []gwapiv1.HTTPHeaderMatch{{
						Name:  "x-gateway-model-name",
						Value: "publishers/ns/models/m",
					}}}},
				},
			},
			want: []string{"v1-catch-all-path", "v1-publisher-path-catch-all", "v1-catch-all-model-routing"},
		},
		{
			name: "lookalike paths are not modality paths",
			rules: []gwapiv1.HTTPRouteRule{
				poolRule("v1-completions-path", prefix("/ns/svc/v1/completions")),
				poolRule("custom", prefix("/ns/svc/v1/audiovisual")),
				poolRule("custom2", exact("/v1/audiostream")),
			},
			want:        []string{"v1-completions-path", "custom", "custom2"},
			wantMatches: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmisvc.StripModalityRouteRules(tt.rules)

			var gotNames []string
			for _, r := range got {
				gotNames = append(gotNames, string(ptr.Deref(r.Name, "")))
				if want, ok := tt.wantMatches[string(ptr.Deref(r.Name, ""))]; ok {
					require.Len(t, r.Matches, want, "rule %q", ptr.Deref(r.Name, ""))
				}
				for _, m := range r.Matches {
					if m.Path != nil {
						require.False(t, llmisvc.IsModalityEndpointPathForTest(ptr.Deref(m.Path.Value, "")),
							"modality path %q survived the strip", ptr.Deref(m.Path.Value, ""))
					}
				}
			}
			require.Equal(t, tt.want, gotNames)
		})
	}
}
