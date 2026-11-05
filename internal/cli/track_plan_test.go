package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// SC-2
func TestTrackPlanSet_WritesOneRow(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.md")
	var out, errW bytes.Buffer
	ids := []string{"b-261105-ab12", "https://github.com/o/r/pull/3"}
	if err := runTrackPlanSet(&out, &errW, logPath, "2026-11-05", ids); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^\S+ · plan · 2026-11-05 · b-261105-ab12 · https://github.com/o/r/pull/3\n$`)
	if !re.Match(b) {
		t.Fatalf("log = %q", b)
	}
	if out.String() != "track: plan 2 items\n" {
		t.Fatalf("stdout = %q", out.String())
	}
}

// SC-3
func TestTrackPlanSet_RejectsBadIDs(t *testing.T) {
	for _, ids := range [][]string{
		{"foo bar"}, {}, {"a · b"}, {"b-261105-ab12\n"}, {"b-261105-ab12\r"}, {"b-261105-ab12", "x y"},
	} {
		logPath := filepath.Join(t.TempDir(), "log.md")
		var out, errW bytes.Buffer
		if err := runTrackPlanSet(&out, &errW, logPath, "2026-11-05", ids); err == nil {
			t.Errorf("ids %q: want error", ids)
		}
		if _, err := os.Stat(logPath); !os.IsNotExist(err) {
			t.Errorf("ids %q: log must not be created, stat err = %v", ids, err)
		}
	}
}

func TestTrackPlanSet_DedupesIDs(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.md")
	var out, errW bytes.Buffer
	x := "b-261105-ab12"
	if err := runTrackPlanSet(&out, &errW, logPath, "2026-11-05", []string{x, x}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(logPath)
	if !regexp.MustCompile(`^\S+ · plan · 2026-11-05 · b-261105-ab12\n$`).Match(b) {
		t.Fatalf("log = %q", b)
	}
	if out.String() != "track: plan 1 items\n" {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestTrackPlanSet_CobraOnAndOff(t *testing.T) {
	off := trackWS(t, false)
	out, err := runTrack(t, off, "plan", "set", "b-261105-ab12")
	if err != nil || out != "track: off"+"\n" {
		t.Fatalf("off: out %q err %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(off, ".zenify", "track")); !os.IsNotExist(err) {
		t.Fatalf("off must not write, stat err = %v", err)
	}
	on := trackWS(t, true)
	out, err = runTrack(t, on, "plan", "set", "b-261105-ab12")
	if err != nil || out != "track: plan 1 items\n" {
		t.Fatalf("on: out %q err %v", out, err)
	}
	if !regexp.MustCompile(` · plan · \d{4}-\d{2}-\d{2} · b-261105-ab12\n$`).MatchString(trackLog(t, on)) {
		t.Fatalf("log = %q", trackLog(t, on))
	}
}
