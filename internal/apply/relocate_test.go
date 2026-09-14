package apply

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("requires git")
	}
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "config", "user.email", "t@t"}, {"-C", dir, "config", "user.name", "t"}} {
		if o, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, o)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", dir, "add", "."}, {"-C", dir, "commit", "-qm", "init"}} {
		if o, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, o)
		}
	}
}

func TestApply_Relocate_MovesLinksAndWires(t *testing.T) {
	ws := t.TempDir()
	src := filepath.Join(t.TempDir(), "projects", "be")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}
	gitInit(t, src)
	// dirty source is NOT a blocker
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("v1\ndirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{}
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Relocate, Path: "repos/be", From: src}}
	res, err := Apply(plans, Options{Workspace: ws, Owned: owned, RepoByName: map[string]manifest.Repo{}}, &fakeGH{}, &fakeGit{})
	if err != nil || res[0].Err != nil {
		t.Fatalf("apply: %v / %v", err, res[0].Err)
	}
	to := filepath.Join(ws, "repos", "be")
	if _, err := os.Stat(filepath.Join(to, ".git")); err != nil {
		t.Fatalf("not moved: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(to, "f.txt")); string(b) != "v1\ndirty\n" {
		t.Fatalf("dirty change lost: %q", b)
	}
	fi, err := os.Lstat(src)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("old path is not a link: %v %v", fi, err)
	}
	if o, err := exec.Command("git", "-C", src, "status", "--short").CombinedOutput(); err != nil {
		t.Fatalf("git through old path broken: %v\n%s", err, o)
	}
	if e, ok := owned.Get(src); !ok || e.Link != to {
		t.Fatalf("link entry missing: %+v %v", e, ok)
	}
	if _, err := os.Stat(filepath.Join(to, ".claude", "settings.local.json")); err != nil {
		t.Fatalf("wire did not run after relocate: %v", err)
	}
	if !strings.HasPrefix(res[0].Action, "moved "+src+" → repos/be (đường dẫn cũ vẫn dùng được qua link)") { //znf:allow-lang
		t.Fatalf("action = %q", res[0].Action)
	}
	if target, err := os.Readlink(src); err != nil || !filepath.IsAbs(target) || target != to {
		t.Fatalf("link target = %q abs=%v err=%v, want %q", target, filepath.IsAbs(target), err, to)
	}
}

// TestApply_Relocate_WireFailureKeepsUserFiles verifies the controller ruling:
// RELOCATE runs (and its move is never rolled back) before the per-repo
// transaction opens, so a later wire/verify failure restores the user's own
// settings/exclude files that travelled with the repo instead of deleting them
// as "created this run".
func TestApply_Relocate_WireFailureKeepsUserFiles(t *testing.T) {
	ws := t.TempDir()
	src := filepath.Join(t.TempDir(), "projects", "be")
	if err := os.MkdirAll(filepath.Join(src, ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	gitInit(t, src)
	claudeDir := filepath.Join(src, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(`{"env":{"X":"1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".git", "info", "exclude"), []byte("mine/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{}
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Relocate, Path: "repos/be", From: src}}
	res, err := Apply(plans, Options{
		Workspace:    ws,
		Owned:        owned,
		RepoByName:   map[string]manifest.Repo{},
		SnapshotRoot: t.TempDir(),
		VerifyRepoFn: func(string, []string) error { return errors.New("boom") },
	}, &fakeGH{}, &fakeGit{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res[0].Err == nil {
		t.Fatal("expected verify failure to surface as Result.Err")
	}
	to := filepath.Join(ws, "repos", "be")
	if _, err := os.Stat(filepath.Join(to, ".git")); err != nil {
		t.Fatalf("relocate must not be undone: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(to, ".claude", "settings.local.json"))
	if err != nil || !strings.Contains(string(b), `"X":"1"`) {
		t.Fatalf("user's settings.local.json lost: %q err=%v", b, err)
	}
	eb, err := os.ReadFile(filepath.Join(to, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(eb), "mine/") {
		t.Fatalf("user's exclude lost: %q err=%v", eb, err)
	}
	fi, err := os.Lstat(src)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("old path is not a link: %v %v", fi, err)
	}
	if e, ok := owned.Get(src); !ok || e.Link != to {
		t.Fatalf("link entry must survive the wire revert: %+v %v", e, ok)
	}
}

// TestApply_Relocate_RelativeWorkspaceStillAbsoluteLink covers a relative
// --workspace (the "." default): the link left at the old path must still
// resolve to an absolute target, not one relative to the process cwd.
func TestApply_Relocate_RelativeWorkspaceStillAbsoluteLink(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	src := filepath.Join(t.TempDir(), "be")
	if err := os.MkdirAll(filepath.Join(src, ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Relocate, Path: "repos/be", From: src}}
	res, err := Apply(plans, Options{Workspace: "ws", Owned: &managed.Manifest{}, RepoByName: map[string]manifest.Repo{}}, &fakeGH{}, &fakeGit{})
	if err != nil || res[0].Err != nil {
		t.Fatalf("apply: %v / %v", err, res[0].Err)
	}
	target, err := os.Readlink(src)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(target) {
		t.Fatalf("link target must be absolute, got %q", target)
	}
	want := filepath.Join(root, "ws", "repos", "be")
	if target != want {
		t.Fatalf("link target = %q, want %q", target, want)
	}
}

func TestApply_Relocate_CrossDeviceIsFriendlyError(t *testing.T) {
	ws := t.TempDir()
	src := filepath.Join(t.TempDir(), "be")
	if err := os.MkdirAll(filepath.Join(src, ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Relocate, Path: "repos/be", From: src}}
	res, _ := Apply(plans, Options{
		Workspace: ws, Owned: &managed.Manifest{}, RepoByName: map[string]manifest.Repo{},
		RenameFn: func(from, to string) error {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV}
		},
	}, &fakeGH{}, &fakeGit{})
	if res[0].Err == nil || res[0].Err.Error() != "different volume — move by hand then re-run" {
		t.Fatalf("err = %v", res[0].Err)
	}
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		t.Fatal("source must be untouched")
	}
	if !errors.Is(&os.LinkError{Err: syscall.EXDEV}, syscall.EXDEV) {
		t.Fatal("sanity: LinkError must unwrap to EXDEV")
	}
}

func TestApply_Relocate_LinkFailureOnlyWarns(t *testing.T) {
	ws := t.TempDir()
	src := filepath.Join(t.TempDir(), "be")
	if err := os.MkdirAll(filepath.Join(src, ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{}
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Relocate, Path: "repos/be", From: src}}
	res, _ := Apply(plans, Options{
		Workspace: ws, Owned: owned, RepoByName: map[string]manifest.Repo{},
		LinkFn: func(string, string) error { return errors.New("junction denied") },
	}, &fakeGH{}, &fakeGit{})
	if res[0].Err != nil {
		t.Fatalf("link failure must not fail the repo: %v", res[0].Err)
	}
	if _, err := os.Stat(filepath.Join(ws, "repos", "be", ".git")); err != nil {
		t.Fatal("move must have happened")
	}
	if !strings.Contains(res[0].Action, "could not leave a link at") {
		t.Fatalf("action = %q, want warning about the failed link", res[0].Action)
	}
	if e, ok := owned.Get(src); ok {
		t.Fatalf("no link entry should be recorded when the link failed: %+v", e)
	}
}
