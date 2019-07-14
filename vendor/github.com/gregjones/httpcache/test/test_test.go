package test_test

import (
	"testing"

	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/test"
)

func TestMemoryCache(t *testing.T) {
	test.Cache(t, httpcache.NewMemoryCache())
}
