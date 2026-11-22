package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// seedRepoCfg writes .claude/worktree.json for a fake repo, returning repoRoot.
func seedRepoCfg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "worktree.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHotfixBaserefStandalone(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"lumi","baseRef":"origin/staging","hotfix":{"baseStrategy":"standalone"}}`)
	out, err := resolveHotfixBaseRef(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out != "origin/staging" {
		t.Fatalf("standalone → %q, want origin/staging", out)
	}
}

func TestHotfixBaserefCustom(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"x","hotfixBaseRef":"origin/main","hotfix":{"baseStrategy":"custom"}}`)
	out, err := resolveHotfixBaseRef(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out != "origin/main" {
		t.Fatalf("custom → %q, want origin/main", out)
	}
}

func TestHotfixBaserefReleaseLatestRetired(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"x","hotfix":{"baseStrategy":"release-latest"}}`)
	if out, err := resolveHotfixBaseRef(dir); err == nil {
		t.Fatalf("release-latest must fail instead of guessing a release branch, got %q", out)
	}
}

func runHotfixBaseref(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newHotfixCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"baseref"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestHotfixBaserefDefaultsToCwdRepo(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"lumi","baseRef":"origin/staging","hotfix":{"baseStrategy":"standalone"}}`)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v: %s", err, out)
	}
	sub := filepath.Join(dir, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	got, err := runHotfixBaseref(t)
	if err != nil {
		t.Fatal(err)
	}
	if got != "origin/staging\n" {
		t.Fatalf("no-arg baseref → %q, want %q", got, "origin/staging\n")
	}
	withArg, err := runHotfixBaseref(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != withArg {
		t.Fatalf("no-arg %q != explicit-arg %q", got, withArg)
	}
}

func TestHotfixBaserefNoArgOutsideRepoFails(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	out, err := runHotfixBaseref(t)
	if err == nil {
		t.Fatalf("outside a git repo must fail, got %q", out)
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("exit code %d, want Fail", exitcode.Code(err))
	}
}
