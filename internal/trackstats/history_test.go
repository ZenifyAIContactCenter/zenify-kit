package trackstats

import (
	"fmt"
	"testing"
	"time"
)

func histRow(i int, kind string, min int, pairs string) string {
	ts := now.Add(-time.Duration(1000-i) * time.Minute)
	return row(ts, "history", fmt.Sprintf("%08x", i), kind, "kit", fmt.Sprint(min), pairs)
}

func histLines(n int, kind string) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, histRow(i, kind, i, "-"))
	}
	return out
}

func TestHistoryParsesWithoutEffect(t *testing.T) {
	r, _ := Compute(base(lines(append(histLines(30, "fix"), histRow(40, "fix", 3, "opus:1/1"))...)))
	if r.Malformed != 0 || r.Scale.Source != "default" || len(r.Quality.ByKind) != 0 || len(r.Estimate.ByKind) != 0 {
		t.Fatalf("history leaked: %+v malformed=%d", r, r.Malformed)
	}
}

func TestHistoryMalformed(t *testing.T) {
	r, _ := Compute(base(lines(
		row(now, "history", "abcd1234", "fix", "kit", "3", "opus:x/1"),
		row(now, "history", "abcd1234", "fix", "kit", "x", "-"),
		row(now, "history", "abcd1234", "nope", "kit", "3", "-"),
		row(now, "history", "abcd1234", "fix", "kit", "3", "opus"),
	)))
	if r.Malformed != 4 {
		t.Fatalf("malformed = %d, want 4", r.Malformed)
	}
}
