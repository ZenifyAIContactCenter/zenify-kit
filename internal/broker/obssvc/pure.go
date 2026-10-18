// Package obssvc holds the pure helpers behind the broker's obs routes:
// time windows, histogram quantiles, trace id handling, PII masking and
// response capping. No HTTP lives here.
package obssvc

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrBadSince   = errors.New("since must be >= 1m and <= 48h")
	ErrBadTraceID = errors.New("trace id must be 1-32 hex characters")
)

const (
	minSince  = time.Minute
	maxSince  = 48 * time.Hour
	maxWindow = 2 * time.Hour
)

// Window is a half-open time range [Start, End).
type Window struct{ Start, End time.Time }

// Windows splits [end-since, end) into contiguous windows of at most 2h.
func Windows(since time.Duration, end time.Time) ([]Window, error) {
	if since < minSince || since > maxSince {
		return nil, ErrBadSince
	}
	var out []Window
	for s := end.Add(-since); s.Before(end); {
		e := s.Add(maxWindow)
		if e.After(end) {
			e = end
		}
		out = append(out, Window{s, e})
		s = e
	}
	return out, nil
}

// PadTraceID validates a 1-32 char hex id and returns it lowercased and
// left-padded with zeros to 32 chars, plus the original input.
func PadTraceID(s string) (padded, orig string, err error) {
	if len(s) == 0 || len(s) > 32 {
		return "", "", ErrBadTraceID
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return "", "", ErrBadTraceID
		}
	}
	return strings.Repeat("0", 32-len(s)) + strings.ToLower(s), s, nil
}

// Hist maps a bucket upper bound (seconds) to its span count.
type Hist map[float64]float64

// Quantile interpolates linearly inside the bucket holding rank q*total.
// A bucket's lower bound is its upper bound / 2. Empty histogram → 0.
func Quantile(h Hist, q float64) float64 {
	bounds := make([]float64, 0, len(h))
	var total float64
	for b, n := range h {
		if n > 0 {
			bounds = append(bounds, b)
			total += n
		}
	}
	if total == 0 {
		return 0
	}
	sort.Float64s(bounds)
	rank := math.Max(0, math.Min(1, q)) * total
	var cum float64
	for _, b := range bounds {
		n := h[b]
		if cum+n >= rank {
			lo := b / 2
			return lo + (b-lo)*(rank-cum)/n
		}
		cum += n
	}
	return bounds[len(bounds)-1]
}

// Truncate cuts s to at most max bytes without splitting a UTF-8 rune.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < 0 {
		max = 0
	}
	n := max
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// preCut cuts s to at most max bytes for masking. A longer s is cut back to
// the last whitespace or value separator before max, then a trailing run of
// phone or IP characters is dropped, so no token straddling the cut survives
// as a fragment the masks no longer recognise.
func preCut(s string, max int) string {
	if len(s) <= max {
		return s
	}
	i := strings.LastIndexAny(s[:max], " \t\r\n\"'&,;")
	if i < 0 {
		return ""
	}
	return strings.TrimRight(s[:i], "0123456789 \t.+-")
}

var allowed = map[string]bool{
	"http.method": true, "http.request.method": true, "http.route": true,
	"http.status_code": true, "http.response.status_code": true,
	"db.system": true, "db.operation": true, "db.name": true,
	"db.mongodb.collection": true, "db.sql.table": true, "db.statement": true,
	"messaging.system": true, "messaging.destination.name": true,
	"rpc.service": true, "rpc.method": true,
	"exception.type": true, "exception.message": true,
	"otel.status_code": true, "otel.status_description": true,
}

// AllowAttr reports whether a span attribute may leave the broker: the fixed
// allowlist plus every key in extra (the catalog's attributes).
func AllowAttr(key string, extra map[string]string) bool {
	if allowed[key] {
		return true
	}
	_, ok := extra[key]
	return ok
}

// Capper is a response that can shed items one at a time, in its own order.
type Capper interface {
	Drop() (kind string, ok bool)
}

// CapJSON encodes v and, while the result exceeds max bytes, calls Drop and
// re-encodes until it fits or nothing is left to drop. It returns the last
// encoding.
func CapJSON(v Capper, max int) ([]byte, error) {
	for {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if len(b) <= max {
			return b, nil
		}
		if _, ok := v.Drop(); !ok {
			return b, nil
		}
	}
}
