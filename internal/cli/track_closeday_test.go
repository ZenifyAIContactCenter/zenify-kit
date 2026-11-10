package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const closeDayYPR = "https://github.com/o/r/pull/501"
const closeDayOldPR = "https://github.com/o/r/pull/502"

func cdRow(at time.Time, parts ...string) string {
	return at.UTC().Format(time.RFC3339) + trackSep + strings.Join(parts, trackSep)
}

// closeDayWS builds a workspace whose log has: yesterday's ended turn (PR closeDayYPR),
// one history row and a 20-day-old ended turn, plus 61 recent fillers so the old rows
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
	for i := 0; i < 61; i++ {
		id := fmt.Sprintf("cook-261120-%04x", i)
		st := now.AddDate(0, 0, -5).Add(time.Duration(i) * time.Minute)
		l = append(l, cdRow(st, "start", id, "cook", "repo", "S"),
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
	trackMerged = func(string) (string, error) { return yd.UTC().Format(time.RFC3339), nil }
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
	if err := runCloseDay(&out, &errb, ws, now); err != nil {
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
	if err := runCloseDay(&out, &bytes.Buffer{}, ws, now); err != nil {
		t.Fatal(err)
	}
	before := trackLog(t, ws)
	out.Reset()
	if err := runCloseDay(&out, &bytes.Buffer{}, ws, now); err != nil {
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
	if err := runCloseDay(&bytes.Buffer{}, &bytes.Buffer{}, ws, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "sessions", "s-old.log")); err != nil {
		t.Errorf("old session not archived: %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("session of a kept id must stay: %v", err)
	}
}
