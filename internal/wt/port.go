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

// AllocateRange returns a deterministic CONTIGUOUS block of `count` free ports
// in [lo,hi] for identity `key`: a base b such that b..b+count-1 are all within
// the span, none in `taken`, and all pass the bind-test. The hash fixes the
// starting base; it then linear-probes the base forward (wrapping within the
// span). Returns (ports,true) or (nil,false) if no such block exists. count<=1
// degenerates to a single-port block. Kept separate from Allocate so the proven
// single-port path is untouched.
func AllocateRange(key string, lo, hi, count int, taken map[int]bool) ([]int, bool) {
	if count < 1 {
		count = 1
	}
	if hi < lo || hi-lo+1 < count {
		return nil, false
	}
	span := hi - lo + 1
	start := int(hashKey(key) % uint32(span))
	// A block cannot start so late it would run past hi; only bases with room for
	// the whole block are candidates. Probe those in hashed order.
	for i := 0; i < span; i++ {
		base := lo + (start+i)%span
		if base+count-1 > hi {
			continue
		}
		ok := true
		for off := 0; off < count; off++ {
			p := base + off
			if taken[p] || !portFree(p) {
				ok = false
				break
			}
		}
		if ok {
			ports := make([]int, count)
			for off := 0; off < count; off++ {
				ports[off] = base + off
			}
			return ports, true
		}
	}
	return nil, false
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
	// Guard p<=0: net.Listen(":0") asks the OS for an arbitrary ephemeral port
	// and would report 0 as "free" — never a real, re-bindable port.
	if p <= 0 {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
