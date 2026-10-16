package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter is an in-memory token bucket per key: burst tokens, refilled at
// burst per window.
type limiter struct {
	burst   float64
	window  time.Duration
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxBuckets bounds memory; past it, buckets that have refilled are dropped.
const maxBuckets = 10000

// maxKeyLen bounds one bucket key: a longer key (a forged X-Real-IP)
// is stored as its SHA-256 hex, so maxBuckets bounds memory in bytes too.
const maxKeyLen = 256

func newLimiter(burst int, window time.Duration) *limiter {
	return &limiter{burst: float64(burst), window: window, buckets: map[string]*bucket{}}
}

func (l *limiter) refill(b *bucket, now time.Time) {
	if el := now.Sub(b.last); el > 0 {
		b.tokens = min(l.burst, b.tokens+el.Seconds()*l.burst/l.window.Seconds())
		b.last = now
	}
}

// allow takes one token from key's bucket, reporting whether one was there.
func (l *limiter) allow(key string, now time.Time) bool {
	if len(key) > maxKeyLen {
		sum := sha256.Sum256([]byte(key))
		key = hex.EncodeToString(sum[:])
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxBuckets {
			for k, o := range l.buckets {
				if l.refill(o, now); o.tokens >= l.burst {
					delete(l.buckets, k)
				}
			}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	l.refill(b, now)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP is X-Real-IP, else the connection's host. Every request arrives
// through the TLS proxy in front of the broker, which overwrites X-Real-IP
// with the peer address. The broker is published on 127.0.0.1 only, so a
// client cannot reach it with a forged value.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
