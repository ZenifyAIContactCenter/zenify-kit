package trackstats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// default scale S<5 M<20 L<60: 10 min => M, 2 min => S, 30 min => L
	endedWithAgent("cook", 1, "M", 10, &lg, &ss) // hit
	endedWithAgent("cook", 2, "S", 10, &lg, &ss) // low
	endedWithAgent("cook", 3, "L", 2, &lg, &ss)  // high
	endedWithAgent("fix", 4, "S", 10, &lg, &ss)
	endedWithAgent("fix", 5, "S", 10, &lg, &ss)
	endedWithAgent("fix", 6, "S", 10, &lg, &ss)
	in := base(lines(lg...))
	in.Sessions = map[string]string{"s1": lines(ss...)}
	r, _ := Compute(in)
	c := r.Estimate.ByKind["cook"]
	if c.N != 3 || c.Hit != 1 || c.Low != 1 || c.High != 1 {
		t.Fatalf("cook = %+v", c)
	}
	if r.Scale.Source != "default" || r.Scale.S != 5 || r.Scale.M != 20 || r.Scale.L != 60 {
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
	for _, k := range []string{"enabled", "malformed", "scale", "rules", "today", "running", "estimate", "quality"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in %s", k, b)
		}
	}
	if !strings.Contains(string(m["running"]), "[]") || !strings.Contains(string(m["rules"]), "[]") {
		t.Fatalf("running/rules must be [] not null: %s", b)
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

func TestUnmeasuredNotSampled(t *testing.T) {
	var lg, ss []string
	endedWithAgent("hotfix", 1, "S", 10, &lg, &ss)
	id := cid("hotfix", 2) // ended, no session turns
	lg = append(lg, row(now.Add(-50*time.Minute), "start", id, "hotfix", "r", "S"), row(now.Add(-40*time.Minute), "end", id, "branch:x", "1", "0"))
	for i := 3; i <= 5; i++ { // unmeasured cooks with est L would fire "est down"
		c := cid("cook", i)
		lg = append(lg, row(now.Add(-50*time.Minute), "start", c, "cook", "r", "L"), row(now.Add(-40*time.Minute), "end", c, "branch:x", "1", "0"))
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
