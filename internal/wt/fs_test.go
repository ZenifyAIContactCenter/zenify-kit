package wt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePortEnv_ReplaceAndAppend(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	// append into a file with no trailing newline
	if err := os.WriteFile(env, []byte("FOO=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePortEnv(env, "PORT", 3207); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	if !strings.Contains(string(b), "FOO=1\n") || !strings.Contains(string(b), "PORT=3207") {
		t.Fatalf("append wrong: %q", b)
	}
	// replace an existing PORT line, not append a second
	if err := WritePortEnv(env, "PORT", 3210); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(env)
	if strings.Count(string(b), "PORT=") != 1 || !strings.Contains(string(b), "PORT=3210") {
		t.Fatalf("replace wrong: %q", b)
	}
}

func TestWritePortEnv_CreatesFile(t *testing.T) {
	env := filepath.Join(t.TempDir(), ".env")
	if err := WritePortEnv(env, "PORT", 3300); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	if strings.TrimSpace(string(b)) != "PORT=3300" {
		t.Fatalf("create wrong: %q", b)
	}
}

func TestSeedCopyFiles_CopiesPresentWarnsMissing(t *testing.T) {
	repo := t.TempDir()
	wtp := t.TempDir()
	// present: a nested file
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "settings.local.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	warns, err := SeedCopyFiles(repo, wtp, []string{".claude/settings.local.json", "CLAUDE.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, e := os.Stat(filepath.Join(wtp, ".claude", "settings.local.json")); e != nil {
		t.Fatalf("present target not seeded: %v", e)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "CLAUDE.md") {
		t.Fatalf("missing target should warn once about CLAUDE.md, got %v", warns)
	}
}

func TestApplyDeps_Symlink(t *testing.T) {
	repo := t.TempDir()
	wtp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(filepath.Join(wtp, "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("node_modules should be a symlink in symlink mode")
	}
}

func TestApplyDeps_SymlinkMissingSourceErrors(t *testing.T) {
	if err := ApplyDeps(t.TempDir(), t.TempDir(), "symlink", "node_modules", ""); err == nil {
		t.Fatal("symlink with no source node_modules must error")
	}
}
