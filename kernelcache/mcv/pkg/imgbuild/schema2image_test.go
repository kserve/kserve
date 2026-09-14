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
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/cache"
)

func TestImageTitleFromName(t *testing.T) {
	assert.Equal(t, "qwen-aot-cache", imageTitleFromName("quay.io/mtahhan/qwen-aot-cache:latest"))
	assert.Equal(t, "myimage", imageTitleFromName("myorg/myimage:1.0"))
}

func TestSchema2ImageManifestMediaTypes(t *testing.T) {
	tmpDir := t.TempDir()
	cacheTag := "io.vllm.cache"
	manifestTag := "io.vllm.manifest"
	cacheDir := filepath.Join(tmpDir, cacheTag)
	manifestDir := filepath.Join(tmpDir, manifestTag)

	assert.NoError(t, os.MkdirAll(cacheDir, 0o750))
	assert.NoError(t, os.MkdirAll(manifestDir, 0o750))
	assert.NoError(t, cache.WriteManifest(filepath.Join(manifestDir, "manifest.json"), cache.Manifest{
		"vllm": []cache.CacheEntry{},
	}))
	assert.NoError(t, os.WriteFile(filepath.Join(cacheDir, "example.txt"), []byte("cache-data"), 0o640))

	prep := &buildContext{
		Labels: cache.Labels{
			"cache.vllm.image/entry-count": "0",
		},
		CacheTag:         cacheTag,
		ManifestTag:      manifestTag,
		CacheBuildDir:    cacheDir,
		ManifestBuildDir: manifestDir,
	}

	img, err := schema2ImageFromBuildContext(prep, "quay.io/example/cache:latest")
	assert.NoError(t, err)

	manifestMT, err := img.MediaType()
	assert.NoError(t, err)
	assert.Equal(t, types.DockerManifestSchema2, manifestMT)

	manifest, err := img.Manifest()
	assert.NoError(t, err)
	assert.Equal(t, types.DockerManifestSchema2, manifest.MediaType)
	assert.Equal(t, types.DockerConfigJSON, manifest.Config.MediaType)
	if assert.Len(t, manifest.Layers, 1) {
		assert.Equal(t, types.DockerLayer, manifest.Layers[0].MediaType)
	}

	cfg, err := img.ConfigFile()
	assert.NoError(t, err)
	assert.Equal(t, "cache", cfg.Config.Labels[imageTitleLabel])
}

func TestCompatLayerContainsExpectedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	cacheTag := "io.triton.cache"
	manifestTag := "io.triton.manifest"
	cacheDir := filepath.Join(tmpDir, cacheTag)
	manifestDir := filepath.Join(tmpDir, manifestTag)

	assert.NoError(t, os.MkdirAll(cacheDir, 0o750))
	assert.NoError(t, os.MkdirAll(manifestDir, 0o750))
	assert.NoError(t, os.WriteFile(filepath.Join(cacheDir, "kernel.bin"), []byte("kernels"), 0o640))
	assert.NoError(t, os.WriteFile(filepath.Join(manifestDir, "manifest.json"), []byte(`{}`), 0o640))

	prep := &buildContext{
		CacheTag:         cacheTag,
		ManifestTag:      manifestTag,
		CacheBuildDir:    cacheDir,
		ManifestBuildDir: manifestDir,
	}

	layer, err := compatLayerFromBuildContext(prep)
	assert.NoError(t, err)
	if prep.TempLayerFile != "" {
		t.Cleanup(func() { _ = os.Remove(prep.TempLayerFile) })
	}

	compressed, err := layer.Compressed()
	assert.NoError(t, err)
	defer compressed.Close()

	gz, err := gzip.NewReader(compressed)
	assert.NoError(t, err)
	defer gz.Close()

	paths := map[string]struct{}{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
		paths[header.Name] = struct{}{}
	}

	assert.Contains(t, paths, cacheTag+"/kernel.bin")
	assert.Contains(t, paths, manifestTag+"/manifest.json")
}
