package leveldbcache

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/gregjones/httpcache/test"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "httpcache")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cache, err := New(filepath.Join(tempDir, "db"))
	if err != nil {
		t.Fatalf("New leveldb,: %v", err)
	}

	test.Cache(t, cache)
}
