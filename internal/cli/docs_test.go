package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
)

// docs sync resolve dir repo qua discovery (repos/<repo> hoặc phẳng).
func TestDocsDirResolves(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "docs")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveWorkspaceRepoDir(ws, defaultDocsRepo, "", os.ReadDir)
	if filepath.Clean(got) != filepath.Clean(repo) {
		t.Fatalf("resolve docs dir: got %q want %q", got, repo)
	}
}

// --dir override phải thắng: resolveDocsStore không được tư vấn, và store
// thật sự dùng chính là --dir (không phải path resolveDocsStore sẽ trả), thể
// hiện qua link farm được tạo dưới workspace/docs → --dir/specs.
func TestDocsSyncDirFlagWins(t *testing.T) {
	ws := t.TempDir()
	store := t.TempDir() // KHÔNG phải là git repo, KHÔNG nằm dưới ws — khác hẳn nơi resolveDocsStore sẽ trỏ tới
	if err := os.MkdirAll(filepath.Join(store, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newDocsCmd()
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"sync", "--workspace", ws, "--dir", store})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// viewDir = ws/docs, phải khác store nên EnsureView chạy và link tới store/specs.
	viewDir := filepath.Join(ws, defaultDocsRepo)
	if _, err := os.Stat(filepath.Join(viewDir, "specs")); err != nil {
		t.Fatalf("view farm không tạo tại %s: %v (--dir có thể đã KHÔNG thắng)", viewDir, err)
	}
	same, err := (docsview.OSFS{}).SameTarget(filepath.Join(viewDir, "specs"), filepath.Join(store, "specs"))
	if err != nil || !same {
		t.Fatalf("view/specs không trỏ về --dir/specs: same=%v err=%v", same, err)
	}
}

// EnsureView phải KHÔNG chạy khi viewDir == dir (chưa migrate: docs store vẫn
// nằm ngay trong workspace) — không có note "docs view:" nào được in ra.
func TestDocsSyncEnsureViewSkippedWhenViewEqualsDir(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, defaultDocsRepo) // == viewDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newDocsCmd()
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"sync", "--workspace", ws, "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if strings.Contains(errBuf.String(), "docs view:") {
		t.Fatalf("EnsureView phải bị skip khi viewDir==dir, nhưng có note: %s", errBuf.String())
	}
}
