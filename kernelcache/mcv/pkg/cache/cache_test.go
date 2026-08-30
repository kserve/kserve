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

package cache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPathWithinBase(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		baseDir  string
		expected bool
	}{
		{
			name:     "valid path directly under base",
			filePath: "/tmp/cache/file.txt",
			baseDir:  "/tmp/cache",
			expected: true,
		},
		{
			name:     "valid path in subdirectory",
			filePath: "/tmp/cache/sub/dir/file.txt",
			baseDir:  "/tmp/cache",
			expected: true,
		},
		{
			name:     "path traversal with dot-dot",
			filePath: "/tmp/cache/../../etc/passwd",
			baseDir:  "/tmp/cache",
			expected: false,
		},
		{
			name:     "path traversal escaping base",
			filePath: "/tmp/cache/../other/file.txt",
			baseDir:  "/tmp/cache",
			expected: false,
		},
		{
			name:     "path equal to base directory",
			filePath: "/tmp/cache",
			baseDir:  "/tmp/cache",
			expected: true,
		},
		{
			name:     "similar prefix but different directory",
			filePath: "/tmp/cache-evil/file.txt",
			baseDir:  "/tmp/cache",
			expected: false,
		},
		{
			name:     "deeply nested traversal",
			filePath: "/tmp/cache/a/b/c/../../../../etc/cron.d/backdoor",
			baseDir:  "/tmp/cache",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPathWithinBase(tt.filePath, tt.baseDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// createTarGz creates a gzipped tar archive with the given entries.
// Each entry is a map with "name" (path) and "content" (file body).
func createTarGz(t *testing.T, entries []map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, entry := range entries {
		name := entry["name"]
		content := entry["content"]
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return &buf
}

func TestExtractCacheAndManifestDirectory_ZipSlipRejected(t *testing.T) {
	cachePrefix := "io.triton.cache/"
	manifestPrefix := "io.triton.manifest/"

	// Create a tar with a path traversal entry
	archive := createTarGz(t, []map[string]string{
		{
			"name":    "io.triton.cache/../../etc/cron.d/backdoor",
			"content": "malicious content",
		},
	})

	extractDir := t.TempDir()
	cacheDir := filepath.Join(extractDir, "cache")
	manifestDir := filepath.Join(extractDir, "manifest")

	_, err := extractCacheAndManifestDirectory(archive, cachePrefix, manifestPrefix, cacheDir, manifestDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal path in tar entry")

	// Verify no file was written outside the extraction directory
	_, statErr := os.Stat(filepath.Join(extractDir, "..", "etc", "cron.d", "backdoor"))
	assert.True(t, os.IsNotExist(statErr), "traversal file should not exist")
}

func TestExtractCacheAndManifestDirectory_ManifestZipSlipRejected(t *testing.T) {
	cachePrefix := "io.triton.cache/"
	manifestPrefix := "io.triton.manifest/"

	// The filter at line 226 only passes entries with prefix "io.triton.manifest/manifest.json",
	// so we craft a traversal name that passes that filter.
	archive := createTarGz(t, []map[string]string{
		{
			"name":    "io.triton.manifest/manifest.json/../../../etc/shadow",
			"content": "malicious manifest",
		},
	})

	extractDir := t.TempDir()
	cacheDir := filepath.Join(extractDir, "cache")
	manifestDir := filepath.Join(extractDir, "manifest")

	_, err := extractCacheAndManifestDirectory(archive, cachePrefix, manifestPrefix, cacheDir, manifestDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal path in tar entry")
}

func TestWriteFile_ExcessiveDeclaredSizeRejected(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bomb.bin")
	reader := bytes.NewReader([]byte("small"))

	err := writeFile(filePath, reader, 0o644, maxFileSize+1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or excessive size")
}

func TestWriteFile_NegativeSizeRejected(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.bin")
	reader := bytes.NewReader([]byte("data"))

	err := writeFile(filePath, reader, 0o644, -1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or excessive size")
}

func TestExtractCacheAndManifestDirectory_ValidEntriesExtracted(t *testing.T) {
	cachePrefix := "io.triton.cache/"
	manifestPrefix := "io.triton.manifest/"

	archive := createTarGz(t, []map[string]string{
		{
			"name":    "io.triton.cache/kernel.so",
			"content": "kernel binary data",
		},
		{
			"name":    "io.triton.manifest/manifest.json",
			"content": `{"triton":[]}`,
		},
	})

	extractDir := t.TempDir()
	cacheDir := filepath.Join(extractDir, "cache")
	manifestDir := filepath.Join(extractDir, "manifest")

	dirs, err := extractCacheAndManifestDirectory(archive, cachePrefix, manifestPrefix, cacheDir, manifestDir)
	require.NoError(t, err)
	assert.NotEmpty(t, dirs)

	// Verify the cache file was extracted
	content, err := os.ReadFile(filepath.Join(cacheDir, "kernel.so")) //nolint:gosec // G304: test-only path from t.TempDir()
	require.NoError(t, err)
	assert.Equal(t, "kernel binary data", string(content))

	// Verify the manifest was extracted
	content, err = os.ReadFile(filepath.Join(manifestDir, "manifest.json")) //nolint:gosec // G304: test-only path from t.TempDir()
	require.NoError(t, err)
	assert.Equal(t, `{"triton":[]}`, string(content))
}
