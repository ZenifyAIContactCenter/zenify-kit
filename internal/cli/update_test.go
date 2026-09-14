package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/update"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

func updateServer(t *testing.T, status int, tag string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if tag != "" {
			w.Header().Set("Location", "/ZenifyAIContactCenter/zenify-kit/releases/tag/"+tag)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setVersionForUpdate(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = v
}

// SC-8: --check with a newer release.
func TestUpdateCheck_ReportsNewer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	srv := updateServer(t, http.StatusFound, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersionForUpdate(t, "0.17.3")

	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"update", "--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr %q)", err, errb.String())
	}
	got := strings.TrimSpace(out.String())
	if !strings.HasPrefix(got, "zenify 0.17.3 — latest v9.9.9 — upgrade: ") {
		t.Fatalf("stdout = %q", got)
	}
}

// SC-8: --check up to date.
func TestUpdateCheck_UpToDate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	srv := updateServer(t, http.StatusFound, "v0.17.3")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersionForUpdate(t, "0.17.3")

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "zenify 0.17.3 — up to date" {
		t.Fatalf("stdout = %q", out.String())
	}
}

// SC-8: --check on failure → error exit, stderr has the reason.
func TestUpdateCheck_FailureExitsNonZero(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	srv := updateServer(t, http.StatusInternalServerError, "")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersionForUpdate(t, "0.17.3")

	var out, errb bytes.Buffer
	err := runUpdateCheck(&out, &errb, updateOptions(true), "darwin", update.Unknown)
	if err == nil {
		t.Fatal("500 must fail --check")
	}
	if !strings.Contains(errb.String(), "500") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

// FR-2.4: dev build → informational line, no comparison.
func TestUpdateCheck_DevBuild(t *testing.T) {
	srv := updateServer(t, http.StatusFound, "v9.9.9")
	var out, errb bytes.Buffer
	opts := update.Options{Current: "dev", URL: srv.URL, Client: srv.Client(), Force: true}
	if err := runUpdateCheck(&out, &errb, opts, "darwin", update.Brew); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "zenify dev build — latest v9.9.9" {
		t.Fatalf("stdout = %q", out.String())
	}
}

// FR-2.3: unknown method → three commands + README link, non-zero, nothing executed.
func TestRunUpdate_UnknownPrintsInstructions(t *testing.T) {
	var errb bytes.Buffer
	ran := false
	err := runUpdate(&errb, update.Unknown, "darwin", func(string, []string) error { ran = true; return nil }, t.TempDir())
	if err == nil {
		t.Fatal("unknown method must fail")
	}
	if ran {
		t.Fatal("unknown method must not execute anything")
	}
	for _, want := range []string{"brew upgrade --cask zenify", "scoop update zenify", "install.sh | sh", "#install"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, errb.String())
		}
	}
}

// FR-2.3: known method → runs the mapped command, clears the cache on success.
func TestRunUpdate_RunsCommandAndClearsCache(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, update.CacheFile)
	if err := os.WriteFile(cache, []byte(`{"checkedAt":"2026-09-14T00:00:00Z","latest":"v9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	var gotName string
	var gotArgs []string
	err := runUpdate(&errb, update.Brew, "darwin", func(name string, args []string) error {
		gotName, gotArgs = name, args
		return nil
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "brew" || strings.Join(gotArgs, " ") != "upgrade --cask zenify" {
		t.Fatalf("ran %q %q", gotName, gotArgs)
	}
	if !strings.Contains(errb.String(), "zenify: upgrading via brew (brew upgrade --cask zenify)") {
		t.Fatalf("stderr = %q", errb.String())
	}
	if _, statErr := os.Stat(cache); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cache must be removed after a successful upgrade: %v", statErr)
	}
}

// FR-2.3: command failure propagates, cache kept.
func TestRunUpdate_CommandFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, update.CacheFile)
	if err := os.WriteFile(cache, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	boom := errors.New("exit status 1")
	err := runUpdate(&errb, update.Scoop, "windows", func(string, []string) error { return boom }, dir)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	if _, statErr := os.Stat(cache); statErr != nil {
		t.Fatalf("cache must be kept when the upgrade fails: %v", statErr)
	}
}

func TestRootCmd_HasUpdateSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	sub, _, err := cmd.Find([]string{"update"})
	if err != nil || sub == nil || sub.Name() != "update" {
		t.Fatalf("update subcommand not registered: %v", err)
	}
	if sub.Flags().Lookup("check") == nil {
		t.Fatal("update has no --check flag")
	}
}
