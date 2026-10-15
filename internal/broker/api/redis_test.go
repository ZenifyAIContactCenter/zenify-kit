package api

import (
	"reflect"
	"testing"
)

func TestCheckRedis(t *testing.T) {
	ok := []struct {
		cmd  string
		args []string
		want []string
	}{
		{"get", []string{"k"}, []string{"GET", "k"}},
		{"HgetAll", []string{"h"}, []string{"HGETALL", "h"}},
		{"SCAN", []string{"0"}, []string{"SCAN", "0", "COUNT", "100"}},
		{"SCAN", nil, []string{"SCAN", "0", "COUNT", "100"}},
		{"scan", []string{"0", "MATCH", "a:*"}, []string{"SCAN", "0", "MATCH", "a:*", "COUNT", "100"}},
		{"SCAN", []string{"0", "count", "1000"}, []string{"SCAN", "0", "count", "1000"}},
		{"SCAN", []string{"0", "COUNT", "10", "MATCH", "a:*", "TYPE", "hash"}, []string{"SCAN", "0", "COUNT", "10", "MATCH", "a:*", "TYPE", "hash"}},
	}
	for _, c := range ok {
		got, err := CheckRedis(c.cmd, c.args)
		if err != nil {
			t.Errorf("CheckRedis(%q, %v) error: %v", c.cmd, c.args, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("CheckRedis(%q, %v) = %v, want %v", c.cmd, c.args, got, c.want)
		}
	}

	bad := []struct {
		cmd  string
		args []string
	}{
		{"DEL", []string{"k"}},
		{"set", []string{"k", "v"}},
		{"FLUSHALL", nil},
		{"SCAN", []string{"0", "COUNT", "5000"}},
		{"SCAN", []string{"0", "COUNT", "0"}},
		{"SCAN", []string{"0", "COUNT", "abc"}},
		{"SCAN", []string{"0", "COUNT"}},
		{"SCAN", []string{"0", "COUNT", "10", "COUNT", "5000"}},
		{"SCAN", []string{"0", "COUNT", "10", "COUNT", "20"}},
		{"SCAN", []string{"0", "MATCH", "COUNT"}},
		{"SCAN", []string{"0", "MATCH"}},
	}
	for _, c := range bad {
		if got, err := CheckRedis(c.cmd, c.args); err == nil {
			t.Errorf("CheckRedis(%q, %v) = %v, want error", c.cmd, c.args, got)
		}
	}
}

func TestRedisAllowed_ExactSet(t *testing.T) {
	want := []string{"GET", "MGET", "HGET", "HGETALL", "HKEYS", "LRANGE", "LLEN", "SMEMBERS",
		"SCARD", "ZRANGE", "ZCARD", "XRANGE", "TTL", "TYPE", "EXISTS", "SCAN"}
	if len(RedisAllowed) != len(want) {
		t.Fatalf("RedisAllowed has %d entries, want %d", len(RedisAllowed), len(want))
	}
	for _, c := range want {
		if !RedisAllowed[c] {
			t.Errorf("%s missing from RedisAllowed", c)
		}
	}
}

func TestCheckRedis_MissingValueMessage(t *testing.T) {
	_, err := CheckRedis("SCAN", []string{"0", "MATCH", "COUNT"})
	if err == nil || err.Error() != "SCAN option MATCH is missing its value" {
		t.Errorf("err = %v", err)
	}
}
