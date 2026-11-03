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

func fillDir(t *testing.T, n int) string {
	t.Helper()
	d := t.TempDir()
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(d, fmt.Sprintf("f%d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestTmpCountCapped_StopsAtLimit(t *testing.T) {
	d := fillDir(t, 3000)
	if n := tmpCountCapped(d, 2000, time.Second); n < 2000 {
		t.Fatalf("count = %d, want >= 2000 (capped)", n)
	}
	if n := tmpCountCapped(d, 10000, time.Second); n != 3000 {
		t.Fatalf("count = %d, want 3000", n)
	}
}

func TestTmpFloodWarning(t *testing.T) {
	d := fillDir(t, 120)
	if s := tmpFloodWarning("user temp dir", d, 100, 0, time.Second); !strings.Contains(s, ">= 100 entries") {
		t.Fatalf("flooded dir: got %q", s)
	}
	if s := tmpFloodWarning("user temp dir", d, 1000, 0, time.Second); s != "" {
		t.Fatalf("below limit must be silent, got %q", s)
	}
	// The size pre-filter skips the count entirely for a small dir.
	if s := tmpFloodWarning("user temp dir", d, 100, 1<<40, time.Second); s != "" {
		t.Fatalf("pre-filter must skip, got %q", s)
	}
	if s := tmpFloodWarning("x", filepath.Join(d, "missing"), 1, 0, time.Second); s != "" {
		t.Fatalf("missing dir must be silent, got %q", s)
	}
}

func TestTmpUptimeWarning(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	if s := tmpUptimeWarning(now.Add(-8*24*time.Hour), now, 7); !strings.Contains(s, "Uptime 8d") {
		t.Fatalf("got %q", s)
	}
	if s := tmpUptimeWarning(now.Add(-2*24*time.Hour), now, 7); s != "" {
		t.Fatalf("short uptime must be silent, got %q", s)
	}
	if s := tmpUptimeWarning(time.Time{}, now, 7); s != "" {
		t.Fatalf("unknown boot time must be silent, got %q", s)
	}
}

// The hook never prints "{}" and always exits 0.
func TestHooksRun_TmpCheckExitsZero(t *testing.T) {
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	if code := dispatchHook("tmp-check", ws, &buf); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(buf.String()) == "{}" {
		t.Fatalf("must not print {}")
	}
}
