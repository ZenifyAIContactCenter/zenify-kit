package hub

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeGH returns a canned body or error and records the args it got.
type fakeGH struct {
	body []byte
	err  error
	args [][]string
}

func (f *fakeGH) Run(args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	return f.body, f.err
}

func TestParsePRs(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "prs.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := parsePRs(body)
	if err != nil {
		t.Fatalf("parsePRs: %v", err)
	}
	want := []PR{
		{
			Number: 812, URL: "https://github.com/ZenifyAIContactCenter/contact-center-be/pull/812",
			Repo: "ZenifyAIContactCenter/contact-center-be", Branch: "phuthuycoding/fix/redis-scan-storm",
			MergeState: "CLEAN", ReviewDecision: "APPROVED", Checks: "SUCCESS",
		},
		{
			Number: 431, URL: "https://github.com/ZenifyAIContactCenter/contact-center-hub/pull/431",
			Repo: "ZenifyAIContactCenter/contact-center-hub", Branch: "phuthuycoding/feat/sla-export",
			MergeState: "BLOCKED", ReviewDecision: "REVIEW_REQUIRED", Checks: "PENDING",
			Draft: true, UnresolvedThreads: 2,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePRs:\n got %+v\nwant %+v", got, want)
	}
}

func TestParsePRs_FailedURL(t *testing.T) {
	body := []byte(`{"data":{"search":{"nodes":[{"number":7,"url":"u","headRefName":"b",
		"repository":{"nameWithOwner":"o/r"},
		"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
			{"conclusion":"SUCCESS","detailsUrl":"https://github.com/o/r/actions/runs/1/job/1"},
			{},
			{"conclusion":"FAILURE","detailsUrl":"https://github.com/o/r/actions/runs/1/job/2"},
			{"conclusion":"FAILURE","detailsUrl":"https://github.com/o/r/actions/runs/1/job/3"}
		]}}}}]},
		"reviewThreads":{"nodes":[]}}]}}}`)
	got, err := parsePRs(body)
	if err != nil {
		t.Fatalf("parsePRs: %v", err)
	}
	if len(got) != 1 || got[0].FailedURL != "https://github.com/o/r/actions/runs/1/job/2" || got[0].Checks != "FAILURE" {
		t.Errorf("got %+v", got)
	}
}

func TestParsePRs_GraphQLErrors(t *testing.T) {
	_, err := parsePRs([]byte(`{"errors":[{"message":"rate limited"}]}`))
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("want rate-limited error, got %v", err)
	}
}

func TestFetchPRs(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "prs.json"))
	if err != nil {
		t.Fatal(err)
	}
	gh := &fakeGH{body: body}
	prs, err := FetchPRs(gh)
	if err != nil || len(prs) != 2 {
		t.Fatalf("FetchPRs: %v %+v", err, prs)
	}
	if len(gh.args) != 1 || !reflect.DeepEqual(gh.args[0], []string{"api", "graphql", "-f", "query=" + prQuery}) {
		t.Errorf("gh args: %q", gh.args)
	}

	gh = &fakeGH{err: errors.New("gh: not logged in")}
	if _, err := FetchPRs(gh); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("want gh error, got %v", err)
	}
}

func TestDetectAlerts(t *testing.T) {
	const now = 1000
	prev := []PR{
		{Repo: "o/be", Number: 812, Checks: "PENDING", MergeState: "BLOCKED"},
		{Repo: "o/be", Number: 813, Checks: "SUCCESS", MergeState: "BLOCKED"},
		{Repo: "o/be", Number: 814, Checks: "SUCCESS", MergeState: "BLOCKED", UnresolvedThreads: 1},
		{Repo: "o/hub", Number: 431, Checks: "SUCCESS", MergeState: "BLOCKED"},
		{Repo: "o/hub", Number: 432, Checks: "FAILURE", MergeState: "BLOCKED", Alert: "ci-failed", AlertAt: 500},
	}
	cur := []PR{
		{Repo: "o/be", Number: 812, Checks: "FAILURE", MergeState: "BLOCKED"},
		{Repo: "o/be", Number: 813, Checks: "SUCCESS", MergeState: "CLEAN"},
		{Repo: "o/be", Number: 814, Checks: "SUCCESS", MergeState: "CLEAN", UnresolvedThreads: 1},
		{Repo: "o/hub", Number: 431, Checks: "SUCCESS", MergeState: "BLOCKED", UnresolvedThreads: 2},
		{Repo: "o/hub", Number: 432, Checks: "FAILURE", MergeState: "BLOCKED"},
	}
	out, fired := DetectAlerts(prev, cur, now)
	wantAlert := map[int]string{812: "ci-failed", 813: "ready", 814: "", 431: "new-review", 432: "ci-failed"}
	wantAt := map[int]int64{812: now, 813: now, 814: 0, 431: now, 432: 500}
	for _, p := range out {
		if p.Alert != wantAlert[p.Number] || p.AlertAt != wantAt[p.Number] {
			t.Errorf("#%d: alert %q at %d, want %q at %d", p.Number, p.Alert, p.AlertAt, wantAlert[p.Number], wantAt[p.Number])
		}
	}
	var firedNums []int
	for _, p := range fired {
		firedNums = append(firedNums, p.Number)
	}
	if !reflect.DeepEqual(firedNums, []int{812, 813, 431}) {
		t.Errorf("fired %v, want [812 813 431]", firedNums)
	}
}

