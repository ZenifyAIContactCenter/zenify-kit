package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateDryRunThenApply(t *testing.T) {
	root := t.TempDir()
	// repo sạch — git init THẬT (mkdir .git trơ khiến `git worktree list` lỗi
	// "not a git repository", nên hasWT trả err → BuildPlan REFUSE oan).
	repo := filepath.Join(root, "svc-a")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// repos.yaml tối thiểu
	man := filepath.Join(root, "manifest")
	os.MkdirAll(man, 0o755)
	os.WriteFile(filepath.Join(man, "repos.yaml"),
		[]byte("repos:\n  - name: svc-a\n    path: svc-a\n"), 0o644)

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
	b, _ := os.ReadFile(filepath.Join(man, "repos.yaml"))
	if !strings.Contains(string(b), "path: repos/svc-a") {
		t.Errorf("repos.yaml phải cập nhật path→repos/svc-a: %s", b)
	}
}
