package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo: a REAL git init (a bare `mkdir .git` makes `git worktree list` error "not a git
// repository", so hasWT returns err → BuildPlan wrongly REFUSEs).
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v: %s", dir, err, out)
	}
}

// commitAll adds+commits everything so the repo is CLEAN (otherwise the untracked manifest
// file makes it dirty → BuildPlan REFUSEs). Uses -c so it doesn't depend on the test
// machine's global git config.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add %s: %v: %s", dir, err, out)
	}
	cmd := exec.Command("git",
		"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false",
		"-C", dir, "commit", "-m", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %s: %v: %s", dir, err, out)
	}
}

func TestMigrateDryRunThenApply(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "svc-a")
	initRepo(t, repo)
	// repos.yaml lives INSIDE the repo (as it really does), commit so the repo is clean.
	man := filepath.Join(repo, "manifest")
	os.MkdirAll(man, 0o755)
	os.WriteFile(filepath.Join(man, "repos.yaml"),
		[]byte("repos:\n  - name: svc-a\n    path: svc-a\n"), 0o644)
	commitAll(t, repo)

	var out, errb bytes.Buffer
	// dry-run
	runMigrate(root, "repos", false, &out, &errb)
	if _, err := os.Stat(filepath.Join(root, "repos", "svc-a")); err == nil {
		t.Fatal("dry-run must NOT move")
	}
	// apply
	out.Reset()
	runMigrate(root, "repos", true, &out, &errb)
	if _, err := os.Stat(filepath.Join(root, "repos", "svc-a", ".git")); err != nil {
		t.Fatalf("--apply must move the repo into repos/: %v", err)
	}
	// repos.yaml is read at its NEW location (inside the moved repo).
	b, _ := os.ReadFile(filepath.Join(root, "repos", "svc-a", "manifest", "repos.yaml"))
	if !strings.Contains(string(b), "path: repos/svc-a") {
		t.Errorf("repos.yaml must update path→repos/svc-a: %s", b)
	}
}

