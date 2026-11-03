package trackstats

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

var kNow = time.Date(2026, 11, 2, 10, 0, 0, 0, time.FixedZone("ICT", 7*3600))

const kToday = "2026-11-02"

func kInput(log string) Input {
	return Input{Log: log, Today: kToday, Now: kNow, Sessions: map[string]string{}, Workspace: "/ws"}
}

func kid(kind string, n int) string { return fmt.Sprintf("%s-261102-%04x", kind, n) }

func kAt(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, kNow.Location()) }

func TestKitchenCookedOrderAndStatus(t *testing.T) {
	a, b, c, d, e := kid("cook", 1), kid("cook", 2), kid("fix", 3), kid("cook", 4), kid("cook", 5)
	log := lines(
		row(kAt(8, 0), "start", a, "cook", "r", "M"),
		row(kAt(9, 0), "end", a, "https://x/pr/1,https://x/pr/2", "3", "1"),
		row(kAt(8, 0), "start", b, "cook", "r", "M"),
		row(kAt(9, 30), "end", b, "branch:feat/x", "3", "1"),
		row(kAt(7, 0), "start", c, "fix", "r", "S"),
		row(kAt(8, 0), "end", c, "cancelled", "-", "-"),
		row(kAt(10, 0).AddDate(0, 0, -1), "start", d, "cook", "r", "M"),
		row(kAt(10, 0).AddDate(0, 0, -1).Add(time.Hour), "end", d, "https://x/pr/9", "1", "0"),
		row(kAt(9, 0), "start", e, "cook", "r", "M"),
	)
	r, _ := Compute(kInput(log))
	got := r.Kitchen.Cooked
	if len(got) != 3 {
		t.Fatalf("cooked = %+v", got)
	}
	if got[0].ID != b || got[1].ID != a || got[2].ID != c {
		t.Fatalf("order = %s %s %s", got[0].ID, got[1].ID, got[2].ID)
	}
	if got[0].Status != "pushed" || got[1].Status != "done" || got[2].Status != "cancelled" {
		t.Fatalf("status = %+v", got)
	}
	if len(got[1].PRs) != 2 || got[2].PRs == nil || len(got[2].PRs) != 0 {
		t.Fatalf("prs = %+v / %#v", got[1].PRs, got[2].PRs)
	}
}

// The Vietnamese text is written with \u escapes to keep the langgate test clean.
func TestKitchenBriefPlan(t *testing.T) { // SC-5
	a := kid("cook", 1)
	log := lines(
		row(kAt(8, 0), "start", a, "cook", "zenify-kit", "M"),
		row(kAt(9, 0), "end", a, "branch:x", "1", "0", "p.md"),
	)
	in := kInput(log)
	in.PlanTitle = func(p string) (string, error) {
		if p != "p.md" {
			t.Fatalf("path = %q", p)
		}
		return "Dashboard Capacity: s\u1ed1 ch\u00ednh x\u00e1c — Implementation Plan", nil
	}
	r, _ := Compute(in)
	if len(r.Kitchen.Cooked) != 1 || r.Kitchen.Cooked[0].Brief != "Dashboard Capacity: s\u1ed1 ch\u00ednh x\u00e1c" {
		t.Fatalf("cooked = %+v", r.Kitchen.Cooked)
	}
}

