package cli

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

func TestScanSources_MatchesByNormalizedRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("requires git")
	}
	parent := t.TempDir()
	be := filepath.Join(parent, "be-clone") // dir name ≠ repo name on purpose
	other := filepath.Join(parent, "other")
	wt := filepath.Join(parent, "with-wt")
	for _, d := range []string{be, other, wt} {
		runGit(t, "", "init", "-q", d)
		runGit(t, d, "config", "user.email", "t@t")
		runGit(t, d, "config", "user.name", "t")
		writeFile(t, filepath.Join(d, "f"), "x")
		runGit(t, d, "add", ".")
		runGit(t, d, "commit", "-qm", "i")
	}
	runGit(t, be, "remote", "add", "origin", "git@github.com:X/be.git")
	runGit(t, other, "remote", "add", "origin", "https://github.com/X/unrelated.git")
	runGit(t, wt, "remote", "add", "origin", "git@github.com:X/web.git")
	runGit(t, wt, "worktree", "add", "-q", filepath.Join(wt, ".worktrees", "a"), "-b", "a")

	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{
		{Name: "be", URL: "https://github.com/X/be", Path: "repos/be"},
		{Name: "web", URL: "git@github.com:X/web.git", Path: "repos/web"},
	}}
	got := scanSources(m, gitx.ExecRunner(), parent)
	if s, ok := got["be"]; !ok || s.Path != be || s.HasWorktrees {
		t.Fatalf("be: %+v ok=%v", s, ok)
	}
	if s, ok := got["web"]; !ok || s.Path != wt || !s.HasWorktrees {
		t.Fatalf("web: %+v ok=%v", s, ok)
	}
	if len(got) != 2 {
		t.Fatalf("unrelated remote must not match: %v", got)
	}
}
