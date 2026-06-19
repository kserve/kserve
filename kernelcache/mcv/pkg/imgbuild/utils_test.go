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

package imgbuild

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateDockerfile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "Dockerfile")

	err := GenerateDockerfile("myorg/myimage:1.0", "cacheLayer", "manifestLayer", outputPath)
	assert.NoError(t, err)

	content, err := os.ReadFile(filepath.Clean(outputPath))
	assert.NoError(t, err)
	assert.Contains(t, string(content), "FROM scratch")
	assert.Contains(t, string(content), "COPY \"./cacheLayer\" \"./cacheLayer\"")
	assert.Contains(t, string(content), "COPY \"./manifestLayer/manifest.json")
}

func TestCleanupDirs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "dummy.txt")
	err := os.WriteFile(testFile, []byte("dummy"), 0o640)
	assert.NoError(t, err)

	CleanupDirs(tmpDir)

	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err) || err != nil)
}

func TestCleanupWithTimeout(t *testing.T) {
	start := time.Now()
	err := CleanupWithTimeout()
	duration := time.Since(start)

	// should complete quickly unless CleanupMCVDirs is slow
	assert.NoError(t, err)
	assert.Less(t, duration.Milliseconds(), int64(5000))
}
