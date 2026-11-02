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

func TestHistoryOnlyScale(t *testing.T) { // SC-34
	r, _ := Compute(base(lines(histLines(20, "-")...)))
	if r.Scale.Source != "history" || r.Scale.N != 20 || r.Scale.S != 5 || r.Scale.M != 10 || r.Scale.L != 18 {
		t.Fatalf("scale = %+v", r.Scale)
	}
	if len(r.Estimate.ByKind) != 0 || r.Malformed != 0 {
		t.Fatalf("estimate = %+v malformed=%d", r.Estimate.ByKind, r.Malformed)
	}
	if r, _ = Compute(base(lines(histLines(19, "-")...))); r.Scale.Source != "default" || r.Scale.N != 19 {
		t.Fatalf("scale = %+v", r.Scale)
	}
}

func TestHistoryTopsUpRealItems(t *testing.T) { // SC-36
	var lg, ss []string
	for i := 1; i <= 10; i++ {
		endedWithAgent("cook", i, "M", 10, &lg, &ss)
	}
	in := base(lines(append(lg, histLines(15, "-")...)...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ := Compute(in)
	if r.Scale.Source != "history" || r.Scale.N != 25 {
		t.Fatalf("scale = %+v", r.Scale)
	}
	if ke := r.Estimate.ByKind["cook"]; ke.N != 10 {
		t.Fatalf("estimate = %+v", r.Estimate.ByKind)
	}

	lg, ss = nil, nil
	for i := 1; i <= 25; i++ {
		endedWithAgent("cook", i, "M", 10, &lg, &ss)
	}
	in = base(lines(append(lg, histLines(15, "-")...)...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ = Compute(in)
	if r.Scale.Source != "learned" || r.Scale.N != 40 {
		t.Fatalf("scale = %+v", r.Scale)
	}
}

func TestHistoryCapsSampleAt60(t *testing.T) {
	r, _ := Compute(base(lines(histLines(80, "-")...)))
	if r.Scale.N != 60 {
		t.Fatalf("n = %d, want 60", r.Scale.N)
	}
}

func TestHistoryTokens(t *testing.T) {
	r, _ := Compute(base(lines(
		histRow(1, "fix", 3, "us.anthropic.claude-x:1m:100/10,opus:5/6"),
		histRow(2, "fix", 3, "opus:15/10"),
		histRow(3, "-", 3, "opus:1000/1000"),
	)))
	q, ok := r.Quality.ByKind["fix"]
	if !ok || q.N != 0 {
		t.Fatalf("quality = %+v", r.Quality.ByKind)
	}
	if got := q.TokensAvg["us.anthropic.claude-x:1m"]; got.In != 100 || got.Out != 10 {
		t.Fatalf("model with colon = %+v", q.TokensAvg)
	}
	if got := q.TokensAvg["opus"]; got.In != 10 || got.Out != 8 {
		t.Fatalf("opus avg = %+v", got)
	}
	if len(r.Quality.ByKind) != 1 || r.Malformed != 0 {
		t.Fatalf("kind - leaked: %+v malformed=%d", r.Quality.ByKind, r.Malformed)
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
