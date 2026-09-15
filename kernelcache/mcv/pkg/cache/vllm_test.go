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

	"github.com/kserve/kserve/kernelcache/mcv/pkg/constants"
	"github.com/stretchr/testify/assert"
)

const (
	megaAOTHash = "d5313e9d59c8842ac8d3b743f0c1c018ea9b101c4f9ae1134b8c85e61557f070"
	secondHash  = "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
	thirdHash   = "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"
	testRank00  = "rank_0_0"
	testRank01  = "rank_0_1"
)

// writeTestFile is a test helper that creates parent dirs and writes content.
func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	assert.NoError(t, os.WriteFile(path, content, 0o640))
}

// newMegaAOTCache builds a fake mega-AOT cache tree rooted at cacheDir with
// the a hash and rank dirs. Each rank dir gets a "model" file plus a
// shared inductor_cache/triton/0/ kernel dir next to the rank dirs.
func newMegaAOTCache(t *testing.T, cacheDir string, ranks []string) {
	hash := megaAOTHash
	t.Helper()
	hashDir := filepath.Join(cacheDir, constants.TorchCompileDir, torchAOTCompileDirName, hash)
	for _, rank := range ranks {
		writeTestFile(t, filepath.Join(hashDir, rank, "model"), []byte("mega-aot-blob"))
	}
	writeTestFile(t, filepath.Join(hashDir, "inductor_cache", "triton", "0", "kernel.cubin"), []byte("cubin"))
}

// newMegaAOTCacheWithHashes builds a fake mega-AOT cache tree with multiple hashes.
// Each hash gets its own directory with the specified ranks.
func newMegaAOTCacheWithHashes(t *testing.T, cacheDir string, hashes, ranks []string) {
	t.Helper()
	for _, hash := range hashes {
		hashDir := filepath.Join(cacheDir, constants.TorchCompileDir, torchAOTCompileDirName, hash)
		for _, rank := range ranks {
			writeTestFile(t, filepath.Join(hashDir, rank, "model"), []byte("mega-aot-blob"))
		}
		writeTestFile(t, filepath.Join(hashDir, "inductor_cache", "triton", "0", "kernel.cubin"), []byte("cubin"))
	}
}

func TestDetectVLLMCache_NoCacheReturnsNil(t *testing.T) {
	assert.Nil(t, DetectVLLMCache(t.TempDir()))
}

func TestDetectVLLMCache_MegaAOTSingleRank(t *testing.T) {
	cacheDir := t.TempDir()
	newMegaAOTCache(t, cacheDir, []string{testRank00})

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
	newMegaAOTCache(t, cacheDir, []string{testRank00, "rank_1_0"})

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
	hashDir := filepath.Join(cacheDir, constants.TorchCompileDir, torchAOTCompileDirName, megaAOTHash)
	// rank_0_0 has model; rank_1_0 is an empty dir (e.g. partial write).
	writeTestFile(t, filepath.Join(hashDir, testRank00, "model"), []byte("blob"))
	assert.NoError(t, os.MkdirAll(filepath.Join(hashDir, "rank_1_0"), 0o750))
	writeTestFile(t, filepath.Join(hashDir, "inductor_cache", "fxgraph", "key"), []byte("x"))

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)
	entry := got.Metadata()[0].(VLLMCacheMetadata)
	assert.Len(t, entry.BinaryCacheEntries, 1)
	assert.Equal(t, testRank00, entry.BinaryCacheEntries[0].Rank)
}

