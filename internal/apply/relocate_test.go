package apply

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	if res[0].Action != "moved "+src+" → repos/be (đường dẫn cũ vẫn dùng được qua link)" {
		t.Fatalf("action = %q", res[0].Action)
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
		RenameFn: func(from, to string) error { return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV} },
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
	plans := []reconcile.RepoPlan{{Name: "be", State: reconcile.Relocate, Path: "repos/be", From: src}}
	res, _ := Apply(plans, Options{
		Workspace: ws, Owned: &managed.Manifest{}, RepoByName: map[string]manifest.Repo{},
		LinkFn: func(string, string) error { return errors.New("junction denied") },
	}, &fakeGH{}, &fakeGit{})
	if res[0].Err != nil {
		t.Fatalf("link failure must not fail the repo: %v", res[0].Err)
	}
	if _, err := os.Stat(filepath.Join(ws, "repos", "be", ".git")); err != nil {
		t.Fatal("move must have happened")
	}
}
