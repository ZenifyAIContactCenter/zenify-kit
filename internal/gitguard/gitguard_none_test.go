package gitguard

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDeny creates <dir>/.claude/deploy-branches with the given content.
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

// NONE at the repo level → empty deny, does NOT union ancestors (even if an ancestor has 'main').
func TestLoadDeny_NoneExempts(t *testing.T) {
	ws := t.TempDir()
	writeDeny(t, ws, "main\nstaging")
	repo := filepath.Join(ws, "docs")
	writeDeny(t, repo, "NONE")
	got := loadDeny(repo)
	if len(got) != 0 {
		t.Fatalf("NONE must produce empty deny, got %v", got)
	}
}

// Control: repo has NO file, ancestor has 'main' → still unions 'main' (unchanged).
func TestLoadDeny_UnionUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeDeny(t, ws, "main")
	repo := filepath.Join(ws, "code-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	got := loadDeny(repo)
	if !hasBranch(got, "main") {
		t.Fatalf("union with ancestor must keep 'main', got %v", got)
	}
}

// An empty file (comment only) does NOT exempt — still unions the ancestor (fail-safe).
func TestLoadDeny_EmptyFileStillUnions(t *testing.T) {
	ws := t.TempDir()
	writeDeny(t, ws, "main")
	repo := filepath.Join(ws, "docs")
	writeDeny(t, repo, "# comment only, no NONE")
	got := loadDeny(repo)
	if !hasBranch(got, "main") {
		t.Fatalf("empty file must NOT be exempt (must keep 'main'), got %v", got)
	}
}
