package gitguard

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDeny tạo <dir>/.claude/deploy-branches với nội dung cho trước.
func writeDeny(t *testing.T, dir, content string) {
	t.Helper()
	cd := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(cd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cd, "deploy-branches"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasBranch(list []string, b string) bool {
	for _, x := range list {
		if x == b {
			return true
		}
	}
	return false
}

// NONE ở cấp repo → deny rỗng, KHÔNG union ancestor (dù ancestor có 'main').
func TestLoadDeny_NoneExempts(t *testing.T) {
	ws := t.TempDir()
	writeDeny(t, ws, "main\nstaging")
	repo := filepath.Join(ws, "docs")
	writeDeny(t, repo, "NONE")
	got := loadDeny(repo)
	if len(got) != 0 {
		t.Fatalf("NONE phải cho deny rỗng, got %v", got)
	}
}

// Control: repo KHÔNG có file, ancestor có 'main' → vẫn union 'main' (giữ cũ).
func TestLoadDeny_UnionUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeDeny(t, ws, "main")
	repo := filepath.Join(ws, "code-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	got := loadDeny(repo)
	if !hasBranch(got, "main") {
		t.Fatalf("union ancestor phải giữ 'main', got %v", got)
	}
}

// File rỗng (chỉ comment) KHÔNG exempt — vẫn union ancestor (fail-safe).
func TestLoadDeny_EmptyFileStillUnions(t *testing.T) {
	ws := t.TempDir()
	writeDeny(t, ws, "main")
	repo := filepath.Join(ws, "docs")
	writeDeny(t, repo, "# chỉ comment, không có NONE")
	got := loadDeny(repo)
	if !hasBranch(got, "main") {
		t.Fatalf("file rỗng KHÔNG được exempt (phải giữ 'main'), got %v", got)
	}
}
