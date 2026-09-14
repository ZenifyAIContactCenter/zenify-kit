package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
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

// nudgeServer mimics github's releases/latest 302 and counts hits.
func nudgeServer(t *testing.T, tag string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Location", "/ZenifyAIContactCenter/zenify-kit/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = v
}

// SC-4: release build behind latest → exactly one nudge line, exit 0, digest still follows.
func TestSessionStart_NudgesWhenNewerRelease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	seedBootstrap(t, home)
	srv, hits := nudgeServer(t, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	if code := runSessionStart(mkWorkspace(t), &buf); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	out := buf.String()
	want := "zenify: v9.9.9 is available (running 0.17.3) — upgrade: "
	if strings.Count(out, "is available") != 1 || !strings.Contains(out, want) {
		t.Fatalf("nudge line missing or duplicated:\n%s", out)
	}
	if !strings.Contains(out, bootstrapMarker) {
		t.Fatalf("digest must still be emitted after the nudge:\n%s", out)
	}
	if strings.Index(out, "is available") > strings.Index(out, bootstrapMarker) {
		t.Fatalf("nudge must come before the digest:\n%s", out)
	}
	if *hits != 1 {
		t.Fatalf("hits = %d, want 1", *hits)
	}
	if _, err := os.Stat(filepath.Join(home, ".zenify", "update-check.json")); err != nil {
		t.Fatalf("cache not written under HOME/.zenify: %v", err)
	}
}

// SC-4: dev build → no request, no line.
func TestSessionStart_DevBuildSkipsUpdateCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	srv, hits := nudgeServer(t, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "dev")

	var buf bytes.Buffer
	runSessionStart(mkWorkspace(t), &buf)
	if strings.Contains(buf.String(), "is available") || *hits != 0 {
		t.Fatalf("dev build must not check: out=%q hits=%d", buf.String(), *hits)
	}
}

// SC-4: ZENIFY_NO_UPDATE_CHECK set → same as dev.
func TestSessionStart_NoUpdateCheckEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "1")
	srv, hits := nudgeServer(t, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	runSessionStart(mkWorkspace(t), &buf)
	if strings.Contains(buf.String(), "is available") || *hits != 0 {
		t.Fatalf("opt-out must not check: out=%q hits=%d", buf.String(), *hits)
	}
}

// Up to date → silent.
func TestSessionStart_UpToDateIsSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	srv, _ := nudgeServer(t, "v0.17.3")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	runSessionStart(mkWorkspace(t), &buf)
	if strings.Contains(buf.String(), "is available") {
		t.Fatalf("equal versions must not nudge: %q", buf.String())
	}
}

// Network failure → silent, exit 0 (fail-open).
func TestSessionStart_UpdateFailureIsSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	srv, _ := nudgeServer(t, "v9.9.9")
	url := srv.URL
	srv.Close()
	t.Setenv("ZENIFY_UPDATE_URL", url)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	if code := runSessionStart(mkWorkspace(t), &buf); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if strings.Contains(buf.String(), "is available") || strings.Contains(buf.String(), "update") {
		t.Fatalf("failure must be silent on stdout: %q", buf.String())
	}
}
