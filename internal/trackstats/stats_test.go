package trackstats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	today = "2026-10-30"
	sep   = " · "
)

var now = time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)

func row(ts time.Time, parts ...string) string {
	return ts.UTC().Format(time.RFC3339) + sep + strings.Join(parts, sep)
}

func lines(l ...string) string { return strings.Join(l, "\n") + "\n" }

func cid(kind string, n int) string { return fmt.Sprintf("%s-261030-%04x", kind, n) }

// agentTurn is a session line: a main turn of min minutes ending at end.
func agentTurn(end time.Time, id string, min int) string {
	return row(end, "turn", id, fmt.Sprint(min*60000), "main", "opus", "10", "20", "-", "-")
}

func base(log string) Input {
	return Input{Log: log, Today: today, Now: now, Sessions: map[string]string{}}
}

func TestTodayItemsAndRunning(t *testing.T) { // SC-3
	a, b := cid("cook", 1), cid("cook", 2)
	in := base(lines(
		row(now.Add(-3*time.Hour), "start", a, "cook", "repo", "M"),
		row(now.Add(-2*time.Hour), "end", a, "https://x/pr/1", "3", "1"),
		row(now.Add(-1*time.Hour), "start", b, "cook", "repo", "-"),
	))
	r, _ := Compute(in)
	if len(r.Today.Items) != 2 {
		t.Fatalf("today items = %d, want 2", len(r.Today.Items))
	}
	if len(r.Running) != 1 || r.Running[0].ID != b {
		t.Fatalf("running = %+v, want only %s", r.Running, b)
	}
	if r.Today.Started["cook"] != 2 || r.Today.Ended["cook"] != 1 || r.Today.Tasks != 3 || r.Today.Rounds != 1 || r.Today.PRsOpened != 1 {
		t.Fatalf("today counts = %+v", r.Today)
	}
	for _, it := range r.Today.Items {
		if it.ID == a && it.Status != "done" {
			t.Fatalf("status of ended = %q", it.Status)
		}
	}
}

func TestPlansToday(t *testing.T) { // SC-6
	root := t.TempDir()
	for p, body := range map[string]string{
		"a/2026-10-30-x.md": "", "b/2026-10-30-y.md": "", "a/2026-10-29-z.md": "",
	} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := base("")
	in.PlansRoot = root
	r, _ := Compute(in)
	if len(r.Today.Plans) != 2 || r.Today.Plans["a"] != 1 || r.Today.Plans["b"] != 1 {
		t.Fatalf("plans = %v, want a:1 b:1", r.Today.Plans)
	}
}

func prLog() string {
	return lines(
		row(now.Add(-4*time.Hour), "start", cid("fix", 1), "fix", "r", "S"),
		row(now.Add(-3*time.Hour), "end", cid("fix", 1), "https://x/pr/A", "1", "0"),
		row(now.Add(-2*time.Hour), "start", cid("fix", 2), "fix", "r", "S"),
		row(now.Add(-1*time.Hour), "end", cid("fix", 2), "https://x/pr/B", "1", "0"),
	)
}

func TestMergedError(t *testing.T) { // SC-7
	in := base(prLog())
	in.Merged = func(string) (string, error) { return "", errors.New("gh: boom") }
	r, cache := Compute(in)
	if !strings.Contains(r.Today.MergedError, "boom") {
		t.Fatalf("mergedError = %q", r.Today.MergedError)
	}
	if r.Today.PRsOpened != 2 || r.Today.Ended["fix"] != 2 {
		t.Fatalf("other numbers lost: %+v", r.Today)
	}
	if len(cache) != 0 {
		t.Fatalf("cache = %v, want empty", cache)
	}
}

func TestMergedCache(t *testing.T) { // SC-12
	asked := map[string]int{}
	in := base(prLog())
	in.Merged = func(u string) (string, error) {
		asked[u]++
		if u == "https://x/pr/A" {
			return "2026-10-30T08:00:00Z", nil
		}
		return "", nil
	}
	r, cache := Compute(in)
	if r.Today.Merged != 1 || r.Today.MergedError != "" {
		t.Fatalf("merged = %d err %q, want 1", r.Today.Merged, r.Today.MergedError)
	}
	if cache["https://x/pr/A"] != "2026-10-30T08:00:00Z" || len(cache) != 1 {
		t.Fatalf("cache = %v", cache)
	}
	in.Cache = cache
	r, _ = Compute(in)
	if asked["https://x/pr/A"] != 1 {
		t.Fatalf("A asked %d times, want 1", asked["https://x/pr/A"])
	}
	if asked["https://x/pr/B"] != 2 {
		t.Fatalf("B (unmerged) asked %d times, want 2", asked["https://x/pr/B"])
	}
	if r.Today.Merged != 1 {
		t.Fatalf("merged from cache = %d, want 1", r.Today.Merged)
	}
}

