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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kservetesting "github.com/kserve/kserve/pkg/testing"
)

// TestTemplateFuncsAreDisjoint guards the split: ReplaceVariables installs both
// maps, so a name present in each would have the deprecated version silently win
// and the live one would never be called.
func TestTemplateFuncsAreDisjoint(t *testing.T) {
	for name := range deprecatedTemplateFuncs {
		assert.NotContains(t, templateFuncs, name,
			"%q is in both maps; the deprecated one wins and shadows the live implementation", name)
	}
}

// TestShippedPresetsDoNotUseDeprecatedFuncs is what makes deprecatedTemplateFuncs
// mean something. Entries there are frozen against the presets already pinned by
// running services; a shipped preset calling one would freeze it against new
// services too, and it could then never be retired.
func TestShippedPresetsDoNotUseDeprecatedFuncs(t *testing.T) {
	root := kservetesting.ProjectRoot()
	// Both copies: config/ is the source, but charts/ is what Helm users install,
	// and only a make target keeps them in step.
	sources := []string{
		filepath.Join(root, "config", "llmisvcconfig"),
		filepath.Join(root, "charts", "kserve-runtime-configs", "files", "llmisvcconfigs"),
	}

	var checked int
	for _, dir := range sources {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(filepath.Clean(path))
			require.NoError(t, err)
			checked++
			for name := range deprecatedTemplateFuncs {
				assert.NotContains(t, string(content), name,
					"%s calls the deprecated template function %q; use its replacement instead", path, name)
			}
		}
	}
	require.NotZero(t, checked, "no preset files were read - the assertions above proved nothing")
}
