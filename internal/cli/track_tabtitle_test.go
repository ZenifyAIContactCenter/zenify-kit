package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeTranscript(t *testing.T, home, session, body string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", "-w-app")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ai(s string) string     { return `{"type":"ai-title","aiTitle":"` + s + `","sessionId":"S"}` + "\n" }
func custom(s string) string { return `{"type":"custom-title","customTitle":"` + s + `"}` + "\n" }

func TestReadTabTitle(t *testing.T) {
	filler := `{"type":"user","message":"` + strings.Repeat("x", 100) + `"}` + "\n"
	cases := []struct{ name, body, want string }{
		{"newest ai-title wins", ai("old") + filler + ai("new") + filler, "new"},
		{"custom-title beats a newer ai-title", custom("Mine") + ai("auto") + filler, "Mine"},
		{"newest custom-title wins", custom("a") + custom("b") + ai("c"), "b"},
		{"malformed line is skipped", ai("good") + `{"type":"ai-title","aiTitle":` + "\n", "good"},
		{"control characters dropped and trimmed", ai(`  Dev\u0007 tab\t `), "Dev tab"},
		{"no title", filler, ""},
		{"title past the 1 MiB window is not seen", ai("far") + strings.Repeat(filler, tabTitleWindow/len(filler)+10), ""},
		{"title inside the window after a partial first line", strings.Repeat(filler, tabTitleWindow/len(filler)+10) + ai("near"), "near"},
		{"complete title line exactly at the window start", filler + ai("edge") + pad(tabTitleWindow-len(ai("edge"))), "edge"},
	}
	for _, c := range cases {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeTranscript(t, home, "S1", c.body)
		got, err := readTabTitle("S1")
		if err != nil || got != c.want {
			t.Errorf("%s: got %q err %v, want %q", c.name, got, err, c.want)
		}
	}
}

// pad is one non-title line of exactly n bytes.
func pad(n int) string {
	head, tail := `{"type":"user","message":"`, `"}`+"\n"
	return head + strings.Repeat("x", n-len(head)-len(tail)) + tail
}

func TestReadTabTitleUnreadableProject(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 0 does not block reads here")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "S1", ai("x"))
	dir := filepath.Join(home, ".claude", "projects", "-w-app")
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
	if got, err := readTabTitle("S1"); err == nil {
		t.Fatalf("got %q, want a permission error", got)
	}
}

func TestReadTabTitleSkipsUnreadableOtherProject(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 0 does not block reads here")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	// "-a" sorts before "-w-app", so the unreadable dir is visited first.
	bad := filepath.Join(home, ".claude", "projects", "-a")
	if err := os.MkdirAll(bad, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, home, "S1", ai("found"))
	if err := os.Chmod(bad, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o750) })
	if got, err := readTabTitle("S1"); err != nil || got != "found" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestReadTabTitleMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got, err := readTabTitle("nope"); err != nil || got != "" {
		t.Fatalf("got %q err %v", got, err)
	}
	if got, err := readTabTitle("../x"); err != nil || got != "" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestTrackStatsRollingBoilCarriesTab(t *testing.T) {
	ws := trackWS(t, true)
	stubMergedAndTitle(t, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "S", ai("Dev track pane ship"))
	live := filepath.Join(ws, ".zenify", "track", "live")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	doc := `{"state":"ask","since":"` + time.Now().Add(-time.Minute).UTC().Format(time.RFC3339) + `","cwd":"` + filepath.Join(ws, "repos", "r") + `","id":""}`
	for f, body := range map[string]string{"S.json": doc, "S.beat": ""} {
		if err := os.WriteFile(filepath.Join(live, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Kitchen struct {
			RollingBoil []struct{ Session, Tab string } `json:"rollingBoil"`
		} `json:"kitchen"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil || len(r.Kitchen.RollingBoil) != 1 || r.Kitchen.RollingBoil[0].Tab != "Dev track pane ship" {
		t.Fatalf("rb = %+v err %v", r.Kitchen.RollingBoil, err)
	}
}