func TestMergedBeforeTodayNotCounted(t *testing.T) {
	in := base(prLog())
	in.Merged = func(string) (string, error) { return "2026-10-29T08:00:00Z", nil }
	r, _ := Compute(in)
	if r.Today.Merged != 0 {
		t.Fatalf("merged = %d, want 0", r.Today.Merged)
	}
}

func TestSessionBinding(t *testing.T) { // SC-11
	x, y := cid("cook", 1), cid("fix", 2)
	in := base(lines(
		row(now.Add(-2*time.Hour), "start", x, "cook", "r", "M"),
		row(now.Add(-2*time.Hour), "start", y, "fix", "r", "S"),
		row(now.Add(-2*time.Hour), "start", cid("fix", 3), "fix", "r", "S"),
	))
	in.Sessions = map[string]string{
		"A": lines(row(now.Add(-2*time.Hour), "cwd", "/w/alpha"), row(now.Add(-2*time.Hour), "bind", x)),
		"B": lines(row(now.Add(-2*time.Hour), "cwd", "/w/beta"), row(now.Add(-2*time.Hour), "bind", y)),
	}
	r, _ := Compute(in)
	got := map[string][2]string{}
	for _, it := range r.Running {
		got[it.ID] = [2]string{it.Session, it.Cwd}
	}
	if got[x] != [2]string{"A", "/w/alpha"} || got[y] != [2]string{"B", "/w/beta"} {
		t.Fatalf("sessions = %v", got)
	}
	if got[cid("fix", 3)][0] != "-" {
		t.Fatalf("unbound session = %q, want -", got[cid("fix", 3)][0])
	}
}

func TestEstEventAndEstimateInclusion(t *testing.T) { // SC-17
	a, b := cid("cook", 1), cid("cook", 2)
	in := base(lines(
		row(now.Add(-5*time.Hour), "start", a, "cook", "r", "-"),
		row(now.Add(-4*time.Hour), "est", a, "M"),
		row(now.Add(-3*time.Hour), "end", a, "https://x/pr/1", "1", "0"),
		row(now.Add(-5*time.Hour), "start", b, "cook", "r", "-"),
		row(now.Add(-3*time.Hour), "end", b, "https://x/pr/2", "1", "0"),
	))
	in.Sessions = map[string]string{"s": lines(agentTurn(now.Add(-3*time.Hour), a, 2), agentTurn(now.Add(-3*time.Hour), b, 2))}
	r, _ := Compute(in)
	est := map[string]string{}
	for _, it := range r.Today.Items {
		est[it.ID] = it.Est
	}
	if est[a] != "M" || est[b] != "-" {
		t.Fatalf("est = %v", est)
	}
	if n := r.Estimate.ByKind["cook"].N; n != 1 {
		t.Fatalf("estimate n = %d, want 1 (only the id with est)", n)
	}
}

func TestQualityShipFix(t *testing.T) { // SC-18, SC-25
	id := cid("fix", 1)
	in := base(lines(
		row(now.Add(-5*time.Hour), "start", id, "fix", "r", "S"),
		row(now.Add(-4*time.Hour), "ship", id, "blocked", "review", "1,0,0,0", "1"),
		row(now.Add(-3*time.Hour), "fix", id, "2", "wide", "test"),
		row(now.Add(-2*time.Hour), "ship", id, "shipped", "-", "-", "2"),
	))
	r, _ := Compute(in)
	q := r.Quality.ByKind["fix"]
	if q.ShipBlockedRate == nil || *q.ShipBlockedRate != 0.5 {
		t.Fatalf("shipBlockedRate = %v", q.ShipBlockedRate)
	}
	if q.TopBlockedStep == nil || *q.TopBlockedStep != "review" {
		t.Fatalf("topBlockedStep = %v", q.TopBlockedStep)
	}
	if q.FindingsAvg == nil || q.FindingsAvg.C != 1 || q.FindingsAvg.H != 0 {
		t.Fatalf("findingsAvg = %+v (the '-' ship must not count)", q.FindingsAvg)
	}
	if q.FixLoopAvg == nil || *q.FixLoopAvg != 1.5 {
		t.Fatalf("fixLoopAvg = %v, want 1.5", q.FixLoopAvg)
	}
	f := r.Quality.Fix
	if f.N != 1 || f.RoundAvg == nil || *f.RoundAvg != 2 || f.WideRate == nil || *f.WideRate != 1 || f.RegressionRate == nil || *f.RegressionRate != 1 {
		t.Fatalf("fix = %+v", f)
	}
}

