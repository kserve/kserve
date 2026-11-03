/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package llmisvc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	kservetesting "github.com/kserve/kserve/pkg/testing"
)

// vLLM-Omni (TTS/TTI) services select the omni workload template and a
// load-only EPP profile: the text prefix-cache chain has no TokenizedRequest
// for audio/image inputs, so the default EPP preset would never score.
func TestOmniPresetsExist(t *testing.T) {
	for _, tc := range []struct {
		file string
		want []string
	}{
		{
			file: "config-llm-omni-template.yaml",
			want: []string{"docker.io/vllm/vllm-omni:v0.30.0", "kserve-config-llm-omni-template"},
		},
		{
			file: "config-llm-scheduler-eppconfig-modality.yaml",
			want: []string{"active-request-scorer", "max-score-picker"},
		},
	} {
		path := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvcconfig", tc.file)
		data, err := os.ReadFile(filepath.Clean(path))
		require.NoError(t, err, "missing omni preset %s", tc.file)
		for _, w := range tc.want {
			require.Contains(t, string(data), w, "preset %s must contain %q", tc.file, w)
		}
	}
}

// The omni template must stay a thin overlay on the base vLLM template: it
// swaps the image, startup probe and security context, while command, probes,
// volumes and the model-based routing model name are inherited. A `vllm serve`
// command in this file would fork the base template and let the two drift
// apart - exactly the duplication the overlay exists to avoid. The --omni flag
// is appended to the container args by the controller instead.
func TestOmniTemplateIsOverlayOnly(t *testing.T) {
	path := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvcconfig", "config-llm-omni-template.yaml")
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)

	overlay := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, yaml.Unmarshal(data, overlay))
	require.NotNil(t, overlay.Spec.Template)
	require.Len(t, overlay.Spec.Template.Containers, 1)

	main := overlay.Spec.Template.Containers[0]
	require.Empty(t, main.Command, "omni template must not fork the base template command")
	require.Empty(t, main.Args, "the --omni flag is injected into container args by the controller")
	require.Empty(t, main.Env, "container env is inherited from the base template")
	require.Empty(t, main.VolumeMounts, "volume mounts are inherited from the base template")
	require.Empty(t, overlay.Spec.Template.Volumes, "volumes are inherited from the base template")
	require.Empty(t, overlay.Spec.Annotations, "annotations (model-based routing) are inherited from the base template")
	require.NotEmpty(t, main.Image, "the overlay's one job that is not a probe: the engine image")
}

// injectOmniRuntimeArgs prepends --omni to the single-node workload's main
// container args for omni services, after the merge, so user-supplied args
// cannot drop it; non-omni specs are untouched.
func TestInjectOmniRuntimeArgs(t *testing.T) {
	mainWithArgs := func(args ...string) *v1alpha2.LLMInferenceServiceConfig {
		return &v1alpha2.LLMInferenceServiceConfig{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{{Name: "main", Args: args}},
					},
				},
			},
		}
	}

	t.Run("omni prepends the flag before user args", func(t *testing.T) {
		cfg := mainWithArgs("--my-flag")
		llmisvc.InjectOmniRuntimeArgs(cfg, true)
		require.Equal(t, []string{"--omni", "--my-flag"}, cfg.Spec.Template.Containers[0].Args)
	})

	t.Run("omni is idempotent", func(t *testing.T) {
		cfg := mainWithArgs("--omni", "--my-flag")
		llmisvc.InjectOmniRuntimeArgs(cfg, true)
		require.Equal(t, []string{"--omni", "--my-flag"}, cfg.Spec.Template.Containers[0].Args)
	})

	t.Run("non-omni is untouched", func(t *testing.T) {
		cfg := mainWithArgs("--my-flag")
		llmisvc.InjectOmniRuntimeArgs(cfg, false)
		require.Equal(t, []string{"--my-flag"}, cfg.Spec.Template.Containers[0].Args)
	})

	t.Run("missing template or main container is a no-op", func(t *testing.T) {
		empty := &v1alpha2.LLMInferenceServiceConfig{}
		llmisvc.InjectOmniRuntimeArgs(empty, true)
		require.Nil(t, empty.Spec.Template)

		noMain := &v1alpha2.LLMInferenceServiceConfig{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{{Name: "other"}},
					},
				},
			},
		}
		llmisvc.InjectOmniRuntimeArgs(noMain, true)
		require.Empty(t, noMain.Spec.Template.Containers[0].Args)
	})
}

// isOmniRuntime gates the omni template overlay, the --omni arg injection and
// the modality EPP preset on the well-known runtime name.
func TestIsOmniRuntime(t *testing.T) {
	require.False(t, llmisvc.IsOmniRuntimeForTest(nil))
	require.False(t, llmisvc.IsOmniRuntimeForTest(ptr.To("")))
	require.False(t, llmisvc.IsOmniRuntimeForTest(ptr.To("kserve-llm-sglang")))
	require.True(t, llmisvc.IsOmniRuntimeForTest(ptr.To("kserve-llm-omni")))
}
