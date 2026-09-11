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