func TestHotfixPushedEnters(t *testing.T) { // SC-23
	id := cid("hotfix", 1)
	in := base(lines(
		row(now.Add(-3*time.Hour), "start", id, "hotfix", "r", "S"),
		row(now.Add(-1*time.Hour), "end", id, "branch:hotfix/x", "1", "0"),
	))
	in.Sessions = map[string]string{"s": lines(agentTurn(now.Add(-1*time.Hour), id, 3))}
	r, _ := Compute(in)
	if len(r.Running) != 0 {
		t.Fatalf("running = %+v", r.Running)
	}
	if r.Today.Items[0].Status != "pushed" {
		t.Fatalf("status = %q", r.Today.Items[0].Status)
	}
	if r.Estimate.ByKind["hotfix"].N != 1 || r.Estimate.Hotfix.N != 1 {
		t.Fatalf("estimate = %+v", r.Estimate)
	}
}

func TestRollbackAndCancelled(t *testing.T) {
	a, b := cid("fix", 1), cid("fix", 2)
	in := base(lines(
		row(now.Add(-3*time.Hour), "start", a, "fix", "r", "S"),
		row(now.Add(-2*time.Hour), "end", a, "rollback", "1", "0"),
		row(now.Add(-3*time.Hour), "start", b, "fix", "r", "S"),
		row(now.Add(-2*time.Hour), "end", b, "cancelled", "-", "-"),
	))
	in.Sessions = map[string]string{"s": lines(agentTurn(now.Add(-2*time.Hour), a, 3), agentTurn(now.Add(-2*time.Hour), b, 3))}
	r, _ := Compute(in)
	st := map[string]string{}
	for _, it := range r.Today.Items {
		st[it.ID] = it.Status
	}
	if st[a] != "rollback" || st[b] != "cancelled" {
		t.Fatalf("status = %v", st)
	}
	if r.Estimate.ByKind["fix"].N != 1 {
		t.Fatalf("cancelled must not enter estimate: %+v", r.Estimate.ByKind["fix"])
	}
}

func TestCIRedFirst(t *testing.T) { // SC-27
	a, b := cid("fix", 1), cid("fix", 2)
	in := base(lines(
		row(now.Add(-6*time.Hour), "start", a, "fix", "r", "S"),
		row(now.Add(-5*time.Hour), "ci", a, "red"),
		row(now.Add(-4*time.Hour), "ci", a, "green"),
		row(now.Add(-6*time.Hour), "start", b, "fix", "r", "S"),
		row(now.Add(-4*time.Hour), "ci", b, "green"),
	))
	r, _ := Compute(in)
	q := r.Quality.ByKind["fix"]
	if q.CIRedFirstRate == nil || *q.CIRedFirstRate != 0.5 {
		t.Fatalf("ciRedFirstRate = %v", q.CIRedFirstRate)
	}
}

func TestStale(t *testing.T) {
	id := cid("cook", 1)
	in := base(row(now.Add(-8*24*time.Hour), "start", id, "cook", "r", "M") + "\n")
	r, _ := Compute(in)
	if len(r.Running) != 1 || r.Running[0].Status != "stale" {
		t.Fatalf("running = %+v", r.Running)
	}
	if len(r.Estimate.ByKind) != 0 {
		t.Fatalf("stale entered estimate: %+v", r.Estimate.ByKind)
	}
	fresh := base(row(now.Add(-6*24*time.Hour), "start", id, "cook", "r", "M") + "\n")
	r, _ = Compute(fresh)
	if r.Running[0].Status != "running" {
		t.Fatalf("status = %q", r.Running[0].Status)
	}
}

// endedWithAgent logs a started+ended id whose single main turn lasts min minutes.
func endedWithAgent(kind string, n int, est string, min int, log *[]string, sess *[]string) {
	id := cid(kind, n)
	s := now.Add(-time.Duration(100-n) * time.Minute)
	e := s.Add(time.Duration(min) * time.Minute)
	*log = append(*log, row(s, "start", id, kind, "r", est), row(e, "end", id, "https://x/pr/"+id, "1", "0"))
	*sess = append(*sess, row(s, "bind", id), agentTurn(e, id, min))
}

