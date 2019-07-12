package redis

import (
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/gregjones/httpcache/test"
)

func TestRedisCache(t *testing.T) {
	conn, err := redis.Dial("tcp", "localhost:6379")
	if err != nil {
		// TODO: rather than skip the test, fall back to a faked redis server
		t.Skipf("skipping test; no server running at localhost:6379")
	}
	conn.Do("FLUSHALL")

	test.Cache(t, NewWithClient(conn))
}
