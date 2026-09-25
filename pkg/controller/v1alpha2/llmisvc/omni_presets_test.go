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
			want: []string{"--omni", "kserve-config-llm-omni-template"},
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