func TestDropAcked(t *testing.T) {
	dir := t.TempDir()
	touch := func(name string, at time.Time) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	touch("o_be_812", time.Unix(2000, 0)) // newer than AlertAt → drop
	touch("o_be_813", time.Unix(500, 0))  // older than AlertAt → keep
	touch("o_be_815", time.Unix(1000, 0)) // same second as AlertAt → keep
	in := []PR{
		{Repo: "o/be", Number: 812, Alert: "ci-failed", AlertAt: 1000},
		{Repo: "o/be", Number: 813, Alert: "ready", AlertAt: 1000},
		{Repo: "o/be", Number: 815, Alert: "ready", AlertAt: 1000},
		{Repo: "o/be", Number: 814, Alert: "new-review", AlertAt: 1000}, // no ack file
	}
	got, err := DropAcked(in, dir)
	if err != nil {
		t.Fatalf("DropAcked: %v", err)
	}
	want := map[int]string{812: "", 813: "ready", 814: "new-review", 815: "ready"}
	for _, p := range got {
		if p.Alert != want[p.Number] {
			t.Errorf("#%d: alert %q, want %q", p.Number, p.Alert, want[p.Number])
		}
	}
}

func TestGitHubInterval(t *testing.T) {
	pending := State{Tasks: []Task{{Repos: []TaskRepo{{PR: &PR{Checks: "PENDING"}}}}}}
	if got := GitHubInterval(pending); got != 20*time.Second {
		t.Errorf("task PR pending: got %v want 20s", got)
	}
	if got := GitHubInterval(State{OtherPRs: []PR{{Checks: "SUCCESS"}, {Checks: "PENDING"}}}); got != 20*time.Second {
		t.Errorf("other PR pending: got %v want 20s", got)
	}
	if got := GitHubInterval(State{OtherPRs: []PR{{Checks: "FAILURE"}}}); got != 60*time.Second {
		t.Errorf("no pending: got %v want 60s", got)
	}
}

func TestParsePRs_FailedURL_OtherConclusions(t *testing.T) {
	mk := func(ctx string) []byte {
		return []byte(`{"data":{"search":{"nodes":[{"number":7,"url":"u","headRefName":"b",
		"repository":{"nameWithOwner":"o/r"},
		"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[` + ctx + `]}}}}]},
		"reviewThreads":{"nodes":[]}}]}}}`)
	}
	cases := []struct{ name, ctx, want string }{
		{"timed out check run", `{"conclusion":"SUCCESS","detailsUrl":"ok"},{"conclusion":"TIMED_OUT","detailsUrl":"https://ci/timed-out"}`, "https://ci/timed-out"},
		{"status context error", `{"state":"SUCCESS","targetUrl":"ok"},{"state":"ERROR","targetUrl":"https://ci/status-error"}`, "https://ci/status-error"},
		{"status context failure", `{"state":"FAILURE","targetUrl":"https://ci/status-failure"}`, "https://ci/status-failure"},
	}
	for _, c := range cases {
		got, err := parsePRs(mk(c.ctx))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(got) != 1 || got[0].FailedURL != c.want {
			t.Errorf("%s: got %+v, want FailedURL %q", c.name, got, c.want)
		}
	}
	if !strings.Contains(prQuery, "... on StatusContext { state targetUrl }") {
		t.Error("prQuery must request StatusContext state and targetUrl")
	}
}

// A PR with more review threads than one page returns has an unknown
// unresolved count, so it must never be reported ready.
func TestParsePRs_TruncatedThreadsNeverReady(t *testing.T) {
	nodes := strings.TrimSuffix(strings.Repeat(`{"isResolved":true},`, 100), ",")
	body := []byte(`{"data":{"search":{"nodes":[{"number":9,"url":"u","headRefName":"b","mergeStateStatus":"CLEAN",
		"repository":{"nameWithOwner":"o/r"},"commits":{"nodes":[]},
		"reviewThreads":{"totalCount":150,"nodes":[` + nodes + `]}}]}}}`)
	cur, err := parsePRs(body)
	if err != nil {
		t.Fatalf("parsePRs: %v", err)
	}
	if len(cur) != 1 || !cur[0].ThreadsTruncated || cur[0].UnresolvedThreads != 0 {
		t.Fatalf("got %+v, want ThreadsTruncated with 0 counted unresolved", cur)
	}
	prev := []PR{{Repo: "o/r", Number: 9, MergeState: "BLOCKED"}}
	out, fired := DetectAlerts(prev, cur, 1000)
	if len(fired) != 0 || out[0].Alert == AlertReady {
		t.Errorf("truncated PR fired %+v, alert %q; want no ready", fired, out[0].Alert)
	}
}
