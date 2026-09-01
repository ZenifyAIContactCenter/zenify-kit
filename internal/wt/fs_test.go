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

func TestSeedCopyFiles_DirectoryTargetNotNested(t *testing.T) {
	// A directory copy target must land FLAT at dst, not nested as
	// dst/<basename>/... — the cp-into-existing-dir gotcha copyTree guards.
	repo := t.TempDir()
	wtp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".claude", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "sub", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedCopyFiles(repo, wtp, []string{".claude"}); err != nil {
		t.Fatal(err)
	}
	// Flat: the file is at wtp/.claude/sub/f.txt, NOT wtp/.claude/.claude/...
	if _, err := os.Stat(filepath.Join(wtp, ".claude", "sub", "f.txt")); err != nil {
		t.Fatalf("directory target not copied flat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtp, ".claude", ".claude")); err == nil {
		t.Fatal("directory target nested one level too deep")
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

func TestSeedIdentityEnv_WritesSlugAndCompose(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	// pre-existing content to prove upsert appends without clobbering
	if err := os.WriteFile(env, []byte("PORT=3207\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Abbrev: "cch", EnvFile: ".env"} // no RedisPrefixEnv → no redis line
	if err := SeedIdentityEnv(env, cfg, "my-task"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	got := string(b)
	for _, want := range []string{"PORT=3207", "WT_SLUG=my-task", "COMPOSE_PROJECT_NAME=cch-my-task"} {
		if !strings.Contains(got, want) {
			t.Errorf("env missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "REDIS") {
		t.Errorf("no RedisPrefixEnv configured, so no redis line expected; got:\n%s", got)
	}
}

func TestSeedIdentityEnv_RedisPrefixWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	cfg := &Config{Abbrev: "cch", EnvFile: ".env", RedisPrefixEnv: "REDIS_PREFIX"}
	if err := SeedIdentityEnv(env, cfg, "my-task"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	if !strings.Contains(string(b), "REDIS_PREFIX=cch-my-task:") {
		t.Errorf("expected REDIS_PREFIX=cch-my-task: ; got:\n%s", string(b))
	}
}

func TestSeedIdentityEnv_EmptyAbbrevUsesSlugOnly(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	cfg := &Config{Abbrev: "", EnvFile: ".env"}
	if err := SeedIdentityEnv(env, cfg, "solo"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	if !strings.Contains(string(b), "COMPOSE_PROJECT_NAME=solo\n") {
		t.Errorf("empty abbrev should yield COMPOSE_PROJECT_NAME=solo; got:\n%s", string(b))
	}
}

func TestUpsertEnvVar_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("WT_SLUG=old\nOTHER=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := upsertEnvVar(env, "WT_SLUG", "new"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	got := string(b)
	if strings.Contains(got, "WT_SLUG=old") || !strings.Contains(got, "WT_SLUG=new") {
		t.Errorf("upsert should replace old→new; got:\n%s", got)
	}
	if !strings.Contains(got, "OTHER=x") {
		t.Errorf("upsert must not drop other keys; got:\n%s", got)
	}
}
