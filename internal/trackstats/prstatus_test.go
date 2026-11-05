package trackstats

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const prFixtureClean = `{"data":{"repository":{"pullRequest":{"createdAt":"2026-11-04T01:02:03Z","reviewDecision":null,"mergeStateStatus":"CLEAN","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]},"reviewThreads":{"nodes":[]}}}}}`

const prFixtureBad = `{"data":{"repository":{"pullRequest":{"createdAt":"2026-11-04T01:02:03Z","reviewDecision":"CHANGES_REQUESTED","mergeStateStatus":"BLOCKED","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]},"reviewThreads":{"nodes":[{"isResolved":false},{"isResolved":false},{"isResolved":true}]}}}}}`

const prFixtureNoRollup = `{"data":{"repository":{"pullRequest":{"createdAt":"2026-11-04T01:02:03Z","reviewDecision":"APPROVED","mergeStateStatus":"DRAFT","commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]},"reviewThreads":{"nodes":[]}}}}}`

func TestParsePRStatus(t *testing.T) {
	url := "https://github.com/o/r/pull/12"
	cases := []struct {
		name string
		in   string
		want PRStatus
	}{
		{"clean", prFixtureClean, PRStatus{URL: url, Repo: "r", Number: 12, CreatedAt: "2026-11-04T01:02:03Z", CI: "success", Review: "-", Threads: 0, Ready: true}},
		{"failing", prFixtureBad, PRStatus{URL: url, Repo: "r", Number: 12, CreatedAt: "2026-11-04T01:02:03Z", CI: "failure", Review: "changes_requested", Threads: 2, Ready: false}},
		{"no rollup, draft", prFixtureNoRollup, PRStatus{URL: url, Repo: "r", Number: 12, CreatedAt: "2026-11-04T01:02:03Z", CI: "-", Review: "approved", Threads: 0, Ready: false}},
	}
	for _, c := range cases {
		got, err := parsePRStatus(url, []byte(c.in))
		if err != nil || got != c.want {
			t.Errorf("%s: got %+v err %v, want %+v", c.name, got, err, c.want)
		}
	}
	if _, err := parsePRStatus(url, []byte(`{"data":{"repository":{"pullRequest":null}}}`)); err == nil {
		t.Error("null pullRequest must be an error")
	}
	if _, err := parsePRStatus(url, []byte(`not json`)); err == nil {
		t.Error("bad json must be an error")
	}
}

func TestPRStatusURL(t *testing.T) {
	o, n, num, err := prStatusURL("https://github.com/o/r/pull/12")
	if err != nil || o != "o" || n != "r" || num != 12 {
		t.Fatalf("got %q %q %d %v", o, n, num, err)
	}
	if _, _, _, err := prStatusURL("https://github.com/o/r/issues/12"); err == nil {
		t.Fatal("a URL without /pull/<n> must be an error")
	}
}

func prOpenLog(n int) (string, []string) {
	var rows []string
	var urls []string
	for i := 1; i <= n; i++ {
		u := fmt.Sprintf("https://github.com/o/r/pull/%d", i)
		urls = append(urls, u)
		rows = append(rows,
			row(now.Add(-time.Duration(n+1-i)*time.Hour-30*time.Minute), "start", cid("fix", i), "fix", "r", "S"),
			row(now.Add(-time.Duration(n+1-i)*time.Hour), "end", cid("fix", i), u, "1", "0"))
	}
	return lines(rows...), urls
}

func fakePRStatus(draft string, failOn map[string]error, asked *[]string) PRStatusFunc {
	return func(u string) (PRStatus, error) {
		*asked = append(*asked, u)
		if err := failOn[u]; err != nil {
			return PRStatus{}, err
		}
		fix := prFixtureClean
		if u == draft {
			fix = prFixtureNoRollup
		}
		return parsePRStatus(u, []byte(fix))
	}
}