func TestKitchenBriefOrder(t *testing.T) { // SC-6
	bid := "b-261102-0001"
	url := "https://github.com/o/r/issues/5"
	a, b, c, d, e := kid("cook", 1), kid("cook", 2), kid("cook", 3), kid("cook", 4), kid("cook", 5)
	prURL := "https://x/pr/7"
	log := lines(
		row(kAt(7, 0), "backlog", bid, "r", "M", "-", "T"),
		row(kAt(8, 0), "start", a, "cook", "zenify-kit", "M", bid),
		row(kAt(9, 0), "end", a, "branch:x", "1", "0"),
		row(kAt(8, 0), "start", b, "cook", "zenify-kit", "M", url),
		row(kAt(9, 1), "end", b, "branch:x", "1", "0"),
		row(kAt(8, 0), "start", c, "cook", "zenify-kit", "M"),
		row(kAt(9, 2), "end", c, prURL, "1", "0"),
		row(kAt(8, 0), "start", d, "cook", "zenify-kit", "M"),
		row(kAt(9, 3), "end", d, "branch:x", "1", "0"),
		row(kAt(8, 0), "start", e, "fix", "other", "M"),
		row(kAt(9, 4), "end", e, "branch:x", "1", "0", "gone.md"),
	)
	calls := 0
	in := kInput(log)
	in.GitHub = func() ([]Issue, error) { return []Issue{{URL: url, Repo: "r", Title: "I", CreatedAt: kAt(1, 0)}}, nil }
	in.PlanTitle = func(string) (string, error) { return "", nil }
	in.PRTitles = map[string]string{}
	in.PRTitle = func(u string) (string, error) {
		calls++
		if u != prURL {
			t.Fatalf("url = %q", u)
		}
		return "P", nil
	}
	r, _ := Compute(in)
	br := map[string]string{}
	for _, it := range r.Kitchen.Cooked {
		br[it.ID] = it.Brief
	}
	want := map[string]string{a: "T", b: "I", c: "P", d: "cook zenify-kit", e: "fix other"}
	for id, w := range want {
		if br[id] != w {
			t.Fatalf("brief[%s] = %q, want %q", id, br[id], w)
		}
	}
	if calls != 1 || in.PRTitles[prURL] != "P" {
		t.Fatalf("calls = %d, cache = %v", calls, in.PRTitles)
	}
	Compute(in)
	if calls != 1 {
		t.Fatalf("second run asked gh again: %d", calls)
	}
}

func TestKitchenPRTitleFetchCap(t *testing.T) {
	var logLines []string
	for i := 1; i <= 5; i++ {
		id := kid("cook", i)
		logLines = append(logLines,
			row(kAt(8, 0), "start", id, "cook", "r", "M"),
			row(kAt(9, i), "end", id, fmt.Sprintf("https://x/pr/%d", i), "1", "0"))
	}
	in := kInput(lines(logLines...))
	in.PRTitles = map[string]string{}
	calls := 0
	in.PRTitle = func(string) (string, error) { calls++; return "P", nil }
	r, _ := Compute(in)
	if calls != maxTitleFetch || maxTitleFetch != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	titled := 0
	for _, it := range r.Kitchen.Cooked {
		switch it.Brief {
		case "P":
			titled++
		case "cook r":
		default:
			t.Fatalf("brief = %q", it.Brief)
		}
	}
	if titled != maxTitleFetch {
		t.Fatalf("titled = %d, want %d", titled, maxTitleFetch)
	}
	Compute(in)
	if calls != 5 {
		t.Fatalf("second run calls = %d, want 5 (the 2 left over)", calls)
	}
}

func TestKitchenTitleError(t *testing.T) {
	a, b := kid("cook", 1), kid("cook", 2)
	log := lines(
		row(kAt(8, 0), "start", a, "cook", "zenify-kit", "M"),
		row(kAt(9, 0), "end", a, "https://x/pr/1", "1", "0"),
		row(kAt(8, 0), "start", b, "fix", "r", "M"),
		row(kAt(9, 1), "end", b, "branch:x", "1", "0", "p.md"),
	)
	in := kInput(log)
	in.PRTitle = func(string) (string, error) { return "", errors.New("boom") }
	in.PlanTitle = func(string) (string, error) { return "", errors.New("denied") }
	r, _ := Compute(in)
	if r.Kitchen.TitleError == "" {
		t.Fatal("no titleError")
	}
	br := map[string]string{}
	for _, it := range r.Kitchen.Cooked {
		br[it.ID] = it.Brief
	}
	if br[a] != "cook zenify-kit" || br[b] != "fix r" {
		t.Fatalf("briefs = %v", br)
	}
	// errors arise in log order (a's PR, then b's plan); the first one is kept
	if want := "pr title https://x/pr/1: boom"; r.Kitchen.TitleError != want {
		t.Fatalf("titleError = %q, want %q", r.Kitchen.TitleError, want)
	}
	in.PRTitle = nil // only the plan lookup fails now
	r, _ = Compute(in)
	if want := "plan title p.md: denied"; r.Kitchen.TitleError != want {
		t.Fatalf("plan-only titleError = %q, want %q", r.Kitchen.TitleError, want)
	}
}

