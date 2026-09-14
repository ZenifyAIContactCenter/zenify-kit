package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

// requireGit skips the test when git is not on PATH (real-git integration).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// These reuse the real-git integration harness (initGitRepo/runWt) from
// wtnew_test.go in this same package: create a repo, `wt new` a task, then
// assert `wt ls`/`wt ls --json`/`wt url` reflect it.

func TestWtLs_And_Url_Integration(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t)
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "my-task", "--type", "feat"); err != nil {
		t.Fatalf("precondition wt new failed: %v", err)
	}

	// table ls: contains header + the slug + a PORT value
	table, err := runWt(t, root, "ls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table, "SLUG") || !strings.Contains(table, "my-task") || !strings.Contains(table, "RUNNING") {
		t.Fatalf("ls table missing columns/slug: %q", table)
	}

	// json ls: parses to an array with our slug and an http url-able port
	js, err := runWt(t, root, "ls", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(js), &rows); err != nil {
		t.Fatalf("ls --json not valid JSON: %v\n%s", err, js)
	}
	found := false
	for _, r := range rows {
		if r["slug"] == "my-task" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ls --json missing my-task: %s", js)
	}

	// url: exact stdout contract
	url, err := runWt(t, root, "url", "my-task")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(url), "http://localhost:32") {
		t.Fatalf("url wrong: %q", url)
	}
}

// TestWtLsAll_JSON_SkipsBrokenRepoToStderr reproduces Important 2 from the
// final review: a repo that fails to load/list must not corrupt `wt ls --all
// --json`'s stdout array — its skip line belongs on stderr.
func TestWtLsAll_JSON_SkipsBrokenRepoToStderr(t *testing.T) {
	requireGit(t)
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	good := filepath.Join(ws, "repos", "good")
	if err := os.MkdirAll(good, 0o750); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", good, "init", "-q").CombinedOutput(); err != nil { //nolint:gosec // G204 -- test fixture
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(good, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, ".claude", "worktree.json"),
		[]byte(`{"abbrev":"good","portRange":[3200,3249],"deps":"none"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// bad: a fake ".git" dir (so gitstate.Scope enumerates it) with a valid
	// worktree.json (so Load succeeds), but no real git repository underneath
	// — `git worktree list --porcelain` fails, which is the error List surfaces.
	bad := filepath.Join(ws, "repos", "bad")
	if err := os.MkdirAll(filepath.Join(bad, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bad, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, ".claude", "worktree.json"),
		[]byte(`{"abbrev":"bad","portRange":[3250,3299],"deps":"none"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(ws)
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"wt", "ls", "--all", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("wt ls --all --json failed: %v\nstderr: %s", err, errb.String())
	}

	if !strings.Contains(errb.String(), "== bad == (skipped:") {
		t.Fatalf("expected bad repo's skip line on stderr, got %q", errb.String())
	}
	if strings.Contains(out.String(), "skipped") {
		t.Fatalf("skip line leaked onto stdout: %q", out.String())
	}
	var rows []wt.Row
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("stdout not a decodable JSON array: %v\n%s", err, out.String())
	}
}
