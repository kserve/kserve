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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestDisaggregatedSetRolloutScreenerCheck(t *testing.T) {
	const screener = `plugins:
- type: disaggregatedset-rollout-screener
  parameters:
    scope:
      labelSelector: "app.kubernetes.io/name=llm"
    revisionGating: {mode: max-role, requiredRoles: [prefill, decode]}
- type: decode-filter
`
	const noScreener = `plugins:
- type: decode-filter
`
	scheduler := func(args ...string) *appsv1.Deployment {
		return &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Args: args}},
		}}}}
	}
	pods := map[string]labels.Set{
		"decode":  {"app.kubernetes.io/name": "llm", "llm-d.ai/role": "decode"},
		"prefill": {"app.kubernetes.io/name": "llm", "llm-d.ai/role": "prefill"},
	}

	tests := []struct {
		name   string
		d      *appsv1.Deployment
		pods   map[string]labels.Set
		reason string
	}{
		{"screener over both roles", scheduler("--allow-experimental-plugins", "--config-text", screener), pods, ""},
		{"no DisaggregatedSet yet: nothing to select", scheduler("--allow-experimental-plugins", "--config-text", screener), nil, ""},
		{"flag set to true", scheduler("--allow-experimental-plugins=true", "--config-text", screener), pods, ""},
		{"no screener", scheduler("--allow-experimental-plugins", "--config-text", noScreener), pods, reasonScreenerMissing},
		{"no experimental flag", scheduler("--config-text", screener), pods, reasonExperimentalPluginsDisabled},
		{"flag turned off", scheduler("--allow-experimental-plugins", "--allow-experimental-plugins=false", "--config-text", screener), pods, reasonExperimentalPluginsDisabled},
		{"config from a file", scheduler("--allow-experimental-plugins", "--config-file", "/etc/epp/config.yaml"), pods, reasonConfigNotInspectable},
		{
			"scope misses a role", scheduler("--allow-experimental-plugins", "--config-text", screener),
			map[string]labels.Set{"decode": pods["decode"], "prefill": {"app.kubernetes.io/name": "other"}},
			reasonScreenerScopeMismatch,
		},
		{
			"scope does not parse", scheduler("--allow-experimental-plugins", "--config-text",
				"plugins:\n- type: disaggregatedset-rollout-screener\n  parameters: {scope: {labelSelector: \"a in (\"}}\n"),
			pods, reasonScreenerScopeMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := disaggregatedSetRolloutScreenerCheck(tt.d, tt.pods)
			assert.Equal(t, tt.reason, got.reason, got.message)
			if tt.reason != "" {
				assert.NotEmpty(t, got.message)
			}
		})
	}
}

func TestExperimentalPluginsAllowed(t *testing.T) {
	assert.True(t, experimentalPluginsAllowed([]string{"--allow-experimental-plugins"}))
	assert.True(t, experimentalPluginsAllowed([]string{"-allow-experimental-plugins=1"}))
	assert.False(t, experimentalPluginsAllowed(nil))
	assert.False(t, experimentalPluginsAllowed([]string{"--allow-experimental-plugins=false"}))
	assert.False(t, experimentalPluginsAllowed([]string{"--allow-experimental-plugins=maybe"}))
	assert.True(t, experimentalPluginsAllowed([]string{"--allow-experimental-plugins=false", "--allow-experimental-plugins"}), "the last one wins")
}