func lf(session, json string, beat time.Time) LiveFile {
	return LiveFile{Session: session, JSON: json, BeatAt: beat}
}

func liveJSON(state, since, cwd, id string) string {
	return `{"state":"` + state + `","since":"` + since + `","cwd":"` + cwd + `","id":"` + id + `"}`
}

func TestKitchenRollingBoil(t *testing.T) { // SC-4
	in := kInput("")
	sA := kAt(9, 58).Format(time.RFC3339)
	in.Live = []LiveFile{
		lf("A", liveJSON("ask", sA, "/tmp/a", ""), kNow.Add(-5*time.Second)),
		lf("B", liveJSON("permission", sA, "/tmp/b", ""), kNow.Add(-31*time.Second)),
		lf("C", liveJSON("ask", sA, "/tmp/c", ""), time.Time{}),
		lf("D", liveJSON("busy", sA, "/tmp/d", ""), kNow),
	}
	r, _ := Compute(in)
	rb := r.Kitchen.RollingBoil
	if len(rb) != 1 || rb[0].Session != "A" || rb[0].State != "ask" || rb[0].Since != sA {
		t.Fatalf("rb = %+v", rb)
	}
	sE := kAt(9, 50).Format(time.RFC3339)
	in.Live = append(in.Live, lf("E", liveJSON("permission", sE, "/tmp/e", ""), kNow))
	r, _ = Compute(in)
	rb = r.Kitchen.RollingBoil
	if len(rb) != 2 || rb[0].Session != "E" || rb[1].Session != "A" {
		t.Fatalf("rb = %+v", rb)
	}
	if r.Malformed != 0 {
		t.Fatalf("malformed = %d", r.Malformed)
	}
}

func TestKitchenBoilBrief(t *testing.T) { // FR-3.4
	a := kid("cook", 1)
	log := lines(
		row(kAt(8, 0), "start", a, "cook", "zenify-kit", "M"),
		row(kAt(9, 0), "end", a, "branch:x", "1", "0", "p.md"),
	)
	in := kInput(log)
	in.PlanTitle = func(string) (string, error) { return "Plan T — Implementation Plan", nil }
	s := kAt(9, 58).Format(time.RFC3339)
	in.Live = []LiveFile{
		lf("1", liveJSON("ask", s, "/ws/repos/x", a), kNow),
		lf("2", liveJSON("ask", s, "/ws/repos/zenify-kit/.worktrees/track-kitchen", ""), kNow),
		lf("3", liveJSON("ask", s, "/ws/repos/zenify-kit", ""), kNow),
		lf("4", liveJSON("ask", s, "/tmp/foo", ""), kNow),
	}
	r, _ := Compute(in)
	got := map[string]string{}
	for _, b := range r.Kitchen.RollingBoil {
		got[b.Session] = b.Brief
	}
	want := map[string]string{"1": "Plan T", "2": "zenify-kit/track-kitchen", "3": "zenify-kit", "4": "foo"}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("brief[%s] = %q, want %q (%v)", k, got[k], w, got)
		}
	}
}

func TestKitchenMalformedLive(t *testing.T) { // SC-10
	in := kInput("")
	s := kAt(9, 58).Format(time.RFC3339)
	in.Live = []LiveFile{
		lf("a", `{"state":`, kNow),
		lf("b", liveJSON("foo", s, "/tmp", ""), kNow),
		lf("c", liveJSON("ask", "yesterday", "/tmp", ""), kNow),
	}
	r, _ := Compute(in)
	if r.Malformed != 3 || len(r.Kitchen.RollingBoil) != 0 {
		t.Fatalf("malformed = %d, rb = %+v", r.Malformed, r.Kitchen.RollingBoil)
	}
}