func TestComputePROpen(t *testing.T) {
	log, urls := prOpenLog(12)
	planned := "https://github.com/o/r/pull/99"
	log += row(now.Add(-10*time.Minute), "plan", today, planned) + "\n"
	in := base(log)
	// pr 12 (newest) is merged, pr 11 closed unmerged: neither is open.
	in.Merged = func(string) (string, error) { return "", nil }
	in.Cache = map[string]string{urls[11]: "2026-10-30T08:00:00Z", urls[10]: ClosedUnmerged}
	var asked []string
	in.PRStatus = fakePRStatus(urls[9], nil, &asked)
	r, _ := Compute(in)
	if len(r.Today.PROpen) != 10 {
		t.Fatalf("len = %d, want 10: %+v", len(r.Today.PROpen), r.Today.PROpen)
	}
	if got := r.Today.PROpen[0].URL; got != urls[9] {
		t.Fatalf("first = %s, want newest ended open %s", got, urls[9])
	}
	if got := r.Today.PROpen[9].URL; got != urls[0] {
		t.Fatalf("last = %s", got)
	}
	for _, p := range r.Today.PROpen {
		if p.URL == urls[11] || p.URL == urls[10] {
			t.Fatalf("merged/closed PR listed: %s", p.URL)
		}
	}
	if r.Today.PROpen[0].Ready {
		t.Fatal("DRAFT PR must not be Ready")
	}
	if !r.Today.PROpen[1].Ready {
		t.Fatal("CLEAN PR with 0 threads must be Ready")
	}
}

func TestComputePROpenPlanOnlyAndEmpty(t *testing.T) {
	planned := "https://github.com/o/r/pull/99"
	in := base(row(now.Add(-10*time.Minute), "plan", today, planned) + "\n")
	in.Merged = func(string) (string, error) { return "", nil }
	var asked []string
	in.PRStatus = fakePRStatus("", nil, &asked)
	r, _ := Compute(in)
	if len(r.Today.PROpen) != 1 || r.Today.PROpen[0].URL != planned {
		t.Fatalf("plan-only PR missing: %+v", r.Today.PROpen)
	}
	r, _ = Compute(base(""))
	if r.Today.PROpen == nil {
		t.Fatal("PROpen must be an empty slice, not nil")
	}
}

func TestPRStatusErrorKeepsOthers(t *testing.T) { // SC-8
	log, urls := prOpenLog(3)
	in := base(log)
	in.Merged = func(string) (string, error) { return "", nil }
	var asked []string
	in.PRStatus = fakePRStatus("", map[string]error{urls[1]: errors.New("boom")}, &asked)
	r, _ := Compute(in)
	if !strings.Contains(r.Today.PRStatusError, urls[1]) || !strings.Contains(r.Today.PRStatusError, "boom") {
		t.Fatalf("prStatusError = %q", r.Today.PRStatusError)
	}
	if len(r.Today.PROpen) != 2 {
		t.Fatalf("other PRs lost: %+v", r.Today.PROpen)
	}
}

func TestPRStatusTimeoutStopsTheLoop(t *testing.T) { // SC-8
	log, _ := prOpenLog(3)
	in := base(log)
	in.Merged = func(string) (string, error) { return "", nil }
	asked := 0
	in.PRStatus = func(string) (PRStatus, error) { asked++; return PRStatus{}, fmt.Errorf("gh api: %w", ErrGhTimeout) }
	r, _ := Compute(in)
	if asked != 1 || r.Today.PRStatusError == "" {
		t.Fatalf("asked %d err %q, want one ask and an error", asked, r.Today.PRStatusError)
	}
}

func TestPRStatusFromCacheWithoutFunc(t *testing.T) {
	log, urls := prOpenLog(2)
	in := base(log)
	in.PRStatusCache = map[string]PRStatus{urls[1]: {URL: urls[1], CI: "success", Review: "-"}}
	r, _ := Compute(in)
	if len(r.Today.PROpen) != 1 || r.Today.PROpen[0].URL != urls[1] {
		t.Fatalf("cache not used: %+v", r.Today.PROpen)
	}
}