// TestMigrateManifestInsideRepo reflects REALITY: manifest/repos.yaml lives INSIDE a
// sub-repo (kit), NOT at the workspace root — and that kit repo itself also gets moved.
// After --apply: kit must be moved into repos/, and repos.yaml (at its NEW location) must
// have path: → repos/<name> for EVERY moved repo. This test FAILs on the old code (it
// writes to <root>/manifest/repos.yaml, which doesn't exist) and PASSes after the fix
// (structural lazy resolve).
func TestMigrateManifestInsideRepo(t *testing.T) {
	root := t.TempDir()

	// kit is the sub-repo that OWNS the manifest.
	kit := filepath.Join(root, "kit")
	initRepo(t, kit)
	man := filepath.Join(kit, "manifest")
	os.MkdirAll(man, 0o755)
	os.WriteFile(filepath.Join(man, "repos.yaml"),
		[]byte("repos:\n  - name: kit\n    path: kit\n  - name: svc-a\n    path: svc-a\n"), 0o644)
	commitAll(t, kit) // clean → not REFUSEd

	// another ordinary repo.
	initRepo(t, filepath.Join(root, "svc-a"))

	var out, errb bytes.Buffer
	runMigrate(root, "repos", true, &out, &errb)

	// the kit repo (manifest owner) must be moved into repos/.
	if _, err := os.Stat(filepath.Join(root, "repos", "kit", ".git")); err != nil {
		t.Fatalf("--apply must move the kit repo into repos/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "repos", "svc-a", ".git")); err != nil {
		t.Fatalf("--apply must move the svc-a repo into repos/: %v", err)
	}
	// repos.yaml at its NEW location must update path for every moved repo.
	newMan := filepath.Join(root, "repos", "kit", "manifest", "repos.yaml")
	b, err := os.ReadFile(newMan)
	if err != nil {
		t.Fatalf("read repos.yaml at its new location: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "path: repos/kit") {
		t.Errorf("repos.yaml must update path→repos/kit: %s", s)
	}
	if !strings.Contains(s, "path: repos/svc-a") {
		t.Errorf("repos.yaml must update path→repos/svc-a: %s", s)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	if dir != "" {
		c.Dir = dir
	}
	if o, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, o)
	}
}
func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestMigrateApplyRealGitWorktreeAndSymlink is the end-to-end proof (rule #3): moves a
// repo with a dirty internal worktree + node_modules symlink (deps:symlink) through real
// git, confirms that after --apply the repo/worktree/symlink/repos.yaml are all correct.
func TestMigrateApplyRealGitWorktreeAndSymlink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("requires git")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "svc")
	runGit(t, "", "init", "-q", repo)
	runGit(t, repo, "config", "user.email", "t@t")
	runGit(t, repo, "config", "user.name", "t")
	writeFile(t, filepath.Join(repo, "f.txt"), "v1\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-qm", "init")
	// internal worktree under .worktrees/
	runGit(t, repo, "worktree", "add", "-q", filepath.Join(repo, ".worktrees", "wt1"), "-b", "feat")
	// dirty main + work-in-progress in the worktree
	writeFile(t, filepath.Join(repo, "f.txt"), "v1\ndirty\n")
	writeFile(t, filepath.Join(repo, ".worktrees", "wt1", "g.txt"), "wtwork\n")
	// fake node_modules + worktree.json deps:symlink + node_modules symlink in the worktree
	mkdir(t, filepath.Join(repo, "node_modules"))
	mkdir(t, filepath.Join(repo, ".claude"))
	writeFile(t, filepath.Join(repo, ".claude", "worktree.json"), `{"deps":"symlink"}`)
	if err := os.Symlink(filepath.Join(repo, "node_modules"), filepath.Join(repo, ".worktrees", "wt1", "node_modules")); err != nil {
		t.Fatal(err)
	}
	// manifest so updateYAML has somewhere to write (this repo contains its own manifest)
	mkdir(t, filepath.Join(repo, "manifest"))
	writeFile(t, filepath.Join(repo, "manifest", "repos.yaml"),
		"repos:\n  - name: svc\n    path: svc\n")

	var out, errb bytes.Buffer
	if err := runMigrate(root, "repos", true, &out, &errb); err != nil {
		t.Fatalf("runMigrate returned an error (should fail-open nil): %v", err)
	}

	newRepo := filepath.Join(root, "repos", "svc")
	newWT := filepath.Join(newRepo, ".worktrees", "wt1")

	// (1) repo is now under repos/
	if _, err := os.Stat(filepath.Join(newRepo, ".git")); err != nil {
		t.Fatalf("repo not moved into repos/: %v", err)
	}
	// (2) dirty main change survived
	if b := readFile(t, filepath.Join(newRepo, "f.txt")); !strings.Contains(b, "dirty") {
		t.Fatalf("lost dirty change: %q", b)
	}
	// (3) worktree linkage survives the repair
	if o, err := exec.Command("git", "-C", newWT, "status", "--short").CombinedOutput(); err != nil {
		t.Fatalf("worktree broken after move: %v\n%s", err, o)
	}
	// (4) work-in-progress in the worktree survived
	if _, err := os.Stat(filepath.Join(newWT, "g.txt")); err != nil {
		t.Fatalf("lost worktree work-in-progress: %v", err)
	}
	// (5) node_modules symlink re-points to the new main
	if tgt, err := os.Readlink(filepath.Join(newWT, "node_modules")); err != nil ||
		tgt != filepath.Join(newRepo, "node_modules") {
		t.Fatalf("symlink not re-pointed: tgt=%q err=%v", tgt, err)
	}
	// (6) repos.yaml updated the path
	if y := readFile(t, filepath.Join(newRepo, "manifest", "repos.yaml")); !strings.Contains(y, "path: repos/svc") {
		t.Fatalf("repos.yaml not updated: %q", y)
	}
}
