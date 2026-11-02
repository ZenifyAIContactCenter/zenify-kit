package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func trackWS(t *testing.T, on bool) string {
	t.Helper()
	ws := t.TempDir()
	if on {
		if err := os.WriteFile(filepath.Join(ws, ".zenify-overlay.yaml"), []byte("features:\n  track: true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func runTrack(t *testing.T, ws string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append(append([]string{"track"}, args...), "--workspace", ws))
	err := root.Execute()
	return out.String(), err
}

func trackLog(t *testing.T, ws string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ws, ".zenify", "track", "log.md"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

var trackOutRe = regexp.MustCompile(`^track: ((cook|fix|hotfix)-\d{6}-[0-9a-f]{4})\n$`)

func startTrack(t *testing.T, ws, kind string) string {
	t.Helper()
	out, err := runTrack(t, ws, "start", "--kind", kind, "--repo", "hub", "--est", "-")
	if err != nil {
		t.Fatal(err)
	}
	m := trackOutRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("start output %q", out)
	}
	return m[1]
}

// SC-2
func TestTrack_OffWritesNothing(t *testing.T) {
	ws := trackWS(t, false)
	for _, args := range [][]string{
		{"start", "--kind", "cook", "--repo", "hub", "--est", "-"},
		{"est", "cook-261030-abcd", "M"},
		{"ship", "cook-261030-abcd", "--outcome", "shipped", "--findings", "-", "--rounds", "0"},
		{"fix", "fix-261030-abcd", "--round", "1", "--path", "narrow"},
		{"ci", "cook-261030-abcd", "--result", "green"},
		{"end", "cook-261030-abcd", "--cancelled", "--rounds", "0"},
	} {
		out, err := runTrack(t, ws, args...)
		if err != nil || out != "track: off\n" {
			t.Errorf("%v: out %q err %v", args, out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(ws, ".zenify", "track")); !os.IsNotExist(err) {
		t.Fatalf(".zenify/track must not exist, stat err = %v", err)
	}
}

func TestTrack_StatsOffReportsDisabled(t *testing.T) {
	ws := trackWS(t, false)
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got["enabled"] != false {
		t.Fatalf("enabled = %v", got["enabled"])
	}
	if _, err := os.Stat(filepath.Join(ws, ".zenify")); !os.IsNotExist(err) {
		t.Fatalf("stats off must not touch .zenify, stat err = %v", err)
	}
}

func TestTrack_RowsAndStats(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "fix")
	steps := [][]string{
		{"est", id, "S"},
		{"fix", id, "--round", "1", "--path", "wide", "--regression", "no-seam"},
		{"ship", id, "--outcome", "blocked", "--blocked-step", "lint", "--findings", "-", "--rounds", "0"},
		{"ship", id, "--outcome", "shipped", "--findings", "0,1,2,3", "--rounds", "2"},
		{"ci", id, "--result", "red"},
		{"end", id, "--pr", "https://github.com/o/r/pull/1", "--pr", "https://github.com/o/r/pull/2", "--rounds", "2"},
	}
	for _, s := range steps {
		if _, err := runTrack(t, ws, s...); err != nil {
			t.Fatalf("%v: %v", s, err)
		}
	}
	want := []string{
		"start · " + id + " · fix · hub · -",
		"est · " + id + " · S",
		"fix · " + id + " · 1 · wide · no-seam",
		"ship · " + id + " · blocked · lint · - · 0",
		"ship · " + id + " · shipped · - · 0,1,2,3 · 2",
		"ci · " + id + " · red",
		"end · " + id + " · https://github.com/o/r/pull/1,https://github.com/o/r/pull/2 · 0 · 2",
	}
	lines := strings.Split(strings.TrimSuffix(trackLog(t, ws), "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("log has %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	iso := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ · `)
	for i, l := range lines {
		if !iso.MatchString(l) || !strings.HasSuffix(l, " · "+want[i]) {
			t.Errorf("line %d = %q, want suffix %q", i, l, want[i])
		}
	}
	var asked []string
	old := trackMerged
	trackMerged = func(u string) (string, error) { asked = append(asked, u); return "2026-10-30T01:00:00Z", nil }
	t.Cleanup(func() { trackMerged = old })
	out, err := runTrack(t, ws, "stats", "--json", "--today", "2000-01-01")
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Enabled bool `json:"enabled"`
		Today   struct {
			Date string `json:"date"`
		} `json:"today"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || !res.Enabled || res.Today.Date != "2000-01-01" {
		t.Fatalf("stats: %v %+v\n%s", err, res, out)
	}
	if _, err := runTrack(t, ws, "stats", "--json"); err != nil {
		t.Fatal(err)
	}
	if len(asked) == 0 {
		t.Fatal("stats for today never asked whether the PRs merged")
	}
	cache, err := os.ReadFile(filepath.Join(ws, ".zenify", "track", "merged.json"))
	if err != nil || !strings.Contains(string(cache), "2026-10-30T01:00:00Z") {
		t.Fatalf("asked %v but merged.json = %q err %v", asked, cache, err)
	}
	if out, err := runTrack(t, ws, "stats"); err != nil || !strings.Contains(out, "scale (") {
		t.Fatalf("human stats: %q %v", out, err)
	}
}

// The pane runs stats on almost every turn: gh is asked about an unmerged PR at most once
// per mergedRecheck, and a failed check is asked again on the next run.
func TestTrackStats_MergedCheckThrottled(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "fix")
	if _, err := runTrack(t, ws, "end", id, "--pr", "https://github.com/o/r/pull/9", "--rounds", "0"); err != nil {
		t.Fatal(err)
	}
	asked := 0
	fail := true
	old := trackMerged
	trackMerged = func(string) (string, error) {
		asked++
		if fail {
			return "", errors.New("gh down")
		}
		return "", nil
	}
	t.Cleanup(func() { trackMerged = old })
	stats := func() {
		t.Helper()
		if _, err := runTrack(t, ws, "stats", "--json"); err != nil {
			t.Fatal(err)
		}
	}
	stats()
	stats()
	if asked != 2 {
		t.Fatalf("a failed check must be asked again: asked %d, want 2", asked)
	}
	fail = false
	stats()
	stats()
	if asked != 3 {
		t.Fatalf("an unmerged PR must not be asked again within %s: asked %d, want 3", mergedRecheck, asked)
	}
	stamp := filepath.Join(ws, ".zenify", "track", "merged.checked")
	old6 := time.Now().Add(-mergedRecheck - time.Minute)
	if err := os.Chtimes(stamp, old6, old6); err != nil {
		t.Fatal(err)
	}
	stats()
	if asked != 4 {
		t.Fatalf("a stale check must ask again: asked %d, want 4", asked)
	}
}

// SC-5
func TestTrack_RejectsBadInputWithoutWriting(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "cook")
	if _, err := runTrack(t, ws, "end", id, "--cancelled", "--rounds", "0"); err != nil {
		t.Fatal(err)
	}
	before := trackLog(t, ws)
	cases := [][]string{
		{"start", "--kind", "foo", "--repo", "hub", "--est", "-"},
		{"start", "--kind", "cook", "--repo", "hub", "--est", "XL"},
		{"start", "--kind", "cook", "--repo", "", "--est", "-"},
		{"est", id, "XL"},
		{"est", "cook-261030-ffff", "M"},
		{"end", "cook-261030-ffff", "--cancelled", "--rounds", "0"},
		{"end", id, "--cancelled", "--rounds", "0"},
		{"end", id, "--rounds", "0"},
		{"end", id, "--cancelled", "--rollback", "--rounds", "0"},
		{"end", id, "--pr", "ftp://x", "--rounds", "0"},
		{"end", id, "--ledger", filepath.Join(ws, "missing.md"), "--cancelled", "--rounds", "0"},
		{"ship", id, "--outcome", "maybe", "--findings", "-", "--rounds", "0"},
		{"ship", id, "--outcome", "blocked", "--findings", "-", "--rounds", "0"},
		{"ship", id, "--outcome", "blocked", "--blocked-step", "foo", "--findings", "-", "--rounds", "0"},
		{"ship", id, "--outcome", "shipped", "--findings", "1,2,3", "--rounds", "0"},
		{"ship", id, "--outcome", "shipped", "--findings", "1,2,-3,4", "--rounds", "0"},
		{"ship", "cook-261030-ffff", "--outcome", "shipped", "--findings", "-", "--rounds", "0"},
		{"fix", id, "--round", "1", "--path", "sideways"},
		{"fix", id, "--round", "1", "--path", "wide", "--regression", "maybe"},
		{"fix", "fix-261030-ffff", "--round", "1", "--path", "wide"},
		{"ci", id, "--result", "blue"},
		{"ci", "cook-261030-ffff", "--result", "green"},
		{"ci", "not-an-id", "--result", "green"},
	}
	for _, c := range cases {
		if _, err := runTrack(t, ws, c...); err == nil {
			t.Errorf("%v: want error", c)
		}
		if got := trackLog(t, ws); got != before {
			t.Fatalf("%v changed log.md:\n%s", c, got)
		}
	}
}

// SC-10: O_APPEND across real processes; goroutines would not prove it.
func TestTrackHelperProcess(t *testing.T) {
	if os.Getenv("TRACK_HELPER_WS") == "" {
		t.Skip("helper process only")
	}
	root := NewRootCmd()
	root.SetArgs([]string{"track", "start", "--kind", "cook", "--repo", "hub", "--est", "-", "--workspace", os.Getenv("TRACK_HELPER_WS")})
	if err := root.Execute(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(0)
}

func TestTrack_TwentyProcessesAppendWithoutLoss(t *testing.T) {
	ws := trackWS(t, true)
	const n = 20
	outs := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestTrackHelperProcess$")
			cmd.Env = append(os.Environ(), "TRACK_HELPER_WS="+ws)
			b, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("child %d: %v\n%s", i, err, b)
			}
			outs[i] = string(b)
		}()
	}
	wg.Wait()
	ids := map[string]bool{}
	for _, o := range outs {
		for _, l := range strings.Split(o, "\n") {
			if m := trackOutRe.FindStringSubmatch(l + "\n"); m != nil {
				ids[m[1]] = true
			}
		}
	}
	if len(ids) != n {
		t.Fatalf("want %d distinct ids, got %d", n, len(ids))
	}
	lines := strings.Split(strings.TrimSuffix(trackLog(t, ws), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("log has %d lines, want %d:\n%s", len(lines), n, strings.Join(lines, "\n"))
	}
	row := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ · start · (cook-\d{6}-[0-9a-f]{4}) · cook · hub · -$`)
	logIDs := map[string]bool{}
	for _, l := range lines {
		m := row.FindStringSubmatch(l)
		if m == nil {
			t.Fatalf("torn or invalid line %q", l)
		}
		logIDs[m[1]] = true
	}
	if len(logIDs) != n {
		t.Fatalf("log holds %d distinct ids, want %d", len(logIDs), n)
	}
}

// SC-13
func TestTrack_EndCountsLedgerTasks(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "cook")
	ledger := filepath.Join(t.TempDir(), "progress.md")
	body := "Task 1: complete (commits a)\nnotes\nTask 2: complete\nTask 3: in progress\nTask 3: complete\n"
	if err := os.WriteFile(ledger, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runTrack(t, ws, "end", id, "--branch", "me/hotfix/x", "--ledger", ledger, "--rounds", "1"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(trackLog(t, ws), "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, " · end · "+id+" · branch:me/hotfix/x · 3 · 1") {
		t.Fatalf("end row = %q", last)
	}
}

// SC-25
func TestTrack_ShipBlockedStepRules(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "cook")
	if _, err := runTrack(t, ws, "ship", id, "--outcome", "blocked", "--blocked-step", "foo", "--findings", "-", "--rounds", "0"); err == nil {
		t.Fatal("unknown blocked step must fail")
	}
	if _, err := runTrack(t, ws, "ship", id, "--outcome", "blocked", "--blocked-step", "lint", "--findings", "-", "--rounds", "0"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(trackLog(t, ws), "\n"), "\n")
	if got := lines[len(lines)-1]; !strings.HasSuffix(got, " · ship · "+id+" · blocked · lint · - · 0") {
		t.Fatalf("ship row = %q", got)
	}
}
