package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

var bidOutRe = regexp.MustCompile(`^track: (b-\d{6}-[0-9a-f]{4})\n$`)

type statsBacklog struct {
	Backlog struct {
		Items []struct {
			Source, ID, Repo, Est, Ref, Title string
		}
		GhError string
	}
}

func trackBacklogStats(t *testing.T, ws string, extra ...string) statsBacklog {
	t.Helper()
	out, err := runTrack(t, ws, append([]string{"stats", "--json"}, extra...)...)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	var s statsBacklog
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("stats json: %v\n%s", err, out)
	}
	return s
}

// noGh replaces the gh search seam with one that finds nothing.
func fakeGh(t *testing.T, fn func(owner, role string) ([]trackstats.Issue, error)) {
	t.Helper()
	old := trackGhSearch
	trackGhSearch = fn
	t.Cleanup(func() { trackGhSearch = old })
}

func noGh(t *testing.T) {
	t.Helper()
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) { return nil, nil })
}

func addBacklog(t *testing.T, ws string, args ...string) string {
	t.Helper()
	out, err := runTrack(t, ws, append([]string{"backlog", "add"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	m := bidOutRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("add output %q", out)
	}
	return m[1]
}

// SC-30
func TestTrackBacklog_AddFromDropStats(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	bid := addBacklog(t, ws, "x", "--repo", "kit", "--est", "M")
	if !regexp.MustCompile(`· backlog · ` + bid + ` · kit · M · - · x\n$`).MatchString(trackLog(t, ws)) {
		t.Fatalf("log = %q", trackLog(t, ws))
	}
	s := trackBacklogStats(t, ws)
	if len(s.Backlog.Items) != 1 || s.Backlog.Items[0].Source != "manual" || s.Backlog.Items[0].ID != bid || s.Backlog.Items[0].Est != "M" {
		t.Fatalf("items = %+v", s.Backlog.Items)
	}
	if _, err := runTrack(t, ws, "start", "--kind", "cook", "--repo", "kit", "--est", "M", "--from", bid); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(trackLog(t, ws)), " · kit · M · "+bid) {
		t.Fatalf("start row lacks --from: %q", trackLog(t, ws))
	}
	if s := trackBacklogStats(t, ws); len(s.Backlog.Items) != 0 {
		t.Fatalf("item survived start --from: %+v", s.Backlog.Items)
	}
}

func TestTrackBacklog_DropRules(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	bid := addBacklog(t, ws, "x", "--repo", "kit", "--ref", "memory:foo")
	before := trackLog(t, ws)
	for _, id := range []string{"b-261030-ffff", "nonsense", "repos/kit/.znf/sdd/p/notprogress.md:3"} {
		if _, err := runTrack(t, ws, "backlog", "drop", id); err == nil {
			t.Errorf("drop %q must fail", id)
		}
	}
	if trackLog(t, ws) != before {
		t.Fatalf("log changed by failed drops: %q", trackLog(t, ws))
	}
	out, err := runTrack(t, ws, "backlog", "drop", bid)
	if err != nil || out != "track: dropped "+bid+"\n" {
		t.Fatalf("drop: %q %v", out, err)
	}
	after := trackLog(t, ws)
	if _, err := runTrack(t, ws, "backlog", "drop", bid); err == nil {
		t.Fatal("second drop must fail")
	}
	if trackLog(t, ws) != after {
		t.Fatal("log changed by a second drop")
	}
	// An sdd id of the right shape is accepted once.
	sdd := "repos/kit/.znf/sdd/p/progress.md:7"
	if _, err := runTrack(t, ws, "backlog", "drop", sdd); err != nil {
		t.Fatal(err)
	}
	if _, err := runTrack(t, ws, "backlog", "drop", sdd); err == nil {
		t.Fatal("second sdd drop must fail")
	}
}

func TestTrackBacklog_AddRejectsBadInput(t *testing.T) {
	ws := trackWS(t, true)
	for _, args := range [][]string{
		{"", "--repo", "kit"},
		{"x\ny", "--repo", "kit"},
		{"x", "--repo", ""},
		{"x"},
		{"x", "--repo", "kit", "--est", "X"},
		{"x", "--repo", "kit", "--ref", "ftp://x"},
		{"x", "--repo", "kit", "--ref", "memory:"},
	} {
		if _, err := runTrack(t, ws, append([]string{"backlog", "add"}, args...)...); err == nil {
			t.Errorf("%q must fail", args)
		}
	}
	if got := trackLog(t, ws); got != "" {
		t.Fatalf("log = %q", got)
	}
}

func TestTrackBacklog_OffPrintsOff(t *testing.T) {
	ws := trackWS(t, false)
	for _, args := range [][]string{
		{"backlog", "add", "x", "--repo", "kit"},
		{"backlog", "drop", "b-261030-ffff"},
		{"onboard", "memory"},
		{"onboard", "history", "--write"},
	} {
		out, err := runTrack(t, ws, args...)
		if err != nil || out != "track: off\n" {
			t.Errorf("%v: %q %v", args, out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(ws, ".zenify", "track")); !os.IsNotExist(err) {
		t.Fatalf("track dir must not exist: %v", err)
	}
}

func TestTrackStart_FromRules(t *testing.T) {
	ws := trackWS(t, true)
	bid := addBacklog(t, ws, "x", "--repo", "kit")
	if _, err := runTrack(t, ws, "backlog", "drop", bid); err != nil {
		t.Fatal(err)
	}
	before := trackLog(t, ws)
	for _, from := range []string{bid, "b-261030-ffff", "ftp://x", "https://a b"} {
		if _, err := runTrack(t, ws, "start", "--kind", "cook", "--repo", "kit", "--from", from); err == nil {
			t.Errorf("--from %q must fail", from)
		}
	}
	if trackLog(t, ws) != before {
		t.Fatalf("log changed: %q", trackLog(t, ws))
	}
	if _, err := runTrack(t, ws, "start", "--kind", "cook", "--repo", "kit", "--from", "https://github.com/o/r/issues/1"); err != nil {
		t.Fatal(err)
	}
}

func TestTrackOnboardMemory(t *testing.T) {
	ws := trackWS(t, true)
	if _, err := runTrack(t, ws, "onboard", "memory"); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ · onboard · memory\n$`).MatchString(trackLog(t, ws)) {
		t.Fatalf("log = %q", trackLog(t, ws))
	}
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil || !strings.Contains(out, `"onboarded": true`) {
		t.Fatalf("stats: %q %v", out, err)
	}
}

func gitRepoWithOrigin(t *testing.T, ws, name, url string) {
	t.Helper()
	dir := filepath.Join(ws, "repos", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"remote", "add", "origin", url}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
	}
}

// SC-37
func TestTrackStats_GhOwnersAndCache(t *testing.T) {
	ws := trackWS(t, true)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	gitRepoWithOrigin(t, ws, "b", "https://github.com/Z/b")
	gitRepoWithOrigin(t, ws, "c", "ssh://git@github.com/Y/c.git")
	var mu sync.Mutex
	var calls []string
	fakeGh(t, func(owner, role string) ([]trackstats.Issue, error) {
		mu.Lock()
		calls = append(calls, owner+"/"+role)
		mu.Unlock()
		return []trackstats.Issue{{URL: "https://github.com/" + owner + "/r/issues/1", Repo: "r", Title: "issue " + owner, CreatedAt: time.Now().Add(-48 * time.Hour)}}, nil
	})
	s := trackBacklogStats(t, ws)
	want := map[string]bool{"Y/assignee": true, "Y/author": true, "Z/assignee": true, "Z/author": true}
	if len(calls) != 4 {
		t.Fatalf("calls = %v", calls)
	}
	for _, c := range calls {
		if !want[c] {
			t.Fatalf("unexpected call %q in %v", c, calls)
		}
		delete(want, c)
	}
	// Both roles return the same issue per owner: merged by URL, so 2 items.
	if len(s.Backlog.Items) != 2 || s.Backlog.GhError != "" {
		t.Fatalf("backlog = %+v", s.Backlog)
	}
	calls = nil
	s = trackBacklogStats(t, ws)
	if len(calls) != 0 || len(s.Backlog.Items) != 2 {
		t.Fatalf("second run within 5 min: calls %v items %d", calls, len(s.Backlog.Items))
	}
	trackBacklogStats(t, ws, "--force")
	if len(calls) != 4 {
		t.Fatalf("--force must bypass the cache: calls %v", calls)
	}
	entries, _ := filepath.Glob(filepath.Join(ws, ".zenify", "track", "backlog-gh*"))
	if len(entries) != 1 || !strings.HasSuffix(entries[0], "backlog-gh.json") {
		t.Fatalf("cache files = %v (temp file left behind?)", entries)
	}
}

func TestTrackStats_GhCacheExpires(t *testing.T) {
	ws := trackWS(t, true)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	var n atomic.Int32
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) { n.Add(1); return nil, nil })
	trackBacklogStats(t, ws)
	if n.Load() != 2 {
		t.Fatalf("calls = %d", n.Load())
	}
	p := filepath.Join(ws, ".zenify", "track", "backlog-gh.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["fetchedAt"] = time.Now().Add(-6 * time.Minute).UTC().Format(time.RFC3339)
	b, _ = json.Marshal(m)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	trackBacklogStats(t, ws)
	if n.Load() != 4 {
		t.Fatalf("expired cache must refetch: calls = %d", n.Load())
	}
}

func TestTrackStats_GhErrorNamesOwnerAndRole(t *testing.T) {
	ws := trackWS(t, true)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	fakeGh(t, func(owner, role string) ([]trackstats.Issue, error) {
		if role == "author" {
			return nil, fmt.Errorf("boom")
		}
		return []trackstats.Issue{{URL: "https://github.com/Z/r/issues/9", Repo: "r", Title: "kept", CreatedAt: time.Now()}}, nil
	})
	if _, err := runTrack(t, ws, "backlog", "add", "m", "--repo", "kit"); err != nil {
		t.Fatal(err)
	}
	s := trackBacklogStats(t, ws)
	if !strings.Contains(s.Backlog.GhError, "Z") || !strings.Contains(s.Backlog.GhError, "author") || !strings.Contains(s.Backlog.GhError, "boom") || strings.Contains(s.Backlog.GhError, "assignee") {
		t.Fatalf("ghError = %q", s.Backlog.GhError)
	}
	if len(s.Backlog.Items) != 2 {
		t.Fatalf("items of the successful search and other sources must stay: %+v", s.Backlog.Items)
	}
	// A failed fetch is not cached, so the next run asks again.
	var n atomic.Int32
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) { n.Add(1); return nil, nil })
	trackBacklogStats(t, ws)
	if n.Load() != 2 {
		t.Fatalf("calls after a failed run = %d", n.Load())
	}
}

func TestTrackStats_CorruptCacheSurfacesButExitsZero(t *testing.T) {
	ws := trackWS(t, true)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	noGh(t)
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backlog-gh.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := trackBacklogStats(t, ws)
	if !strings.Contains(s.Backlog.GhError, "backlog-gh.json") {
		t.Fatalf("ghError = %q", s.Backlog.GhError)
	}
}

func TestTrackStats_UnwritableCacheSurfaces(t *testing.T) {
	ws := trackWS(t, true)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	noGh(t)
	// A directory named backlog-gh.json cannot be replaced by rename and cannot be read as a file.
	if err := os.MkdirAll(filepath.Join(ws, ".zenify", "track", "backlog-gh.json", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := trackBacklogStats(t, ws)
	if !strings.Contains(s.Backlog.GhError, "backlog-gh.json") {
		t.Fatalf("ghError = %q", s.Backlog.GhError)
	}
}

func TestTrackStats_SDDScan(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	write := func(rel, body string) {
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("repos/kit/.worktrees/x/.znf/sdd/p/progress.md", "Task 1: complete\nTask 2: minor (deferred): rename the helper\nTask 3: parked — dead branch in parse — Ruling: out of scope\n")
	write("repos/hub/.znf/sdd/q/progress.md", "x\nTask 4: minor (deferred): tidy logs\n")
	write("repos/hub/.znf/sdd/q/other.md", "Task 5: minor (deferred): not a ledger\n")
	s := trackBacklogStats(t, ws)
	got := map[string]string{}
	for _, it := range s.Backlog.Items {
		if it.Source != "sdd" {
			t.Fatalf("unexpected %+v", it)
		}
		got[it.ID] = it.Repo + "|" + it.Title
	}
	want := map[string]string{
		"repos/kit/.worktrees/x/.znf/sdd/p/progress.md:2": "kit|rename the helper",
		"repos/kit/.worktrees/x/.znf/sdd/p/progress.md:3": "kit|dead branch in parse — Ruling: out of scope",
		"repos/hub/.znf/sdd/q/progress.md:2":              "hub|tidy logs",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, err := runTrack(t, ws, "backlog", "drop", "repos/hub/.znf/sdd/q/progress.md:2"); err != nil {
		t.Fatal(err)
	}
	// SDD items live in the cache (one TTL), so force a fresh scan view of the drop.
	if s := trackBacklogStats(t, ws); len(s.Backlog.Items) != 2 {
		t.Fatalf("drop must hide the sdd item: %+v", s.Backlog.Items)
	}
}

func TestTrackStats_OffSkipsGhAndScan(t *testing.T) {
	ws := trackWS(t, false)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) {
		t.Error("gh called while track is off")
		return nil, nil
	})
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil || !strings.Contains(out, `"enabled": false`) {
		t.Fatalf("%q %v", out, err)
	}
}