func TestDetectVLLMCache_MegaAOTMetadataMarshalsToManifest(t *testing.T) {
	cacheDir := t.TempDir()
	newMegaAOTCache(t, cacheDir, []string{testRank00})

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

func TestVLLMCache_GenericMountingLabels(t *testing.T) {
	cacheDir := t.TempDir()
	newMegaAOTCache(t, cacheDir, []string{testRank00})

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)

	labels := got.Labels()

	// Verify the 5 generic mounting labels for KServe Kernel Manager integration
	assert.Equal(t, constants.VLLM, labels[kmFramework], "framework label should be 'vllm'")
	assert.Equal(t, constants.CacheTypeVLLMTorchCompile, labels[kmCacheType],
		"cache-type label should be 'torch-compile'")

	assert.Equal(t, megaAOTHash, labels[kmCacheHash], "cache-hash label should contain the vLLM hash")

	// Mega-AOT caches use torch_compile_cache/torch_aot_compile/<hash>
	expectedSubpath := filepath.Join(constants.TorchCompileDir, torchAOTCompileDirName, megaAOTHash)
	assert.Equal(t, expectedSubpath, labels[kmCacheMountSubpath],
		"cache-mount-subpath should be torch_compile_cache/torch_aot_compile/<hash>")

	assert.Equal(t, vllmCacheRootEnvDefault, labels[kmCacheRootEnv],
		"cache-root-env should be VLLM_CACHE_ROOT=/home/kserve/.cache/vllm")

	// Verify existing labels still present
	assert.Equal(t, BinaryCacheFormat, labels[cacheVLLMImageFormat])
	assert.Equal(t, "1", labels[cacheVLLMImageEntryCount])
}

func TestVLLMCache_GenericMountingLabels_MultipleHashes(t *testing.T) {
	cacheDir := t.TempDir()
	// Create cache with 3 unique hashes, plus a duplicate to test deduplication
	// Note: filesystem order is non-deterministic, so we can't assert specific order
	hashes := []string{megaAOTHash, secondHash, megaAOTHash, thirdHash}
	newMegaAOTCacheWithHashes(t, cacheDir, hashes, []string{testRank00, testRank01})

	got := DetectVLLMCache(cacheDir)
	assert.NotNil(t, got)

	labels := got.Labels()

	// Verify framework and cache-type labels
	assert.Equal(t, constants.VLLM, labels[kmFramework])
	assert.Equal(t, constants.CacheTypeVLLMTorchCompile, labels[kmCacheType])

	// Verify kmCacheHash contains comma-separated UNIQUE hashes
	// We can't assert specific order (filesystem-dependent), but we can verify:
	// 1. All 3 unique hashes are present
	// 2. No duplicates
	// 3. Comma-separated format
	cacheHashLabel := labels[kmCacheHash]
	hashesInLabel := make(map[string]bool)
	for _, h := range []string{megaAOTHash, secondHash, thirdHash} {
		assert.Contains(t, cacheHashLabel, h, "cache-hash label should contain hash %s", h)
		hashesInLabel[h] = true
	}
	// Count commas - should be 2 (for 3 hashes)
	commaCount := 0
	for _, c := range cacheHashLabel {
		if c == ',' {
			commaCount++
		}
	}
	assert.Equal(t, 2, commaCount, "cache-hash label should have exactly 2 commas for 3 unique hashes")

	// Verify kmCacheMountSubpath uses ONLY ONE hash (the first in discovery order)
	// We can verify it's one of our 3 hashes, but not which one (order is non-deterministic)
	// Mega-AOT caches include torch_aot_compile in the path
	mountSubpath := labels[kmCacheMountSubpath]
	foundValidHash := false
	for h := range hashesInLabel {
		expectedSubpath := filepath.Join(constants.TorchCompileDir, torchAOTCompileDirName, h)
		if mountSubpath == expectedSubpath {
			foundValidHash = true
			break
		}
	}
	assert.True(t, foundValidHash,
		"cache-mount-subpath should use exactly one of the unique hashes, got: %s", mountSubpath)

	// Verify kmCacheRootEnv is still set correctly
	assert.Equal(t, vllmCacheRootEnvDefault, labels[kmCacheRootEnv],
		"cache-root-env should be VLLM_CACHE_ROOT=/home/kserve/.cache/vllm")

	// Verify existing labels still present
	assert.Equal(t, BinaryCacheFormat, labels[cacheVLLMImageFormat])
	// Entry count should be 3 (deduplicated unique hashes), not 4
	assert.Equal(t, "3", labels[cacheVLLMImageEntryCount], "should count 3 unique hash directories")
}