func TestEstimateAccuracyAndRules(t *testing.T) {
	var lg, ss []string
	// default scale S<15 M<45 L<120: 5 min => S, 30 min => M, 60 min => L
	endedWithAgent("cook", 1, "M", 30, &lg, &ss) // hit
	endedWithAgent("cook", 2, "S", 30, &lg, &ss) // low
	endedWithAgent("cook", 3, "L", 5, &lg, &ss)  // high
	endedWithAgent("fix", 4, "S", 30, &lg, &ss)
	endedWithAgent("fix", 5, "S", 30, &lg, &ss)
	endedWithAgent("fix", 6, "S", 60, &lg, &ss)
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	c := r.Estimate.ByKind["cook"]
	if c.N != 3 || c.Hit != 1 || c.Low != 1 || c.High != 1 {
		t.Fatalf("cook = %+v", c)
	}
	if r.Scale.Source != "default" || r.Scale.S != 15 || r.Scale.M != 45 || r.Scale.L != 120 {
		t.Fatalf("scale = %+v", r.Scale)
	}
	if len(r.Rules) != 1 || r.Rules[0] != "fix: est up 1 size" {
		t.Fatalf("rules = %v", r.Rules)
	}
	share := r.Estimate.PhaseShare["cook"]
	if share["design"] != 1 {
		t.Fatalf("phaseShare = %v", share)
	}
}

func TestRuleDown(t *testing.T) {
	var lg, ss []string
	for i := 1; i <= 3; i++ {
		endedWithAgent("cook", i, "L", 2, &lg, &ss)
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	if len(r.Rules) != 1 || r.Rules[0] != "cook: est down 1 size" {
		t.Fatalf("rules = %v", r.Rules)
	}
}

func TestLearnedScale(t *testing.T) {
	var lg, ss []string
	for i := 1; i <= 20; i++ {
		endedWithAgent("fix", i, "S", i, &lg, &ss) // 1..20 min
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	// nearest rank: p25 of 1..20 = 5, p50 = 10, p90 = 18
	if r.Scale.Source != "learned" || r.Scale.N != 20 || r.Scale.S != 5 || r.Scale.M != 10 || r.Scale.L != 18 {
		t.Fatalf("scale = %+v", r.Scale)
	}
}

func TestHotfixMedianP90(t *testing.T) {
	var lg, ss []string
	for i := 1; i <= 10; i++ {
		endedWithAgent("hotfix", i, "S", i, &lg, &ss)
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	h := r.Estimate.Hotfix
	if h.N != 10 || h.MedianMin == nil || *h.MedianMin != 5 || h.P90Min == nil || *h.P90Min != 9 {
		t.Fatalf("hotfix = %+v", h)
	}
}

func TestMalformedCounted(t *testing.T) {
	id := cid("cook", 1)
	in := base(lines(
		row(now, "start", id, "cook", "r", "M"),
		"garbage line",
	))
	in.Sessions = map[string]string{"s": lines(row(now, "bind", id), "bad session line")}
	r, _ := Compute(in)
	if r.Malformed != 2 {
		t.Fatalf("malformed = %d, want 2", r.Malformed)
	}
}

func TestJSONShape(t *testing.T) {
	r, _ := Compute(base(""))
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"enabled", "malformed", "scale", "rules", "today", "running", "estimate", "quality", "kitchen"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in %s", k, b)
		}
	}
	if !strings.Contains(string(m["running"]), "[]") || !strings.Contains(string(m["rules"]), "[]") {
		t.Fatalf("running/rules must be [] not null: %s", b)
	}
	if !strings.Contains(string(m["kitchen"]), `"cooked":[]`) || !strings.Contains(string(m["kitchen"]), `"rollingBoil":[]`) {
		t.Fatalf("kitchen lists must be [] not null: %s", m["kitchen"])
	}
	// a kind with no ship lines marshals its ratios as null
	id := cid("fix", 1)
	r, _ = Compute(base(row(now, "start", id, "fix", "r", "S") + "\n"))
	b, _ = json.Marshal(r.Quality.ByKind["fix"])
	for _, want := range []string{`"shipBlockedRate":null`, `"topBlockedStep":null`, `"findingsAvg":null`, `"ciRedFirstRate":null`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("want %s in %s", want, b)
		}
	}
}

