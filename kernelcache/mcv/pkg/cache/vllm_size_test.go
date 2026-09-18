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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVLLMCache_CacheSizeBytesPrefersTmpPath(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()

	writeTestFile(t, filepath.Join(root, "ignored.bin"), []byte("12345"))
	writeTestFile(t, filepath.Join(staging, "packed.bin"), []byte("abc"))

	cache := &VLLMCache{rootPath: root}
	assert.Equal(t, int64(5), cache.CacheSizeBytes())

	cache.SetTmpPath(staging)
	assert.Equal(t, int64(3), cache.CacheSizeBytes())
}

func TestTritonCache_CacheSizeBytesUsesTmpPath(t *testing.T) {
	staging := t.TempDir()
	writeTestFile(t, filepath.Join(staging, "kernel.bin"), []byte("triton"))

	cache := &TritonCache{}
	assert.Equal(t, int64(0), cache.CacheSizeBytes())

	cache.SetTmpPath(staging)
	assert.Equal(t, int64(6), cache.CacheSizeBytes())
}

func TestTotalDirSize(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a"), []byte("aa"))
	writeTestFile(t, filepath.Join(dir, "sub", "b"), []byte("bbb"))

	size, err := TotalDirSize(dir)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), size)
}
