package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mkWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	zdir := filepath.Join(ws, ".zenify")
	if err := os.MkdirAll(zdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zdir, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// SC-6: outside a workspace => "{}" exit 0, no dispatch.
func TestHooksRun_OutsideWorkspace(t *testing.T) {
	outside := t.TempDir() // no .zenify
	if root, ok := findWorkspaceRoot(outside); ok {
		t.Fatalf("unexpectedly found workspace root %q", root)
	}
	var buf bytes.Buffer
	code := dispatchHook("docs-sync", "", &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("output = %q, want {}", buf.String())
	}
}

// SC-7: inside workspace => root found; dispatch reaches known id.
func TestHooksRun_InsideWorkspaceFindsRoot(t *testing.T) {
	ws := mkWorkspace(t)
	nested := filepath.Join(ws, "repos", "contact-center-be")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := findWorkspaceRoot(nested)
	if !ok {
		t.Fatal("expected to find workspace from nested dir")
	}
	if got != ws {
		t.Fatalf("root = %q, want %q", got, ws)
	}
}

// FR-5.2: unknown id => "{}" exit 0 (never crash user session).
func TestHooksRun_UnknownID(t *testing.T) {
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	code := dispatchHook("totally-unknown", ws, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("output = %q, want {}", buf.String())
	}
}

func mkGitRepo(t *testing.T, ws, name, branch string, dirtyTree bool) {
	t.Helper()
	dir := filepath.Join(ws, "repos", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", branch}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if dirtyTree {
		if err := os.WriteFile(filepath.Join(dir, "wip"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// SC-4: git-state (SessionStart) prints the block; git-state-stop prints JSON.
func TestHooksRun_GitState(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "") // the hook falls back to wsRoot; isolate from the host env
	ws := mkWorkspace(t)
	if err := os.MkdirAll(filepath.Join(ws, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".claude", "deploy-branches"), []byte("staging\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mkGitRepo(t, ws, "a", "staging", true)
	mkGitRepo(t, ws, "b", "namph/feat/x", false)

	var buf bytes.Buffer
	if code := dispatchHook("git-state", ws, &buf); code != 0 {
		t.Fatalf("exit %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "2 repo(s) in scope.") || strings.Count(out, "⚠ EDITING ON A DEPLOY BRANCH") != 1 {
		t.Fatalf("session block wrong:\n%s", out)
	}

	buf.Reset()
	dispatchHook("git-state-stop", ws, &buf)
	var msg map[string]string
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("stop output not JSON: %q", buf.String())
	}
	if !strings.Contains(msg["systemMessage"], "a[staging]") {
		t.Fatalf("stop alarm missing: %q", buf.String())
	}

	// clean workspace => "{}"
	ws2 := mkWorkspace(t)
	mkGitRepo(t, ws2, "b", "namph/feat/x", false)
	buf.Reset()
	dispatchHook("git-state-stop", ws2, &buf)
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("clean stop = %q, want {}", buf.String())
	}
	// outside workspace => "{}" (existing contract — dispatchHook's own
	// wsRoot=="" no-op, unrelated to runGitStateHook's empty-report handling)
	buf.Reset()
	dispatchHook("git-state", "", &buf)
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("outside = %q, want {}", buf.String())
	}
	// inside a workspace but scope finds no repos, Session mode => nothing
	// (SessionStart stdout is injected verbatim as context; a literal "{}"
	// would land in it as noise)
	ws3 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws3, ".zenify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws3, ".zenify", "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	dispatchHook("git-state", ws3, &buf)
	if buf.String() != "" {
		t.Fatalf("empty scope session = %q, want empty", buf.String())
	}
}
