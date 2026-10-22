package cli

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

type baseFakeRunner struct {
	out map[string]string
	err map[string]bool
}

func (f baseFakeRunner) Run(dir string, args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	if f.err[k] {
		return nil, errors.New("boom")
	}
	return []byte(f.out[k]), nil
}

func TestResolveWtBase(t *testing.T) {
	const key = "config --worktree --get wt.base"
	cfg := func(string) (*wt.Config, error) { return &wt.Config{BaseRef: "origin/staging"}, nil }
	noCfg := func(string) (*wt.Config, error) { return nil, errors.New("no worktree.json") }
	cases := []struct {
		name string
		r    baseFakeRunner
		load func(string) (*wt.Config, error)
		want string
	}{
		{"recorded wins", baseFakeRunner{out: map[string]string{key: "origin/feat-a\n"}}, cfg, "origin/feat-a"},
		{"empty falls to baseRef", baseFakeRunner{out: map[string]string{key: ""}}, cfg, "origin/staging"},
		{"git error falls to baseRef", baseFakeRunner{err: map[string]bool{key: true}}, cfg, "origin/staging"},
		{"both missing", baseFakeRunner{}, noCfg, "origin/main"},
		{"nil load func", baseFakeRunner{err: map[string]bool{key: true}}, nil, "origin/main"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveWtBase(c.r, "/wt", c.load); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestWtBase_Integration_PrintsRecordedBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := initGitRepo(t)
	git := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...) //nolint:gosec // G204 -- test, fixed binary
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	wtPath := filepath.Join(root, ".worktrees", "w1")
	git(root, "config", "extensions.worktreeConfig", "true")
	git(root, "worktree", "add", "-q", "-b", "namph/feat/w1", wtPath)
	git(wtPath, "config", "--worktree", "wt.base", "origin/feat-a")

	t.Chdir(wtPath)
	out, err := runWt(t, root, "base")
	if err != nil {
		t.Fatalf("wt base: %v\n%s", err, out)
	}
	if out != "origin/feat-a\n" {
		t.Fatalf("stdout = %q, want %q", out, "origin/feat-a\n")
	}

	// A worktree with no wt.base falls back to worktree.json baseRef ("main").
	git(wtPath, "config", "--worktree", "--unset", "wt.base")
	out, err = runWt(t, root, "base")
	if err != nil || out != "main\n" {
		t.Fatalf("fallback: out=%q err=%v", out, err)
	}
}
