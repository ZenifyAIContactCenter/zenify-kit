package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo: git init THẬT (mkdir .git trơ khiến `git worktree list` lỗi "not a git
// repository", nên hasWT trả err → BuildPlan REFUSE oan).
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v: %s", dir, err, out)
	}
}

// commitAll add+commit mọi thứ để repo SẠCH (không thì file manifest untracked làm dirty
// → BuildPlan REFUSE). Dùng -c để không phụ thuộc git config toàn cục của máy chạy test.
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
	// repos.yaml sống BÊN TRONG repo (như thực tế), commit để repo sạch.
	man := filepath.Join(repo, "manifest")
	os.MkdirAll(man, 0o755)
	os.WriteFile(filepath.Join(man, "repos.yaml"),
		[]byte("repos:\n  - name: svc-a\n    path: svc-a\n"), 0o644)
	commitAll(t, repo)

	var out, errb bytes.Buffer
	// dry-run
	runMigrate(root, "repos", false, &out, &errb)
	if _, err := os.Stat(filepath.Join(root, "repos", "svc-a")); err == nil {
		t.Fatal("dry-run KHÔNG được move")
	}
	// apply
	out.Reset()
	runMigrate(root, "repos", true, &out, &errb)
	if _, err := os.Stat(filepath.Join(root, "repos", "svc-a", ".git")); err != nil {
		t.Fatalf("--apply phải move repo vào repos/: %v", err)
	}
	// repos.yaml đọc ở VỊ TRÍ MỚI (trong repo đã move).
	b, _ := os.ReadFile(filepath.Join(root, "repos", "svc-a", "manifest", "repos.yaml"))
	if !strings.Contains(string(b), "path: repos/svc-a") {
		t.Errorf("repos.yaml phải cập nhật path→repos/svc-a: %s", b)
	}
}

