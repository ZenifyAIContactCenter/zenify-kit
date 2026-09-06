package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// mkGitRepoDir tạo <parent>/<name>/.git (thư mục) để workspace.Discover nhận là repo.
func mkGitRepoDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Regression sau `zenify migrate`: repo nằm ở <ws>/repos/<repo> → default phải trỏ
// vào đó, KHÔNG phải path phẳng <ws>/<repo> (bug làm config/release-report gãy default).
func TestResolveWorkspaceRepoDir_ReposLayout(t *testing.T) {
	ws := t.TempDir()
	repoDir := mkGitRepoDir(t, filepath.Join(ws, "repos"), "zenify-knowledge")

	got := resolveWorkspaceRepoDir(ws, "zenify-knowledge", "config", os.ReadDir)
	want := filepath.Join(repoDir, "config")
	if got != want {
		t.Fatalf("layout repos/: got %q, want %q", got, want)
	}
}

// Layout phẳng cũ (<ws>/<repo>) vẫn phải hoạt động (tương thích ngược).
func TestResolveWorkspaceRepoDir_FlatLayout(t *testing.T) {
	ws := t.TempDir()
	repoDir := mkGitRepoDir(t, ws, "zenify-knowledge")

	got := resolveWorkspaceRepoDir(ws, "zenify-knowledge", "releases", os.ReadDir)
	want := filepath.Join(repoDir, "releases")
	if got != want {
		t.Fatalf("layout phẳng: got %q, want %q", got, want)
	}
}

// Không tìm thấy repo → fallback path phẳng <ws>/<repo>/sub (fail-open: caller tự xử
// path không tồn tại). KHÔNG panic, KHÔNG trả rỗng.
func TestResolveWorkspaceRepoDir_FallbackWhenAbsent(t *testing.T) {
	ws := t.TempDir()

	got := resolveWorkspaceRepoDir(ws, "zenify-knowledge", "config", os.ReadDir)
	want := filepath.Join(ws, "zenify-knowledge", "config")
	if got != want {
		t.Fatalf("fallback: got %q, want %q", got, want)
	}
}