func TestKitchenDeadLiveNotMalformed(t *testing.T) {
	in := kInput("")
	in.Live = []LiveFile{lf("a", `{"state":`, kNow.Add(-31*time.Second))}
	r, _ := Compute(in)
	if r.Malformed != 0 {
		t.Fatalf("malformed = %d", r.Malformed)
	}
}

func TestKitchenEmptyJSON(t *testing.T) {
	r, _ := Compute(kInput(""))
	b, _ := json.Marshal(r.Kitchen)
	if string(b) != `{"cooked":[],"rollingBoil":[],"titleError":""}` {
		t.Fatalf("kitchen = %s", b)
	}
}

func prItems(n int) Input {
	var l []string
	for i := 1; i <= n; i++ {
		id := kid("cook", i)
		l = append(l, row(kAt(8, 0), "start", id, "cook", "r", "M"),
			row(kAt(9, i), "end", id, fmt.Sprintf("https://x/pr/%d", i), "1", "0"))
	}
	in := kInput(lines(l...))
	in.PRTitles = map[string]string{}
	return in
}

func TestKitchenNoTitleCallsWhenMergedLookupTimedOut(t *testing.T) {
	in := prItems(3)
	in.Merged = func(string) (string, error) { return "", ErrGhTimeout }
	calls := 0
	in.PRTitle = func(string) (string, error) { calls++; return "P", nil }
	Compute(in)
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestKitchenTitleTimeoutStopsFetchesAndIsNotCached(t *testing.T) {
	in := prItems(3)
	calls := 0
	in.PRTitle = func(string) (string, error) { calls++; return "", ErrGhTimeout }
	Compute(in)
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if len(in.PRTitles) != 0 {
		t.Fatalf("timeout was cached: %v", in.PRTitles)
	}
}

func TestKitchenFailedTitleIsCachedNegative(t *testing.T) {
	in := prItems(4)
	calls := 0
	in.PRTitle = func(u string) (string, error) {
		calls++
		if u == "https://x/pr/4" {
			return "T4", nil
		}
		return "", errors.New("no access")
	}
	Compute(in)
	if calls != 3 {
		t.Fatalf("first run calls = %d, want 3", calls)
	}
	if want := titleFailedPrefix + kNow.UTC().Format(time.RFC3339); in.PRTitles["https://x/pr/1"] != want {
		t.Fatalf("cached = %q, want %q", in.PRTitles["https://x/pr/1"], want)
	}
	r, _ := Compute(in)
	if calls != 4 {
		t.Fatalf("second run calls = %d, want 4", calls)
	}
	for _, it := range r.Kitchen.Cooked {
		if it.ID == kid("cook", 4) && it.Brief != "T4" {
			t.Fatalf("brief = %q", it.Brief)
		}
		if it.ID == kid("cook", 1) && it.Brief != "cook r" {
			t.Fatalf("brief 1 = %q", it.Brief)
		}
	}
}

func TestKitchenFailedTitleRetriedAfterAnHour(t *testing.T) {
	for _, c := range []struct {
		age   time.Duration
		calls int
		brief string
	}{{30 * time.Minute, 0, "cook r"}, {61 * time.Minute, 1, "New"}} {
		in := prItems(1)
		in.PRTitles["https://x/pr/1"] = titleFailedPrefix + kNow.Add(-c.age).UTC().Format(time.RFC3339)
		calls := 0
		in.PRTitle = func(string) (string, error) { calls++; return "New", nil }
		r, _ := Compute(in)
		if calls != c.calls || r.Kitchen.Cooked[0].Brief != c.brief {
			t.Fatalf("age %v: calls = %d brief = %q, want %d %q", c.age, calls, r.Kitchen.Cooked[0].Brief, c.calls, c.brief)
		}
	}
	// A failed entry that cannot be parsed is retried too, and never shown.
	in := prItems(1)
	in.PRTitles["https://x/pr/1"] = titleFailedPrefix + "garbage"
	in.PRTitle = nil
	if r, _ := Compute(in); r.Kitchen.Cooked[0].Brief != "cook r" {
		t.Fatalf("brief = %q", r.Kitchen.Cooked[0].Brief)
	}
}
