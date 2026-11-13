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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	kservetesting "github.com/kserve/kserve/pkg/testing"
)

// TestVLLMTemplateForwardsContainerArgs runs the rendered startup command and checks
// the arguments received by vLLM. Before escaping "$@" for eval, the JSON cases
// fail because the second shell parse removes JSON quotes and splits values at
// spaces; the plain-argument cases still pass.
func TestVLLMTemplateForwardsContainerArgs(t *testing.T) {
	filePath := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvcconfig", "config-llm-template.yaml")
	data, err := os.ReadFile(filePath) // #nosec G304 -- fixed repository preset path, not user input
	require.NoError(t, err)
	binDir := t.TempDir()
	// Use an executable stub because the preset launches vLLM with exec.
	stub := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "0.23.0"
else
    printf '%s\0' "$@" > "$VLLM_TEST_ARGS"
fi
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "vllm"), []byte(stub), 0o700)) // #nosec G306 -- private executable stub in t.TempDir
	jsonArg := `{"rope_scaling": {"rope_type": "linear", "factor": 2}, "text": "hello world"}`
	cases := []struct {
		name string
		args []string
	}{
		{name: "plain", args: []string{"--max-model-len=512", "--gpu-memory-utilization", "0.8"}},
		{name: "nested JSON", args: []string{"--hf-overrides", jsonArg}},
		{name: "nested JSON with equals", args: []string{"--hf-overrides=" + jsonArg}},
	}
	for _, tls := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "HTTPS"}[tls], func(t *testing.T) {
			config := loadConfig(t, data, filePath)
			rendered, err := llmisvc.ReplaceVariables(llmisvc.LLMInferenceServiceSample(), config, &llmisvc.Config{EnableTLS: tls})
			require.NoError(t, err)
			container := findMainContainerForPresetTest(t, rendered.Spec.Template.Containers)

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					argsFile := filepath.Join(t.TempDir(), "argv")
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, container.Command[0], slices.Concat(container.Command[1:], tc.args)...) // #nosec G204 -- repository preset and fixed test input, not user data
					cmd.Env = append(os.Environ(),
						"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
						"VLLM_TEST_ARGS="+argsFile,
						"KSERVE_INFER_ROCE=false",
						"KSERVE_MODEL_ARGS=",
						"KSERVE_KV_TRANSFER_ARGS=",
						"VLLM_ADDITIONAL_ARGS=--seed 42",
					)
					output, err := cmd.CombinedOutput()
					require.NoError(t, err, "startup command failed:\n%s", output)
					captured, err := os.ReadFile(argsFile) // #nosec G304 -- path inside a test-owned temporary directory
					require.NoError(t, err)
					args := strings.Split(strings.TrimSuffix(string(captured), "\x00"), "\x00")
					// Check the additional flags and user args without coupling to preset defaults.
					start := slices.Index(args, "--seed")
					require.NotEqual(t, -1, start, "VLLM_ADDITIONAL_ARGS was not forwarded: %q", args)
					require.Equal(t, slices.Concat([]string{"--seed", "42"}, tc.args), args[start:])
				})
			}
		})
	}
}
