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
