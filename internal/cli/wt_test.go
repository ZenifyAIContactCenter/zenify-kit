package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runWt executes `wt <args...>` against a repo whose root is repoRoot, capturing
// stdout. It sets the command's working dir via the WT_REPO_ROOT test seam.
func runWt(t *testing.T, repoRoot string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("WT_REPO_ROOT", repoRoot) // test seam: skip `git rev-parse` in tests
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"wt"}, args...))
	err := root.Execute()
	return out.String(), err
}

func seedRepo(t *testing.T, root string) {
	t.Helper()
	cdir := filepath.Join(root, ".claude")
	wdir := filepath.Join(root, ".wt")
	for _, d := range []string{cdir, wdir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(cdir, "worktree.json"),
		[]byte(`{"abbrev":"ccbe","portRange":[3200,3249]}`), 0o644)
	os.WriteFile(filepath.Join(wdir, "state.json"),
		[]byte(`{"version":1,"worktrees":[{"slug":"foo","path":".worktrees/foo","ports":[3207]}]}`), 0o644)
}

func TestWtPath_ExactStdoutContract(t *testing.T) {
	root := t.TempDir()
	seedRepo(t, root)
	out, err := runWt(t, root, "path", "foo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".worktrees", "foo") + "\n"
	if out != want {
		t.Fatalf("path output not the exact contract:\n got %q\nwant %q", out, want)
	}
}

func TestWtPath_UnknownSlugErrors(t *testing.T) {
	root := t.TempDir()
	seedRepo(t, root)
	if _, err := runWt(t, root, "path", "nope"); err == nil {
		t.Fatal("want error for unknown slug")
	}
}

func TestWtConfig_PlainOutput(t *testing.T) {
	root := t.TempDir()
	seedRepo(t, root)
	out, err := runWt(t, root, "config")
	if err != nil {
		t.Fatal(err)
	}
	// worktree.json seeds abbrev + portRange; every other key falls to its
	// documented default. Assert the rendered lines match that resolution.
	for _, want := range []string{
		"abbrev=ccbe",
		"baseRef=origin/main",
		"worktreeDir=.worktrees/",
		"portEnv=PORT",
		"portRange=3200 3249",
		"deps=install",
		"user=namph",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config output missing %q:\n%s", want, out)
		}
	}
}

func TestWtPath_EmptyPathInStateErrors(t *testing.T) {
	root := t.TempDir()
	cdir := filepath.Join(root, ".claude")
	wdir := filepath.Join(root, ".wt")
	for _, d := range []string{cdir, wdir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(cdir, "worktree.json"),
		[]byte(`{"abbrev":"ccbe","portRange":[3200,3249]}`), 0o644)
	// A malformed entry with an empty path must error, not print the repo root
	// (filepath.Join(root, "") == root).
	os.WriteFile(filepath.Join(wdir, "state.json"),
		[]byte(`{"version":1,"worktrees":[{"slug":"foo","path":"","ports":[3207]}]}`), 0o644)
	out, err := runWt(t, root, "path", "foo")
	if err == nil {
		t.Fatalf("want error for empty path in state, got stdout %q", out)
	}
	if strings.TrimSpace(out) == root {
		t.Fatalf("printed repo root as if valid: %q", out)
	}
}

func TestWtConfigPort_Deterministic(t *testing.T) {
	root := t.TempDir()
	seedRepo(t, root)
	a, err := runWt(t, root, "config", "--port", "foo")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := runWt(t, root, "config", "--port", "foo")
	if a != b || strings.TrimSpace(a) == "" {
		t.Fatalf("config --port not deterministic: %q vs %q", a, b)
	}
}