func TestZeroWallNotSampled(t *testing.T) {
	var lg, ss []string
	endedWithAgent("hotfix", 1, "S", 10, &lg, &ss)
	id := cid("hotfix", 2) // ended in the same instant: zero wall time
	lg = append(lg, row(now.Add(-50*time.Minute), "start", id, "hotfix", "r", "S"), row(now.Add(-50*time.Minute), "end", id, "branch:x", "1", "0"))
	for i := 3; i <= 5; i++ { // zero-wall cooks with est L would fire "est down"
		c := cid("cook", i)
		lg = append(lg, row(now.Add(-50*time.Minute), "start", c, "cook", "r", "L"), row(now.Add(-50*time.Minute), "end", c, "branch:x", "1", "0"))
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	h := r.Estimate.Hotfix
	if h.N != 1 || h.MedianMin == nil || *h.MedianMin != 10 {
		t.Fatalf("hotfix = %+v", h)
	}
	if _, ok := r.Estimate.ByKind["cook"]; ok {
		t.Fatalf("unmeasured cook in estimate: %+v", r.Estimate.ByKind["cook"])
	}
	if len(r.Rules) != 0 {
		t.Fatalf("rules = %v", r.Rules)
	}
}

func TestRepeatedStartCounted(t *testing.T) {
	id := cid("cook", 1)
	r, _ := Compute(base(lines(
		row(now.Add(-2*time.Hour), "start", id, "cook", "first", "M"),
		row(now.Add(-1*time.Hour), "start", id, "cook", "second", "L"),
	)))
	if r.Malformed != 1 || r.Running[0].Repo != "first" {
		t.Fatalf("malformed %d repo %q", r.Malformed, r.Running[0].Repo)
	}
}

func TestUnknownEndOutcomeCounted(t *testing.T) {
	id := cid("cook", 1)
	r, _ := Compute(base(lines(
		row(now.Add(-2*time.Hour), "start", id, "cook", "r", "M"),
		row(now.Add(-1*time.Hour), "end", id, "weird", "1", "0"),
	)))
	if r.Malformed != 1 || len(r.Running) != 1 || r.Running[0].Status != "running" {
		t.Fatalf("malformed %d running %+v", r.Malformed, r.Running)
	}
}

func TestRunGhTimeout(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	start := time.Now()
	_, err := runGh(bin, 200*time.Millisecond, "https://x/pr/1")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
}

func TestParseMergedAt(t *testing.T) {
	if got, err := parseMergedAt([]byte(`{"mergedAt":"2026-10-30T08:00:00Z"}`)); err != nil || got != "2026-10-30T08:00:00Z" {
		t.Fatalf("got %q %v", got, err)
	}
	if got, err := parseMergedAt([]byte(`{"mergedAt":null}`)); err != nil || got != "" {
		t.Fatalf("null: got %q %v", got, err)
	}
	if _, err := parseMergedAt([]byte(`nope`)); err == nil {
		t.Fatal("want error on bad json")
	}
}

// One PR's gh error must not hide the PRs after it; a missing gh stops the loop.
func TestMergedErrorDoesNotStarveLaterPRs(t *testing.T) {
	in := base(prLog())
	in.Merged = func(u string) (string, error) {
		if u == "https://x/pr/A" {
			return "", errors.New("gh: no access")
		}
		return "2026-10-30T08:00:00Z", nil
	}
	r, cache := Compute(in)
	if r.Today.Merged != 1 || !strings.Contains(r.Today.MergedError, "no access") || cache["https://x/pr/B"] == "" {
		t.Fatalf("merged %d err %q cache %v, want B merged and A's error kept", r.Today.Merged, r.Today.MergedError, cache)
	}
	asked := 0
	in.Merged = func(string) (string, error) { asked++; return "", exec.ErrNotFound }
	if r, _ := Compute(in); asked != 1 || r.Today.MergedError == "" {
		t.Fatalf("missing gh: asked %d err %q, want one ask and an error", asked, r.Today.MergedError)
	}
}

// A PR closed without a merge is cached and never counted as merged.
func TestMergedClosedUnmergedCached(t *testing.T) {
	in := base(prLog())
	in.Merged = func(string) (string, error) { return ClosedUnmerged, nil }
	r, cache := Compute(in)
	if r.Today.Merged != 0 || r.Malformed != 0 || cache["https://x/pr/A"] != ClosedUnmerged {
		t.Fatalf("merged %d malformed %d cache %v", r.Today.Merged, r.Malformed, cache)
	}
	if got, err := parseMergedAt([]byte(`{"mergedAt":null,"state":"CLOSED"}`)); err != nil || got != ClosedUnmerged {
		t.Fatalf("closed: got %q %v", got, err)
	}
}

// Two ends naming the same PR (a CI fix pushes to the open PR) count it once.
func TestPRsOpenedCountsAPROnce(t *testing.T) {
	in := base(lines(
		row(now.Add(-4*time.Hour), "start", cid("fix", 1), "fix", "r", "S"),
		row(now.Add(-3*time.Hour), "end", cid("fix", 1), "https://x/pr/A", "1", "0"),
		row(now.Add(-2*time.Hour), "start", cid("fix", 2), "fix", "r", "S"),
		row(now.Add(-1*time.Hour), "end", cid("fix", 2), "https://x/pr/A", "1", "0"),
	))
	if r, _ := Compute(in); r.Today.PRsOpened != 1 {
		t.Fatalf("prsOpened = %d, want 1", r.Today.PRsOpened)
	}
}

// FR-11.7 (5): prsWaiting counts tracked PRs not known as merged or closed.
func TestPRsWaiting(t *testing.T) {
	in := base(prLog())
	in.Merged = func(u string) (string, error) {
		if u == "https://x/pr/A" {
			return "2026-10-30T08:00:00Z", nil
		}
		return "", nil
	}
	if r, _ := Compute(in); r.Today.PRsWaiting != 1 {
		t.Fatalf("prsWaiting = %d, want 1 (B still open)", r.Today.PRsWaiting)
	}
	in.Merged = func(string) (string, error) { return ClosedUnmerged, nil }
	if r, _ := Compute(in); r.Today.PRsWaiting != 0 {
		t.Fatalf("prsWaiting = %d, want 0 (both closed)", r.Today.PRsWaiting)
	}
}

// A CI-fix run that ends the next day on the same PR does not count it as opened again.
func TestPRsOpenedOnFirstEndDayOnly(t *testing.T) {
	in := base(lines(
		row(now.Add(-30*time.Hour), "start", cid("fix", 1), "fix", "r", "S"),
		row(now.Add(-26*time.Hour), "end", cid("fix", 1), "https://x/pr/A", "1", "0"),
		row(now.Add(-2*time.Hour), "start", cid("fix", 2), "fix", "r", "S"),
		row(now.Add(-1*time.Hour), "end", cid("fix", 2), "https://x/pr/A", "1", "0"),
	))
	if r, _ := Compute(in); r.Today.PRsOpened != 0 {
		t.Fatalf("prsOpened = %d, want 0 (A was first ended yesterday)", r.Today.PRsOpened)
	}
}

// A gh timeout stops the merged loop like a missing gh: N slow PRs never add up.
func TestMergedTimeoutStopsTheLoop(t *testing.T) {
	asked := 0
	in := base(prLog())
	in.Merged = func(string) (string, error) { asked++; return "", fmt.Errorf("gh pr view: %w", ErrGhTimeout) }
	if r, _ := Compute(in); asked != 1 || r.Today.MergedError == "" {
		t.Fatalf("asked %d err %q, want one ask and an error", asked, r.Today.MergedError)
	}
}

func TestWallMin(t *testing.T) { // SC-1
	a, b := cid("fix", 1), cid("fix", 2)
	s := time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)
	in := base(lines(
		row(s, "start", a, "fix", "r", "S"),
		row(s.Add(31*time.Minute), "end", a, "https://x/pr/1", "1", "0"),
		row(now.Add(-12*time.Minute-time.Second), "start", b, "fix", "r", "S"),
	))
	r, _ := Compute(in)
	got := map[string]int{}
	for _, it := range r.Today.Items {
		got[it.ID] = it.WallMin
	}
	if got[a] != 31 || got[b] != 13 {
		t.Fatalf("wallMin = %v, want 31 and 13 (ceil)", got)
	}
}

