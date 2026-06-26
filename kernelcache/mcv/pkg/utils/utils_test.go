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

package utils

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePathExists(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_file_exists")
	require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o640))
	defer os.Remove(tmpFile)

	exists, err := FilePathExists(tmpFile)
	assert.NoError(t, err)
	assert.True(t, exists)

	notExists, err := FilePathExists("/nonexistent/path/123")
	assert.NoError(t, err)
	assert.False(t, notExists)
}

func TestHasApp(t *testing.T) {
	assert.True(t, HasApp("ls"))
	assert.False(t, HasApp("fake_app_that_does_not_exist"))
}

func TestSanitizeGroupJSON(t *testing.T) {
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "test.json")

	originalJSON := `{
  "child_paths": {
    "one": "/home/user/.triton/cache/a",
    "two": "/tmp/.triton/cache/b"
  }
}`

	expectedSanitized := map[string]string{
		"one": ".triton/cache/a",
		"two": ".triton/cache/b",
	}

	err := os.WriteFile(testFile, []byte(originalJSON), 0o640)
	assert.NoError(t, err)

	err = SanitizeGroupJSON(testFile)
	assert.NoError(t, err)

	// Read back and verify
	sanitized := map[string]map[string]string{}
	content, err := os.ReadFile(filepath.Clean(testFile))
	assert.NoError(t, err)
	assert.NoError(t, json.Unmarshal(content, &sanitized))
	assert.Equal(t, expectedSanitized, sanitized["child_paths"])
}

func TestCleanupMCVDirs(t *testing.T) {
	testDir := filepath.Join(os.TempDir(), "mcv_test_cleanup")

	require.NoError(t, os.MkdirAll(testDir, 0o750))

	err := CleanupMCVDirs(context.Background(), testDir)
	assert.NoError(t, err)

	_, statErr := os.Stat(testDir)
	assert.True(t, os.IsNotExist(statErr))
}
