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

// scanOptions are SCAN's option keywords. A value equal to one of them is
// read as a missing value (so MATCH cannot match a key literally named "count").
var scanOptions = map[string]bool{"MATCH": true, "COUNT": true, "TYPE": true}

// SCAN COUNT bounds.
const (
	scanDefaultCount = 100
	scanMaxCount     = 1000
)

// CheckRedis validates one command against RedisAllowed and returns the full
// argv to send (command upper-cased first, then args). SCAN gets COUNT 100
// appended when absent and is refused when COUNT is repeated, not in 1..1000,
// or any option lacks its value. Any error
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
	// SCAN cursor [MATCH p] [COUNT n] [TYPE t]: options come in keyword/value
	// pairs after the cursor. Every pair is checked, so a later COUNT cannot
	// override a valid earlier one.
	hasCount := false
	opts := args[min(1, len(args)):]
	for len(opts) > 0 {
		kw := strings.ToUpper(opts[0])
		if len(opts) < 2 || scanOptions[strings.ToUpper(opts[1])] {
			return nil, fmt.Errorf("SCAN option %s is missing its value", kw)
		}
		val := opts[1]
		opts = opts[2:]
		if kw != "COUNT" {
			continue
		}
		if hasCount {
			return nil, fmt.Errorf("SCAN COUNT given more than once")
		}
		hasCount = true
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > scanMaxCount {
			return nil, fmt.Errorf("SCAN COUNT must be 1..%d", scanMaxCount)
		}
	}
	if hasCount {
		return out, nil
	}
	return append(out, "COUNT", strconv.Itoa(scanDefaultCount)), nil
}