func TestTodayDoneExcludesCancelled(t *testing.T) { // SC-2
	a, b, c, d := cid("fix", 1), cid("hotfix", 2), cid("fix", 3), cid("fix", 4)
	s := now.Add(-3 * time.Hour)
	in := base(lines(
		row(s, "start", a, "fix", "r", "S"),
		row(s.Add(20*time.Minute), "end", a, "https://x/pr/1", "1", "0"),
		row(s, "start", b, "hotfix", "r", "S"),
		row(s.Add(10*time.Minute), "end", b, "branch:hotfix/x", "1", "0"),
		row(s, "start", c, "fix", "r", "S"),
		row(s.Add(5*time.Minute), "end", c, "cancelled", "-", "-"),
		row(s, "start", d, "fix", "r", "S"),
		row(s.Add(40*time.Minute), "end", d, "rollback", "1", "0"),
	))
	in.Sessions = map[string]string{"s": lines(
		row(s, "bind", a), agentTurn(s.Add(20*time.Minute), a, 6),
		row(s, "bind", d), agentTurn(s.Add(40*time.Minute), d, 9),
	)}
	r, _ := Compute(in)
	td := r.Today
	if len(td.Done) != 2 || td.Done["fix"] != 1 || td.Done["hotfix"] != 1 {
		t.Fatalf("done = %v", td.Done)
	}
	if len(td.Cancelled) != 1 || td.Cancelled["fix"] != 1 || len(td.RolledBack) != 1 || td.RolledBack["fix"] != 1 {
		t.Fatalf("cancelled = %v rolledBack = %v", td.Cancelled, td.RolledBack)
	}
	if td.Ended["fix"] != 3 || td.Ended["hotfix"] != 1 {
		t.Fatalf("ended = %v, want fix 3 (done, cancelled, rollback) and hotfix 1", td.Ended)
	}
	if td.WallMin != 30 {
		t.Fatalf("today wallMin = %d, want 30 (20+10; cancelled 5 and rollback 40 excluded)", td.WallMin)
	}
	if td.AgentMin != 6 {
		t.Fatalf("today agentMin = %d, want 6 (done item only; rollback 9 excluded)", td.AgentMin)
	}
}