// TestMigrateManifestInsideRepo phản ánh THỰC TẾ: manifest/repos.yaml sống BÊN TRONG
// một repo con (kit), KHÔNG ở workspace root — và repo kit đó tự nó cũng bị move.
// Sau --apply: kit phải được move vào repos/, và repos.yaml (ở vị trí MỚI) phải có
// path: của MỌI repo đã move → repos/<name>. Test này FAIL trên code cũ (nó ghi vào
// <root>/manifest/repos.yaml không tồn tại) và PASS sau fix (resolve lười theo cấu trúc).
func TestMigrateManifestInsideRepo(t *testing.T) {
	root := t.TempDir()

	// kit là repo con SỞ HỮU manifest.
	kit := filepath.Join(root, "kit")
	initRepo(t, kit)
	man := filepath.Join(kit, "manifest")
	os.MkdirAll(man, 0o755)
	os.WriteFile(filepath.Join(man, "repos.yaml"),
		[]byte("repos:\n  - name: kit\n    path: kit\n  - name: svc-a\n    path: svc-a\n"), 0o644)
	commitAll(t, kit) // sạch → không bị REFUSE

	// một repo thường khác.
	initRepo(t, filepath.Join(root, "svc-a"))

	var out, errb bytes.Buffer
	runMigrate(root, "repos", true, &out, &errb)

	// repo kit (chủ manifest) phải được move vào repos/.
	if _, err := os.Stat(filepath.Join(root, "repos", "kit", ".git")); err != nil {
		t.Fatalf("--apply phải move repo kit vào repos/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "repos", "svc-a", ".git")); err != nil {
		t.Fatalf("--apply phải move repo svc-a vào repos/: %v", err)
	}
	// repos.yaml ở VỊ TRÍ MỚI phải cập nhật path cho mọi repo đã move.
	newMan := filepath.Join(root, "repos", "kit", "manifest", "repos.yaml")
	b, err := os.ReadFile(newMan)
	if err != nil {
		t.Fatalf("đọc repos.yaml ở vị trí mới: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "path: repos/kit") {
		t.Errorf("repos.yaml phải cập nhật path→repos/kit: %s", s)
	}
	if !strings.Contains(s, "path: repos/svc-a") {
		t.Errorf("repos.yaml phải cập nhật path→repos/svc-a: %s", s)
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

// TestMigrateApplyRealGitWorktreeAndSymlink là bằng chứng end-to-end (rule #3): move
// một repo có worktree nội bộ dirty + node_modules symlink (deps:symlink) qua git thật,
// xác nhận sau --apply repo/worktree/symlink/repos.yaml đều đúng.
func TestMigrateApplyRealGitWorktreeAndSymlink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("cần git")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "svc")
	runGit(t, "", "init", "-q", repo)
	runGit(t, repo, "config", "user.email", "t@t")
	runGit(t, repo, "config", "user.name", "t")
	writeFile(t, filepath.Join(repo, "f.txt"), "v1\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-qm", "init")
	// worktree nội bộ dưới .worktrees/
	runGit(t, repo, "worktree", "add", "-q", filepath.Join(repo, ".worktrees", "wt1"), "-b", "feat")
	// main dirty + việc dở trong worktree
	writeFile(t, filepath.Join(repo, "f.txt"), "v1\ndirty\n")
	writeFile(t, filepath.Join(repo, ".worktrees", "wt1", "g.txt"), "wtwork\n")
	// giả node_modules + worktree.json deps:symlink + symlink node_modules trong worktree
	mkdir(t, filepath.Join(repo, "node_modules"))
	mkdir(t, filepath.Join(repo, ".claude"))
	writeFile(t, filepath.Join(repo, ".claude", "worktree.json"), `{"deps":"symlink"}`)
	if err := os.Symlink(filepath.Join(repo, "node_modules"), filepath.Join(repo, ".worktrees", "wt1", "node_modules")); err != nil {
		t.Fatal(err)
	}
	// manifest để updateYAML có chỗ ghi (repo này tự chứa manifest)
	mkdir(t, filepath.Join(repo, "manifest"))
	writeFile(t, filepath.Join(repo, "manifest", "repos.yaml"),
		"repos:\n  - name: svc\n    path: svc\n")

	var out, errb bytes.Buffer
	if err := runMigrate(root, "repos", true, &out, &errb); err != nil {
		t.Fatalf("runMigrate trả lỗi (phải fail-open nil): %v", err)
	}

	newRepo := filepath.Join(root, "repos", "svc")
	newWT := filepath.Join(newRepo, ".worktrees", "wt1")

	// (1) repo đã ở repos/
	if _, err := os.Stat(filepath.Join(newRepo, ".git")); err != nil {
		t.Fatalf("repo chưa vào repos/: %v", err)
	}
	// (2) main dirty còn nguyên
	if b := readFile(t, filepath.Join(newRepo, "f.txt")); !strings.Contains(b, "dirty") {
		t.Fatalf("mất thay đổi dirty: %q", b)
	}
	// (3) worktree linkage sống sau repair
	if o, err := exec.Command("git", "-C", newWT, "status", "--short").CombinedOutput(); err != nil {
		t.Fatalf("worktree hỏng sau move: %v\n%s", err, o)
	}
	// (4) việc dở trong worktree còn
	if _, err := os.Stat(filepath.Join(newWT, "g.txt")); err != nil {
		t.Fatalf("mất việc dở worktree: %v", err)
	}
	// (5) symlink node_modules re-point sang main mới
	if tgt, err := os.Readlink(filepath.Join(newWT, "node_modules")); err != nil ||
		tgt != filepath.Join(newRepo, "node_modules") {
		t.Fatalf("symlink chưa re-point: tgt=%q err=%v", tgt, err)
	}
	// (6) repos.yaml cập nhật path
	if y := readFile(t, filepath.Join(newRepo, "manifest", "repos.yaml")); !strings.Contains(y, "path: repos/svc") {
		t.Fatalf("repos.yaml chưa cập nhật: %q", y)
	}
}
