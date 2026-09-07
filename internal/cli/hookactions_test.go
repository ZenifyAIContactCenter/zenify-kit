package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bootstrapMarker = "BOOTSTRAP-DIGEST-MARKER"

// seedBootstrap writes a distinctive BOOTSTRAP.txt under the temp HOME at the
// same materialized path readBootstrapDigest reads, so the tests genuinely
// exercise emission/suppression instead of passing because the file is absent.
func seedBootstrap(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "skills", "znf", "skills", "using-zenify-kit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BOOTSTRAP.txt"), []byte(bootstrapMarker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// FR-6.3: sentinel present in ~/.claude/CLAUDE.md => no digest emitted.
func TestSessionStart_SentinelSuppressesDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedBootstrap(t, home)
	cdir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "CLAUDE.md"),
		[]byte("# local\nznf-discipline-local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	code := runSessionStart(ws, &buf)
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if strings.Contains(buf.String(), bootstrapMarker) {
		t.Fatal("digest emitted despite sentinel")
	}
}

// FR-6.3: no sentinel => digest emitted raw (matching the shell hook's `cat`).
func TestSessionStart_EmitsDigestWithoutSentinel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedBootstrap(t, home)
	// no ~/.claude/CLAUDE.md at all => sentinelPresent() is false.
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	code := runSessionStart(ws, &buf)
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(buf.String(), bootstrapMarker) {
		t.Fatalf("digest not emitted without sentinel; got %q", buf.String())
	}
}