// realItems logs n done fix items with est S whose wall times are 1..n minutes.
func realItems(n int) []string {
	var lg, ss []string
	for i := 1; i <= n; i++ {
		endedWithAgent("fix", i, "S", i, &lg, &ss)
	}
	return lg
}

func TestScaleDefaultUntil20Real(t *testing.T) { // SC-3
	r, _ := Compute(base(lines(realItems(19)...)))
	if r.Scale != (Scale{Source: "default", N: 19, S: 15, M: 45, L: 120}) {
		t.Fatalf("19 items: scale = %+v", r.Scale)
	}
	r, _ = Compute(base(lines(realItems(20)...)))
	if r.Scale != (Scale{Source: "learned", N: 20, S: 5, M: 10, L: 18}) {
		t.Fatalf("20 items: scale = %+v", r.Scale)
	}
	r, _ = Compute(base(lines(histLines(300, "fix")...)))
	if r.Scale != (Scale{Source: "default", N: 0, S: 15, M: 45, L: 120}) {
		t.Fatalf("history only: scale = %+v", r.Scale)
	}
}

func TestEstimateOnWall(t *testing.T) { // SC-4
	var lg, ss []string
	id := cid("cook", 1)
	s := now.Add(-2 * time.Hour)
	lg = append(lg, row(s, "start", id, "cook", "r", "M"), row(s.Add(50*time.Minute), "end", id, "https://x/pr/1", "1", "0"))
	ss = append(ss, row(s, "bind", id), agentTurn(s.Add(50*time.Minute), id, 8))
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ := Compute(in)
	// wall 50 on the default scale is L (45..120), agent 8 would be S: est M is under.
	if ke := r.Estimate.ByKind["cook"]; ke.N != 1 || ke.Low != 1 || ke.Hit != 0 || ke.High != 0 {
		t.Fatalf("cook = %+v", ke)
	}
}

