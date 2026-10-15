package api

import (
	"fmt"
	"strconv"
	"strings"
)

// RedisAllowed is the read-only command allowlist of POST /redis (upper case).
var RedisAllowed = map[string]bool{
	"GET": true, "MGET": true, "HGET": true, "HGETALL": true, "HKEYS": true,
	"LRANGE": true, "LLEN": true, "SMEMBERS": true, "SCARD": true, "ZRANGE": true,
	"ZCARD": true, "XRANGE": true, "TTL": true, "TYPE": true, "EXISTS": true, "SCAN": true,
}

// SCAN COUNT bounds.
const (
	scanDefaultCount = 100
	scanMaxCount     = 1000
)

// CheckRedis validates one command against RedisAllowed and returns the full
// argv to send (command upper-cased first, then args). SCAN gets COUNT 100
// appended when absent and is refused when COUNT is not in 1..1000. Any error
// maps to 400 command_not_allowed.
func CheckRedis(cmd string, args []string) (normArgs []string, err error) {
	c := strings.ToUpper(strings.TrimSpace(cmd))
	if !RedisAllowed[c] {
		return nil, fmt.Errorf("redis command %q is not allowed", cmd)
	}
	out := append([]string{c}, args...)
	if c != "SCAN" {
		return out, nil
	}
	// SCAN cursor [MATCH p] [COUNT n] [TYPE t]: options start after the cursor.
	for i := 1; i < len(args); i++ {
		if !strings.EqualFold(args[i], "COUNT") {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("SCAN COUNT needs a value")
		}
		n, err := strconv.Atoi(args[i+1])
		if err != nil || n < 1 || n > scanMaxCount {
			return nil, fmt.Errorf("SCAN COUNT must be 1..%d", scanMaxCount)
		}
		return out, nil
	}
	return append(out, "COUNT", strconv.Itoa(scanDefaultCount)), nil
}
