package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
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

func TestDropInPlaceSources(t *testing.T) {
	ws := t.TempDir()
	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{
		{Name: "be", URL: "https://github.com/X/be", Path: "repos/be"},
		{Name: "web", URL: "https://github.com/X/web", Path: "repos/web"},
	}}

	sources := map[string]reconcile.Source{
		// a sources dir that contains the workspace resolves "be" to exactly
		// its own destination — must be dropped, or reconcile turns it SKIP.
		"be": {Path: filepath.Join(ws, "repos", "be")},
		// a source elsewhere on disk must be kept untouched.
		"web": {Path: filepath.Join(t.TempDir(), "web-clone")},
	}

	got := dropInPlaceSources(m, ws, sources)
	if _, ok := got["be"]; ok {
		t.Fatalf("in-place source for be must be dropped: %+v", got)
	}
	if s, ok := got["web"]; !ok || s.Path != sources["web"].Path {
		t.Fatalf("source elsewhere must be kept: %+v", got)
	}
	if len(got) != 1 {
		t.Fatalf("want exactly 1 surviving source, got %+v", got)
	}
}

func TestDropInPlaceSources_RelativeWorkspace(t *testing.T) {
	// The workspace and the source path are both handed in relative to cwd —
	// dropInPlaceSources must filepath.Abs both sides before comparing, or a
	// relative workspace (e.g. "." from a caller that never absolutized it)
	// would never match an absolute source path for the same directory.
	parent := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	defer func() { _ = os.Chdir(wd) }()

	if err := os.MkdirAll(filepath.Join("ws", "repos", "be"), 0o750); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Org: "X", Repos: []manifest.Repo{
		{Name: "be", URL: "https://github.com/X/be", Path: "repos/be"},
	}}
	abs, _ := filepath.Abs(filepath.Join("ws", "repos", "be"))
	sources := map[string]reconcile.Source{"be": {Path: abs}}

	got := dropInPlaceSources(m, "ws", sources) // relative workspace
	if _, ok := got["be"]; ok {
		t.Fatalf("relative workspace must still resolve and drop the in-place source: %+v", got)
	}
}
