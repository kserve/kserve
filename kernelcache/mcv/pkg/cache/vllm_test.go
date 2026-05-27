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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	megaAOTHash = "d5313e9d59c8842ac8d3b743f0c1c018ea9b101c4f9ae1134b8c85e61557f070"
	testRank00  = "rank_0_0"
)

// writeTestFile is a test helper that creates parent dirs and writes content.
func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoError(t, os.WriteFile(path, content, 0o644))
}

// newMegaAOTCache builds a fake mega-AOT cache tree rooted at cacheDir with
// the given hash and rank dirs. Each rank dir gets a "model" file plus a
// shared inductor_cache/triton/0/ kernel dir next to the rank dirs.
func newMegaAOTCache(t *testing.T, cacheDir, hash string, ranks []string) {
	t.Helper()
	hashDir := filepath.Join(cacheDir, "torch_compile_cache", torchAOTCompileDirName, hash)
	for _, rank := range ranks {
		writeTestFile(t, filepath.Join(hashDir, rank, "model"), []byte("mega-aot-blob"))
	}
	writeTestFile(t, filepath.Join(hashDir, "inductor_cache", "triton", "0", "kernel.cubin"), []byte("cubin"))
}

func TestDetectVLLMCache_NoCacheReturnsNil(t *testing.T) {
	assert.Nil(t, DetectVLLMCache(t.TempDir()))
}

func TestDetectVLLMCache_MegaAOTSingleRank(t *testing.T) {
	cacheDir := t.TempDir()
	newMegaAOTCache(t, cacheDir, megaAOTHash, []string{testRank00})

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)
	assert.Equal(t, 1, got.EntryCount())

	meta := got.Metadata()
	assert.Len(t, meta, 1)

	entry, ok := meta[0].(VLLMCacheMetadata)
	assert.True(t, ok, "expected VLLMCacheMetadata, got %T", meta[0])
	assert.Equal(t, megaAOTHash, entry.VllmHash)
	assert.Equal(t, BinaryCacheFormat, entry.CacheFormat)
	assert.Len(t, entry.BinaryCacheEntries, 1)

	bin := entry.BinaryCacheEntries[0]
	assert.Equal(t, testRank00, bin.Rank)
	assert.Equal(t, 1, bin.ArtifactCount)
	assert.Equal(t, []string{"model"}, bin.ArtifactNames)
	assert.Equal(t, megaAOTSaveFormat, bin.CacheSaveFormat)

	// Labels flag the cache as binary format, matching existing manifest
	// consumers and the preflight check.
	labels := got.Labels()
	assert.Equal(t, BinaryCacheFormat, labels[cacheVLLMImageFormat])
	assert.Equal(t, "1", labels[cacheVLLMImageEntryCount])
}

func TestDetectVLLMCache_MegaAOTMultiRank(t *testing.T) {
	cacheDir := t.TempDir()
	newMegaAOTCache(t, cacheDir, megaAOTHash, []string{testRank00, "rank_1_0"})

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)

	meta := got.Metadata()
	assert.Len(t, meta, 1)
	entry, ok := meta[0].(VLLMCacheMetadata)
	assert.True(t, ok)
	assert.Len(t, entry.BinaryCacheEntries, 2)

	ranks := []string{entry.BinaryCacheEntries[0].Rank, entry.BinaryCacheEntries[1].Rank}
	assert.ElementsMatch(t, []string{testRank00, "rank_1_0"}, ranks)
}

func TestDetectVLLMCache_MegaAOTSkipsRankWithoutModel(t *testing.T) {
	cacheDir := t.TempDir()
	hashDir := filepath.Join(cacheDir, "torch_compile_cache", torchAOTCompileDirName, megaAOTHash)
	// rank_0_0 has model; rank_1_0 is an empty dir (e.g. partial write).
	writeTestFile(t, filepath.Join(hashDir, testRank00, "model"), []byte("blob"))
	assert.NoError(t, os.MkdirAll(filepath.Join(hashDir, "rank_1_0"), 0o755))
	writeTestFile(t, filepath.Join(hashDir, "inductor_cache", "fxgraph", "key"), []byte("x"))

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)
	entry := got.Metadata()[0].(VLLMCacheMetadata)
	assert.Len(t, entry.BinaryCacheEntries, 1)
	assert.Equal(t, testRank00, entry.BinaryCacheEntries[0].Rank)
}

func TestDetectVLLMCache_MegaAOTMetadataMarshalsToManifest(t *testing.T) {
	cacheDir := t.TempDir()
	newMegaAOTCache(t, cacheDir, megaAOTHash, []string{testRank00})

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)

	// Round-trip through the VLLMManifest shape used on disk, matching what
	// the preflight check ingests at mcv/pkg/preflightcheck/vllm.go.
	entries := make([]VLLMCacheMetadata, 0, len(got.Metadata()))
	for _, m := range got.Metadata() {
		entries = append(entries, m.(VLLMCacheMetadata))
	}
	data, err := json.Marshal(VLLMManifest{VLLM: entries})
	assert.NoError(t, err)

	var round VLLMManifest
	assert.NoError(t, json.Unmarshal(data, &round))
	assert.Len(t, round.VLLM, 1)
	assert.Equal(t, BinaryCacheFormat, round.VLLM[0].CacheFormat)
	assert.Len(t, round.VLLM[0].BinaryCacheEntries, 1)
	assert.Equal(t, megaAOTSaveFormat, round.VLLM[0].BinaryCacheEntries[0].CacheSaveFormat)
}
