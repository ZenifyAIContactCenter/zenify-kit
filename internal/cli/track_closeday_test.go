package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

const closeDayYPR = "https://github.com/o/r/pull/501"
const closeDayOldPR = "https://github.com/o/r/pull/502"

func cdRow(at time.Time, parts ...string) string {
	return at.UTC().Format(time.RFC3339) + trackSep + strings.Join(parts, trackSep)
}

// closeDayWS builds a workspace whose log has: yesterday's ended turn (PR closeDayYPR),
// one history row and a 20-day-old ended turn, plus 61 recent fillers per size so the old rows
// fall out of every stat window.
func closeDayWS(t *testing.T, now time.Time) (ws string, oldID string) {
	t.Helper()
	ws = trackWS(t, true)
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	yd := now.AddDate(0, 0, -1)
	yID := "cook-" + yd.Format("060102") + "-aaaa"
	oldAt := now.AddDate(0, 0, -20)
	oldID = "cook-" + oldAt.Format("060102") + "-bbbb"
	var l []string
	l = append(l,
		cdRow(oldAt, "history", "deadbeef", "cook", "repo", "3", "-"),
		cdRow(oldAt, "start", oldID, "cook", "repo", "M"),
		cdRow(oldAt.Add(time.Hour), "end", oldID, closeDayOldPR, "1", "0"),
	)
	for i := 0; i < 3*61; i++ { // 61 of each size: every size's scale window is full
		id := fmt.Sprintf("cook-261120-%04x", i)
		st := now.AddDate(0, 0, -5).Add(time.Duration(i) * time.Minute)
		l = append(l, cdRow(st, "start", id, "cook", "repo", []string{"S", "M", "L"}[i%3]),
			cdRow(st.Add(time.Second), "end", id, fmt.Sprintf("https://github.com/o/r/pull/f%d", i), "1", "0"))
	}
	l = append(l,
		cdRow(yd.Add(-time.Hour), "start", yID, "cook", "repo", "M"),
		cdRow(yd, "end", yID, closeDayYPR, "1", "0"),
	)
	if err := os.WriteFile(filepath.Join(dir, "log.md"), []byte(strings.Join(l, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := trackMerged
	// The old PR merged before the cut, so its turn can be archived; the rest merged yesterday.
	trackMerged = func(u string) (string, error) {
		if u == closeDayOldPR {
			return oldAt.Add(2 * time.Hour).UTC().Format(time.RFC3339), nil
		}
		return yd.UTC().Format(time.RFC3339), nil
	}
	t.Cleanup(func() { trackMerged = old })
	stubMergedAndTitleKeepMerged(t)
	return ws, oldID
}

func stubMergedAndTitleKeepMerged(t *testing.T) {
	t.Helper()
	oldT := trackPRTitle
	trackPRTitle = func(string) (string, error) { return "Title", nil }
	t.Cleanup(func() { trackPRTitle = oldT })
}

func readAll(t *testing.T, glob string) string {
	t.Helper()
	files, _ := filepath.Glob(glob)
	var sb strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
	}
	return sb.String()
}

// SC-1
func TestCloseDayReportsAndArchives(t *testing.T) {
	now := time.Now()
	ws, oldID := closeDayWS(t, now)
	dir := filepath.Join(ws, ".zenify", "track")
	var out, errb bytes.Buffer
	if err := runCloseDay(&out, &errb, ws, now, true); err != nil {
		t.Fatalf("%v\n%s", err, errb.String())
	}
	if !strings.Contains(out.String(), closeDayYPR) {
		t.Fatalf("report lacks PR url:\n%s", out.String())
	}
	yd := now.AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := os.Stat(filepath.Join(dir, "days", yd+".md")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "day.closed"))
	if strings.TrimSpace(string(b)) != now.Format("2006-01-02") {
		t.Fatalf("day.closed = %q", b)
	}
	arch := readAll(t, filepath.Join(dir, "archive", "*.md"))
	month := now.AddDate(0, 0, -20).UTC().Format("2006-01")
	if got := readAll(t, filepath.Join(dir, "archive", month+".md")); !strings.Contains(got, oldID) {
		t.Errorf("archive/%s.md lacks %s:\n%s", month, oldID, got)
	}
	log := trackLog(t, ws)
	for _, want := range []string{"history" + trackSep + "deadbeef", oldID} {
		if !strings.Contains(arch, want) {
			t.Errorf("archive lacks %q:\n%s", want, arch)
		}
		if strings.Contains(log, want) {
			t.Errorf("log.md still has %q", want)
		}
	}
	if !strings.Contains(log, "-aaaa") {
		t.Error("yesterday's turn must stay in log.md")
	}
}

// SC-2
func TestCloseDayOncePerDay(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	var out bytes.Buffer
	if err := runCloseDay(&out, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	before := trackLog(t, ws)
	out.Reset()
	if err := runCloseDay(&out, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || trackLog(t, ws) != before {
		t.Fatalf("second run changed something: out %q", out.String())
	}
}

func TestCloseDayMovesOldSessions(t *testing.T) {
	now := time.Now()
	ws, oldID := closeDayWS(t, now)
	dir := filepath.Join(ws, ".zenify", "track")
	sdir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sdir, 0o750); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(sdir, "s-old.log")
	live := filepath.Join(sdir, "s-live.log")
	body := func(id string) []byte {
		return []byte(cdRow(now.AddDate(0, 0, -20), "bind", id) + "\n")
	}
	if err := os.WriteFile(moved, body(oldID), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, body("cook-261120-0000"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -20)
	for _, f := range []string{moved, live} {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "sessions", "s-old.log")); err != nil {
		t.Errorf("old session not archived: %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("session of a kept id must stay: %v", err)
	}
}

func TestCloseDayPrunesOldLiveFiles(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	ldir := filepath.Join(ws, ".zenify", "track", "live")
	if err := os.MkdirAll(ldir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]time.Time{
		"s-dead.beat":   now.Add(-48 * time.Hour), // both old: removed
		"s-dead.json":   now.Add(-48 * time.Hour),
		"s-alive.json":  now.Add(-72 * time.Hour), // old state, fresh beat: kept
		"s-alive.beat":  now.Add(-10 * time.Second),
		"s-nobeat.json": now.Add(-48 * time.Hour), // no beat, old: removed
	}
	for name, at := range files {
		f := filepath.Join(ldir, name)
		if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	for name := range files {
		_, err := os.Stat(filepath.Join(ldir, name))
		gone := errors.Is(err, os.ErrNotExist)
		if err != nil && !gone {
			t.Fatal(err)
		}
		if want := !strings.HasPrefix(name, "s-alive"); gone != want {
			t.Errorf("%s removed = %v, want %v", name, gone, want)
		}
	}
}

// Review snapshots and ledger copies past the keep window are deleted; newer ones stay.
func TestCloseDayPrunesOldReviewsAndLedgers(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	dir := filepath.Join(ws, ".zenify", "track")
	old, fresh := now.AddDate(0, 0, -(closeDayKeepDays+1)), now.AddDate(0, 0, -1)
	files := map[string]time.Time{
		filepath.Join(dir, "reviews", "o__r__1.json"):        old,
		filepath.Join(dir, "reviews", "o__r__2.json"):        fresh,
		filepath.Join(dir, "ledgers", "cook-261101-aaaa.md"): old,
		filepath.Join(dir, "ledgers", "cook-261119-bbbb.md"): fresh,
	}
	for f, at := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	for f, at := range files {
		_, err := os.Stat(f)
		gone := errors.Is(err, os.ErrNotExist)
		if err != nil && !gone {
			t.Fatal(err)
		}
		if want := at.Equal(old); gone != want {
			t.Errorf("%s removed = %v, want %v", filepath.Base(f), gone, want)
		}
	}
}

func TestCloseDayNoMarkOnError(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	dir := filepath.Join(ws, ".zenify", "track")
	before := trackLog(t, ws)
	unlock, err := lockTrackLog(filepath.Join(dir, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlock() }()
	oldT := trackLockTimeout
	trackLockTimeout = 100 * time.Millisecond
	t.Cleanup(func() { trackLockTimeout = oldT })
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, true); err == nil {
		t.Fatal("expected a lock error")
	}
	if _, err := os.Stat(filepath.Join(dir, "day.closed")); !os.IsNotExist(err) {
		t.Fatalf("day.closed must be absent: %v", err)
	}
	if trackLog(t, ws) != before {
		t.Fatal("log.md changed")
	}
}

func TestCloseDayKeepsExistingArchivedDay(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	dir := filepath.Join(ws, ".zenify", "track")
	d := now.AddDate(0, 0, -30).Format("2006-01-02")
	for _, p := range []string{filepath.Join(dir, "days"), filepath.Join(dir, "archive", "days")} {
		if err := os.MkdirAll(p, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "archive", "days", d+".md"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "days", d+".md"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(dir, "archive", "days", d+".md"))
	b, _ := os.ReadFile(filepath.Join(dir, "archive", "days", d+".md.1"))
	if string(a) != "first" || string(b) != "second" {
		t.Fatalf("got %q / %q", a, b)
	}
}

// A source another close-day run already moved counts as moved.
func TestMoveIntoMissingSource(t *testing.T) {
	d := t.TempDir()
	if err := moveInto(filepath.Join(d, "gone.log"), filepath.Join(d, "archive")); err != nil {
		t.Fatalf("missing source must be skipped, got %v", err)
	}
}

// Parallel first sessions (tmux restore) all close the day without an error.
func TestCloseDayParallelRuns(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	sdir := filepath.Join(ws, ".zenify", "track", "sessions")
	if err := os.MkdirAll(sdir, 0o750); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -20)
	for i := 0; i < 20; i++ {
		f := filepath.Join(sdir, fmt.Sprintf("s-%d.log", i))
		if err := os.WriteFile(f, []byte(cdRow(old, "bind", fmt.Sprintf("cook-x-%d", i))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { errs <- runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, false) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-errs; err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
}

// A run that reaches the log lock after another run closed the day skips compaction.
func TestCloseDayRechecksUnderLock(t *testing.T) {
	now := time.Now()
	ws, _ := closeDayWS(t, now)
	dir := filepath.Join(ws, ".zenify", "track")
	before := trackLog(t, ws)
	unlock, err := lockTrackLog(filepath.Join(dir, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, false) }()
	report := filepath.Join(dir, "days", now.AddDate(0, 0, -1).Format("2006-01-02")+".md")
	for i := 0; ; i++ {
		if _, err := os.Stat(report); err == nil {
			break
		}
		if i > 200 {
			t.Fatal("report never written")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(dir, "day.closed"), []byte(now.Format("2006-01-02")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if trackLog(t, ws) != before {
		t.Fatal("log.md compacted although the day was already closed")
	}
}

// After a break longer than the keep window, the report written by this run stays in days/.
func TestCloseDayKeepsOwnReportAfterLongBreak(t *testing.T) {
	now := time.Now()
	ws := trackWS(t, true)
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	at := now.AddDate(0, 0, -20)
	id := "cook-" + at.Format("060102") + "-cccc"
	log := cdRow(at, "start", id, "cook", "repo", "S") + "\n" + cdRow(at.Add(time.Minute), "end", id, closeDayOldPR, "1", "0") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "log.md"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	stubTrackGh(t)
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now, true); err != nil {
		t.Fatal(err)
	}
	d := at.Format("2006-01-02")
	if _, err := os.Stat(filepath.Join(dir, "days", d+".md")); err != nil {
		t.Errorf("own report moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "days", d+".md")); !os.IsNotExist(err) {
		t.Errorf("own report must not be archived: %v", err)
	}
}

// A session log a parallel run archived between the glob and the read is skipped.
func TestReadSessionLogsSkipsVanished(t *testing.T) {
	dir := t.TempDir()
	sdir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sdir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdir, "a.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A dangling symlink is listed by the glob and reads as ErrNotExist, like a file moved in between.
	if err := os.Symlink(filepath.Join(dir, "gone.log"), filepath.Join(sdir, "b.log")); err != nil {
		t.Fatal(err)
	}
	got, err := readSessionLogs(dir)
	if err != nil {
		t.Fatalf("vanished session log must be skipped, got %v", err)
	}
	if len(got) != 1 || got["a"] != "x" {
		t.Fatalf("got %v", got)
	}
}

// feedbackWS is a workspace whose yesterday has one ended run with PR fbPR, merged yesterday
// in merged.json, with a review snapshot; feedback sets the track.feedback feature.
func feedbackWS(t *testing.T, now time.Time, feedback bool) (ws, day string) {
	t.Helper()
	const fbPR = "https://github.com/o/r/pull/1"
	ws = t.TempDir()
	overlay := "features:\n  track: true\n"
	if feedback {
		overlay += "  track.feedback: true\n"
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify-overlay.yaml"), []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.MkdirAll(filepath.Join(dir, "reviews"), 0o750); err != nil {
		t.Fatal(err)
	}
	yd := now.AddDate(0, 0, -1)
	id := "cook-" + yd.Format("060102") + "-aaaa"
	log := strings.Join([]string{
		cdRow(yd.Add(-time.Hour), "start", id, "cook", "repo", "M"),
		cdRow(yd.Add(-30*time.Minute), "ship", id, "shipped", "-", "0,0,0,0", "1"),
		cdRow(yd, "end", id, fbPR, "1", "0"),
	}, "\n") + "\n"
	files := map[string]string{
		"log.md":               log,
		"merged.json":          `{"` + fbPR + `":"` + yd.UTC().Format(time.RFC3339) + `"}`,
		"reviews/o__r__1.json": `{"url":"` + fbPR + `","threads":[{"comments":[{"authorType":"Bot","body":"HIGH: leak"}]}]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws, yd.Format("2006-01-02")
}

// noGhCalls fails the test when close-day reaches gh.
func noGhCalls(t *testing.T) {
	t.Helper()
	oldM, oldR := trackMerged, trackReview
	trackMerged = func(u string) (string, error) { t.Errorf("gh merged call for %s", u); return "", nil }
	trackReview = func(u string) (trackstats.ReviewSnapshot, error) {
		t.Errorf("gh review call for %s", u)
		return trackstats.ReviewSnapshot{}, nil
	}
	t.Cleanup(func() { trackMerged, trackReview = oldM, oldR })
}

// SC-13, SC-9
func TestCloseDayFeedbackFollowsFeature(t *testing.T) {
	now := time.Now()
	noGhCalls(t)
	for _, on := range []bool{false, true} {
		ws, day := feedbackWS(t, now, on)
		var out, errb bytes.Buffer
		if err := runCloseDay(&out, &errb, ws, now, false); err != nil {
			t.Fatalf("%v\n%s", err, errb.String())
		}
		b, err := os.ReadFile(filepath.Join(ws, ".zenify", "track", "days", day+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(b), "## Feedback"); got != on {
			t.Fatalf("track.feedback %t: Feedback section present = %t\n%s", on, got, b)
		}
		if on {
			for _, want := range []string{"### https://github.com/o/r/pull/1", "- review data: .zenify/track/reviews/o__r__1.json", "escaped:"} {
				if !strings.Contains(string(b), want) {
					t.Errorf("report lacks %q:\n%s", want, b)
				}
			}
		}
	}
}

// A broken snapshot is reported on stderr and counted as missing.
func TestCloseDayFeedbackBrokenSnapshot(t *testing.T) {
	now := time.Now()
	noGhCalls(t)
	ws, day := feedbackWS(t, now, true)
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "reviews", "o__r__1.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := runCloseDay(&out, &errb, ws, now, false); err != nil {
		t.Fatalf("%v\n%s", err, errb.String())
	}
	if !strings.Contains(errb.String(), "track: read review o__r__1.json: ") {
		t.Errorf("stderr = %q", errb.String())
	}
	b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "track", "days", day+".md"))
	if !strings.Contains(string(b), "- review data: missing") {
		t.Errorf("report:\n%s", b)
	}
}

// A snapshot whose url is another PR's is counted as missing.
func TestCloseDayFeedbackSnapshotURLMismatch(t *testing.T) {
	now := time.Now()
	noGhCalls(t)
	ws, day := feedbackWS(t, now, true)
	body := `{"url":"https://github.com/o/r/pull/99","threads":[{"comments":[{"authorType":"Bot","body":"HIGH: leak"}]}]}`
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "reviews", "o__r__1.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := runCloseDay(&out, &errb, ws, now, false); err != nil {
		t.Fatalf("%v\n%s", err, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(ws, ".zenify", "track", "days", day+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "- review data: missing") || strings.Contains(string(b), "escaped:") {
		t.Errorf("report:\n%s", b)
	}
}

// SC-14: with track.feedback off the ledger copy and the review snapshot still happen.
func TestFeedbackOffStillCollects(t *testing.T) {
	pr := "https://github.com/o/r/pull/1"
	ws := trackWS(t, true)
	id := startTrack(t, ws, "cook")
	ledger := filepath.Join(t.TempDir(), "progress.md")
	if err := os.WriteFile(ledger, []byte("# SDD ledger — plan: docs/plans/x.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runTrackErr(t, ws, "end", id, "--pr", pr, "--ledger", ledger, "--rounds", "0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "merged.json"), []byte(`{"`+pr+`":"2026-11-18T05:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	oldM, oldT := trackMerged, trackPRTitle
	trackMerged = func(string) (string, error) { return "", nil }
	trackPRTitle = func(string) (string, error) { return "", nil }
	t.Cleanup(func() { trackMerged, trackPRTitle = oldM, oldT })
	stubReview(t, nil)
	reviewStats(t, ws)
	for _, f := range []string{filepath.Join("ledgers", id+".md"), filepath.Join("reviews", "o__r__1.json")} {
		if _, err := os.Stat(filepath.Join(ws, ".zenify", "track", f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// FR-6.4
func TestTrackStatsFeedbackField(t *testing.T) {
	for _, on := range []bool{false, true} {
		ws, _ := feedbackWS(t, time.Now(), on)
		if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "merged.checked"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := runTrack(t, ws, "stats", "--json")
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf(`"feedback": %t`, on); !strings.Contains(out, want) {
			t.Errorf("stats lacks %s", want)
		}
	}
}
