package wt

import (
	"fmt"
	"hash/fnv"
	"net"
)

// Allocate returns a deterministic free port in [lo,hi] for identity `key`
// (e.g. "repo:slug:service"). The hash fixes a starting offset; Allocate then
// linear-probes forward (wrapping within the span), skipping ports in `taken`
// and ports that fail a bind-test. Returns (port,true), or (0,false) if the
// whole span is unavailable. The port VALUE is not a contract (it need not
// match the bash wt); only "deterministic + free" is.
func Allocate(key string, lo, hi int, taken map[int]bool) (int, bool) {
	if hi < lo {
		return 0, false
	}
	span := hi - lo + 1
	start := int(hashKey(key) % uint32(span))
	for i := 0; i < span; i++ {
		p := lo + (start+i)%span
		if taken[p] {
			continue
		}
		if portFree(p) {
			return p, true
		}
	}
	return 0, false
}

func hashKey(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// portFree bind-tests 0.0.0.0:p natively (replacing the bash `lsof` check). A
// listen that succeeds and closes cleanly means the port was free at check time.
// This is best-effort by nature: a port can be grabbed between check and use —
// the mutating slice that actually binds it must handle that race.
func portFree(p int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
