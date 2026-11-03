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
	oldTitle := trackPRTitle
	trackPRTitle = func(string) (string, error) { return "", nil }
	t.Cleanup(func() { trackPRTitle = oldTitle })
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
// per mergedRecheck; a failed check is stamped too and its error stays reported until the
// next check.
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
	oldTitle := trackPRTitle
	trackPRTitle = func(string) (string, error) { return "", nil }
	t.Cleanup(func() { trackPRTitle = oldTitle })
	stats := func() string {
		t.Helper()
		out, err := runTrack(t, ws, "stats", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var r struct {
			Today struct {
				MergedError string `json:"mergedError"`
			} `json:"today"`
		}
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			t.Fatal(err)
		}
		return r.Today.MergedError
	}
	stamp := filepath.Join(ws, ".zenify", "track", "merged.checked")
	age := func() {
		t.Helper()
		old := time.Now().Add(-mergedRecheck - time.Minute)
		if err := os.Chtimes(stamp, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if e := stats(); asked != 1 || !strings.Contains(e, "gh down") {
		t.Fatalf("first run: asked %d err %q, want 1 and gh down", asked, e)
	}
	if e := stats(); asked != 1 || !strings.Contains(e, "gh down") {
		t.Fatalf("within %s: asked %d err %q, want no new ask and the error kept", mergedRecheck, asked, e)
	}
	fail = false
	age()
	if e := stats(); asked != 2 || e != "" {
		t.Fatalf("stale stamp: asked %d err %q, want a new ask and no error", asked, e)
	}
	stats()
	if asked != 2 {
		t.Fatalf("an unmerged PR must not be asked again within %s: asked %d", mergedRecheck, asked)
	}
}

// FR-7.2: a start never reuses an id that already has a start row.
func TestTrack_StartSkipsUsedID(t *testing.T) {
	ws := trackWS(t, true)
	logPath := filepath.Join(ws, ".zenify", "track", "log.md")
	first := startTrack(t, ws, "fix")
	if used, err := trackIDStarted(logPath, first); err != nil || !used {
		t.Fatalf("trackIDStarted(%s) = %v %v, want true", first, used, err)
	}
	if used, err := trackIDStarted(logPath, "fix-000101-0000"); err != nil || used {
		t.Fatalf("trackIDStarted(unknown) = %v %v, want false", used, err)
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

// A successful end prints `track: ended <id>` (the hook unbinds on it); a rejected end prints nothing.
func TestTrack_EndPrintsEndedLine(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "fix")
	if out, err := runTrack(t, ws, "end", id, "--rounds", "0"); err == nil || out != "" {
		t.Fatalf("end without an outcome: out %q err %v, want an error and no output", out, err)
	}
	if out, err := runTrack(t, ws, "end", "--cancelled", id, "--rounds", "0"); err != nil || out != "track: ended "+id+"\n" {
		t.Fatalf("end: out %q err %v", out, err)
	}
}

// runTrackErr is runTrack that also returns what the command wrote to stderr.
func runTrackErr(t *testing.T, ws string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var errBuf bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&errBuf)
	root.SetArgs(append(append([]string{"track"}, args...), "--workspace", ws))
	err := root.Execute()
	return errBuf.String(), err
}

func endWithLedger(t *testing.T, body string) (last, stderr string) {
	t.Helper()
	ws := trackWS(t, true)
	id := startTrack(t, ws, "cook")
	ledger := filepath.Join(t.TempDir(), "progress.md")
	if err := os.WriteFile(ledger, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr, err := runTrackErr(t, ws, "end", id, "--cancelled", "--ledger", ledger, "--rounds", "0")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(trackLog(t, ws), "\n"), "\n")
	return lines[len(lines)-1], stderr
}

func TestTrackEndLedgerPlan(t *testing.T) {
	last, stderr := endWithLedger(t, "# SDD ledger — plan: /ws/docs/plans/zenify-kit/2026-11-02-x.md\nTask 1: complete\nTask 2: complete\n")
	if !strings.HasSuffix(last, " · 2 · 0 · /ws/docs/plans/zenify-kit/2026-11-02-x.md") || stderr != "" {
		t.Fatalf("row=%q stderr=%q", last, stderr)
	}
}

func TestTrackEndLedgerNoPlan(t *testing.T) {
	last, stderr := endWithLedger(t, "# progress\n")
	if !strings.HasSuffix(last, " · cancelled · 0 · 0") || !strings.Contains(stderr, `has no "# SDD ledger — plan:" first line`) {
		t.Fatalf("row=%q stderr=%q", last, stderr)
	}
}

func TestTrackEndLedgerPlanWithBareCR(t *testing.T) {
	last, stderr := endWithLedger(t, "# SDD ledger — plan: /a\rb.md\nTask 1: complete\n")
	if !strings.HasSuffix(last, " · cancelled · 1 · 0") || !strings.Contains(stderr, "plan path") {
		t.Fatalf("row=%q stderr=%q", last, stderr)
	}
}

func TestTrackEndLedgerEmptyPlanPath(t *testing.T) {
	for _, body := range []string{"# SDD ledger — plan: \n", "# SDD ledger — plan:\n"} {
		last, stderr := endWithLedger(t, body)
		if !strings.HasSuffix(last, " · cancelled · 0 · 0") || !strings.Contains(stderr, "has an empty plan path; end row written without the plan") {
			t.Fatalf("%q: row=%q stderr=%q", body, last, stderr)
		}
	}
}

func TestReadLiveFilesSkipsVanishedJSON(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	// A dangling symlink is listed by the glob but its read fails with ErrNotExist.
	if err := os.Symlink(filepath.Join(live, "nowhere"), filepath.Join(live, "S.json")); err != nil {
		t.Skip("symlink:", err)
	}
	if err := os.WriteFile(filepath.Join(live, "S.beat"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readLiveFiles(dir, time.Now())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestTrackEndLedgerPlanWithSeparator(t *testing.T) {
	last, stderr := endWithLedger(t, "# SDD ledger — plan: /a · b.md\n")
	if !strings.HasSuffix(last, " · cancelled · 0 · 0") || !strings.Contains(stderr, "plan path") {
		t.Fatalf("row=%q stderr=%q", last, stderr)
	}
}

const kitchenPR = "https://github.com/o/r/pull/7"

// kitchenLog writes a log whose item ended just now, with an optional plan field.
func kitchenLog(t *testing.T, ws, endTarget, plan string) {
	t.Helper()
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute).Format("2006-01-02T15:04:05Z")
	id := "cook-" + time.Now().Format("060102") + "-abcd"
	end := at + " · end · " + id + " · " + endTarget + " · 1 · 0"
	if plan != "" {
		end += " · " + plan
	}
	if err := os.WriteFile(filepath.Join(dir, "log.md"), []byte(at+" · start · "+id+" · cook · r · M\n"+end+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

type kitchenOut struct {
	Kitchen struct {
		Cooked []struct {
			Brief string `json:"brief"`
		} `json:"cooked"`
		RollingBoil []struct {
			Session, Brief, State string
		} `json:"rollingBoil"`
		TitleError string `json:"titleError"`
	} `json:"kitchen"`
}

func kitchenStats(t *testing.T, ws string) kitchenOut {
	t.Helper()
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var r kitchenOut
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return r
}

func stubMergedAndTitle(t *testing.T, title string) *int {
	t.Helper()
	oldM, oldT := trackMerged, trackPRTitle
	calls := 0
	trackMerged = func(string) (string, error) { return "", nil }
	trackPRTitle = func(string) (string, error) { calls++; return title, nil }
	t.Cleanup(func() { trackMerged, trackPRTitle = oldM, oldT })
	return &calls
}

func TestTrackStatsPRTitleCachedOnce(t *testing.T) {
	ws := trackWS(t, true)
	kitchenLog(t, ws, kitchenPR, "")
	calls := stubMergedAndTitle(t, "P")
	dir := filepath.Join(ws, ".zenify", "track")
	for i := 0; i < 2; i++ {
		_ = os.Remove(filepath.Join(dir, "merged.checked"))
		r := kitchenStats(t, ws)
		if len(r.Kitchen.Cooked) != 1 || r.Kitchen.Cooked[0].Brief != "P" {
			t.Fatalf("run %d: cooked %+v", i, r.Kitchen.Cooked)
		}
	}
	if *calls != 1 {
		t.Fatalf("pr title asked %d times, want 1", *calls)
	}
	b, err := os.ReadFile(filepath.Join(dir, "pr-titles.json"))
	if err != nil || string(b) != `{"`+kitchenPR+`":"P"}` {
		t.Fatalf("pr-titles.json = %q err %v", b, err)
	}
}

func TestTrackStatsPRTitleThrottled(t *testing.T) {
	ws := trackWS(t, true)
	kitchenLog(t, ws, kitchenPR, "")
	calls := stubMergedAndTitle(t, "P")
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.WriteFile(filepath.Join(dir, "merged.checked"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r := kitchenStats(t, ws)
	if *calls != 0 {
		t.Fatalf("pr title asked %d times while throttled", *calls)
	}
	if len(r.Kitchen.Cooked) != 1 || r.Kitchen.Cooked[0].Brief != "cook r" {
		t.Fatalf("cooked %+v", r.Kitchen.Cooked)
	}
	// Once the stamp is gone the same run asks gh again, exactly once.
	if err := os.Remove(filepath.Join(dir, "merged.checked")); err != nil {
		t.Fatal(err)
	}
	r = kitchenStats(t, ws)
	if *calls != 1 {
		t.Fatalf("pr title asked %d times after the stamp was removed, want 1", *calls)
	}
	if len(r.Kitchen.Cooked) != 1 || r.Kitchen.Cooked[0].Brief != "P" {
		t.Fatalf("cooked %+v", r.Kitchen.Cooked)
	}
}

func TestTrackStatsLiveFiles(t *testing.T) {
	ws := trackWS(t, true)
	live := filepath.Join(ws, ".zenify", "track", "live")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-60 * time.Second).UTC().Format(time.RFC3339)
	doc := `{"state":"ask","since":"` + since + `","cwd":"` + filepath.Join(ws, "repos", "r") + `","id":""}`
	for _, s := range []string{"S", "T"} {
		if err := os.WriteFile(filepath.Join(live, s+".json"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(live, "S.beat"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r := kitchenStats(t, ws)
	if len(r.Kitchen.RollingBoil) != 1 {
		t.Fatalf("rollingBoil %+v", r.Kitchen.RollingBoil)
	}
	if b := r.Kitchen.RollingBoil[0]; b.Session != "S" || b.Brief != "r" || b.State != "ask" {
		t.Fatalf("rollingBoil[0] = %+v", b)
	}
}

func TestTrackStatsPlanTitle(t *testing.T) {
	ws := trackWS(t, true)
	stubMergedAndTitle(t, "")
	if err := os.MkdirAll(filepath.Join(ws, "docs", "plans", "r"), 0o750); err != nil {
		t.Fatal(err)
	}
	plan := "# Foo — Implementation Plan\n\nbody\n"
	if err := os.WriteFile(filepath.Join(ws, "docs", "plans", "r", "p.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "docs", "plans", "r", "nohead.md"), []byte("no heading here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(t.TempDir(), "abs.md")
	if err := os.WriteFile(abs, []byte("# Bar — Implementation Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ plan, want string }{
		{abs, "Bar"},
		{"docs/plans/r/p.md", "Foo"},
		{"docs/plans/r/gone.md", "cook r"},
		{"docs/plans/r/nohead.md", "cook r"},
	} {
		kitchenLog(t, ws, "branch:x", c.plan)
		r := kitchenStats(t, ws)
		if len(r.Kitchen.Cooked) != 1 || r.Kitchen.Cooked[0].Brief != c.want || r.Kitchen.TitleError != "" {
			t.Fatalf("plan %q: cooked %+v titleError %q, want brief %q", c.plan, r.Kitchen.Cooked, r.Kitchen.TitleError, c.want)
		}
	}
}

func TestTrackEndMissingLedgerWritesThreeFieldRow(t *testing.T) {
	ws := trackWS(t, true)
	id := startTrack(t, ws, "cook")
	stderr, err := runTrackErr(t, ws, "end", id, "--cancelled", "--ledger", filepath.Join(ws, "missing.md"), "--rounds", "2")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(trackLog(t, ws), "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, " · cancelled · 0 · 2") {
		t.Fatalf("row = %q", last)
	}
	if !strings.Contains(stderr, "track: read --ledger ") || !strings.Contains(stderr, "end row written without tasks or plan") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestTrackEndRelativePlanStoredVerbatim(t *testing.T) {
	last, _ := endWithLedger(t, "# SDD ledger — plan: docs/plans/r/p.md\nTask 1: complete\n")
	if !strings.HasSuffix(last, " · 1 · 0 · docs/plans/r/p.md") {
		t.Fatalf("row = %q", last)
	}
}

func TestReadPlanTitleLongFirstLineGivesNoTitle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.md")
	if err := os.WriteFile(p, []byte("# "+strings.Repeat("a", 5000)+"\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readPlanTitle("", p); err != nil || got != "" {
		t.Fatalf("got %d bytes, err %v", len(got), err)
	}
}

func TestReadPlanTitleNotRegularFile(t *testing.T) {
	got, err := readPlanTitle(t.TempDir(), t.TempDir())
	if err != nil || got != "" {
		t.Fatalf("dir: got %q err %v", got, err)
	}
	p := filepath.Join(t.TempDir(), "p.md")
	if err := os.WriteFile(p, []byte("# T\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readPlanTitle("", p); err != nil || got != "T" {
		t.Fatalf("file: got %q err %v", got, err)
	}
}

func TestTrackStatsSkipsReadingExpiredLiveJSON(t *testing.T) {
	ws := trackWS(t, true)
	stubMergedAndTitle(t, "")
	live := filepath.Join(ws, ".zenify", "track", "live")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	j := filepath.Join(live, "S.json")
	if err := os.WriteFile(j, []byte("{corrupt"), 0o000); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(live, "S.beat")
	if err := os.WriteFile(b, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(b, old, old); err != nil {
		t.Fatal(err)
	}
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil {
		t.Fatalf("expired live json was read: %v", err)
	}
	var r struct{ Malformed int }
	if err := json.Unmarshal([]byte(out), &r); err != nil || r.Malformed != 0 {
		t.Fatalf("malformed = %d err %v", r.Malformed, err)
	}
}

func TestTrackStatsTextCountsDone(t *testing.T) {
	ws := trackWS(t, true)
	kitchenLog(t, ws, "branch:x", "")
	stubMergedAndTitle(t, "P")
	out, err := runTrack(t, ws, "stats")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 started, 1 done, ") {
		t.Fatalf("stats text = %q", out)
	}
}