func TestHotfixMedianOnWall(t *testing.T) {
	var lg, ss []string
	for i, w := range []int{10, 20, 30, 40, 50} {
		id := cid("hotfix", i+1)
		s := now.Add(-time.Duration(200-i) * time.Minute)
		lg = append(lg, row(s, "start", id, "hotfix", "r", "S"), row(s.Add(time.Duration(w)*time.Minute), "end", id, "branch:x", "1", "0"))
		ss = append(ss, row(s, "bind", id), agentTurn(s.Add(time.Minute), id, 1))
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ := Compute(in)
	if h := r.Estimate.Hotfix; h.N != 5 || h.MedianMin == nil || *h.MedianMin != 30 {
		t.Fatalf("hotfix = %+v", h)
	}
}

func TestQualityIgnoresHistory(t *testing.T) {
	var lg, ss []string
	endedWithAgent("fix", 1, "S", 5, &lg, &ss)
	lg = append(lg, histRow(1, "fix", 3, "opus:100/10"), histRow(2, "fix", 4, "opus:300/30"))
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ := Compute(in)
	q := r.Quality.ByKind["fix"]
	if q.N != 1 || len(r.Quality.ByKind) != 1 {
		t.Fatalf("quality = %+v", r.Quality.ByKind)
	}
	if got := q.TokensAvg["opus"]; got.In != 10 || got.Out != 20 {
		t.Fatalf("opus avg = %+v, want the real item only (10/20)", got)
	}
}

// wallItem logs a fix item that ends wall minutes after it starts with the given outcome.
func wallItem(n int, est string, wall int, outcome string, lg *[]string) {
	id := cid("fix", n)
	st := now.Add(-time.Duration(1000-n) * time.Minute)
	*lg = append(*lg, row(st, "start", id, "fix", "r", est), row(st.Add(time.Duration(wall)*time.Minute), "end", id, outcome, "1", "0"))
}

func TestScaleFromWallNotAgent(t *testing.T) { // review: wall must differ from agent
	var lg, ss []string
	for i := 1; i <= 20; i++ {
		id := cid("fix", i)
		st := now.Add(-time.Duration(1000-i) * time.Minute)
		en := st.Add(time.Duration(i) * time.Minute)
		lg = append(lg, row(st, "start", id, "fix", "r", "S"), row(en, "end", id, "https://x/pr/"+id, "1", "0"))
		ss = append(ss, row(st, "bind", id), agentTurn(en, id, 1))
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ := Compute(in)
	if r.Scale != (Scale{Source: "learned", N: 20, S: 5, M: 10, L: 18}) {
		t.Fatalf("scale = %+v, want wall-based 5/10/18 (agent is 1 min each)", r.Scale)
	}
}

func TestScaleSampleFilter(t *testing.T) {
	var lg []string
	for i := 1; i <= 20; i++ { // valid: wall 1..20, odd ones pushed, even ones done
		out := "https://x/pr/" + cid("fix", i)
		if i%2 == 1 {
			out = "branch:x"
		}
		wallItem(i, "S", i, out, &lg)
	}
	wallItem(21, "XL", 500, "https://x/pr/xl", &lg)   // XL est
	wallItem(22, "S", 500, "rollback", &lg)           // rolled back
	wallItem(23, "S", 500, "cancelled", &lg)          // cancelled
	wallItem(24, "S", 0, "https://x/pr/zero", &lg)    // zero wall
	wallItem(25, "-", 500, "https://x/pr/noest", &lg) // no est
	r, _ := Compute(base(lines(lg...)))
	if r.Scale != (Scale{Source: "learned", N: 20, S: 5, M: 10, L: 18}) {
		t.Fatalf("scale = %+v, want N 20 and 5/10/18 from the valid items only", r.Scale)
	}
}

func TestScaleCapsAtNewest60(t *testing.T) {
	var lg, ss []string
	for i := 1; i <= 61; i++ { // wall = i, oldest end first
		endedWithAgent("fix", i, "S", i, &lg, &ss)
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s": lines(ss...)}
	r, _ := Compute(in)
	// newest 60 are walls 2..61 (nearest rank: index 14, 29, 53)
	if r.Scale != (Scale{Source: "learned", N: 60, S: 16, M: 31, L: 55}) {
		t.Fatalf("scale = %+v", r.Scale)
	}
}

func TestParseTaskID(t *testing.T) {
	if !idRe.MatchString("task-261111-0a1b") {
		t.Fatal("idRe must match a task id")
	}
}

func TestStatsTaskDone(t *testing.T) { // SC-5
	var lg, ss []string
	for i := 1; i <= 6; i++ {
		endedWithAgent("cook", i, "M", 20+i, &lg, &ss)
	}
	without, _ := Compute(func() Input { in := base(lines(lg...)); in.Sessions = map[string]string{"s1": lines(ss...)}; return in }())

	tid := "task-261030-0a1b"
	lg = append(lg,
		row(now.Add(-300*time.Minute), "start", tid, "task", "infra", "S"),
		row(now.Add(-200*time.Minute), "end", tid, "done", "0", "0"),
	)
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	var got *Item
	for i := range r.Today.Items {
		if r.Today.Items[i].ID == tid {
			got = &r.Today.Items[i]
		}
	}
	if got == nil || got.Status != "done" {
		t.Fatalf("task item = %+v", got)
	}
	if r.Today.Done["task"] != 1 {
		t.Fatalf("Done = %v", r.Today.Done)
	}
	if r.Scale != without.Scale {
		t.Fatalf("scale %+v != %+v", r.Scale, without.Scale)
	}
}

func TestStatsTaskEstimate(t *testing.T) { // SC-6
	var lg, ss []string
	endedWithAgent("cook", 1, "M", 30, &lg, &ss)
	endedWithAgent("fix", 2, "S", 10, &lg, &ss)
	in0 := base(lines(lg...))
	in0.Sessions = map[string]string{"s1": lines(ss...)}
	without, _ := Compute(in0)

	for i, m := range []int{10, 20, 30, 40, 50} {
		id := fmt.Sprintf("task-261030-%04x", i+1)
		s := now.Add(-time.Duration(200-i*10) * time.Minute)
		lg = append(lg, row(s, "start", id, "task", "infra", "-"), row(s.Add(time.Duration(m)*time.Minute), "end", id, "done", "0", "0"))
	}
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	h := r.Estimate.Task
	if h.N != 5 || h.MedianMin == nil || *h.MedianMin != 30 || h.P90Min == nil || *h.P90Min != 50 {
		t.Fatalf("task = %+v", h)
	}
	if _, ok := r.Estimate.ByKind["task"]; ok {
		t.Fatal("ByKind has task")
	}
	if !reflect.DeepEqual(r.Estimate.ByKind, without.Estimate.ByKind) || !reflect.DeepEqual(r.Rules, without.Rules) {
		t.Fatalf("ByKind/Rules changed: %+v %v", r.Estimate.ByKind, r.Rules)
	}
}
