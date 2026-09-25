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
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"

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
