package apply

import (
	"os"
	"path/filepath"
	"testing"
)

// codexSkillsFixture lays out a workspace link and a marketplace plugin skills
// dir, the targets ensureWorkspace passes in.
func codexSkillsFixture(t *testing.T) (ws, link, target string) {
	t.Helper()
	ws = t.TempDir()
	target = filepath.Join(t.TempDir(), "plugin-marketplace", "plugins", "znf", "skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	return ws, filepath.Join(ws, ".agents", "skills", "znf"), target
}

func TestEnsureCodexSkillsLink_CreatedThenUnchanged(t *testing.T) {
	_, link, target := codexSkillsFixture(t)
	st, err := EnsureCodexSkillsLink(link, target, false)
	if err != nil || st != LinkCreated {
		t.Fatalf("first: st=%v err=%v", st, err)
	}
	if got, _ := os.Readlink(link); got != target {
		t.Fatalf("readlink=%q want %q", got, target)
	}
	if st, err = EnsureCodexSkillsLink(link, target, false); err != nil || st != LinkUnchanged {
		t.Fatalf("second: st=%v err=%v", st, err)
	}
}

func TestEnsureCodexSkillsLink_RewritesWrongTarget(t *testing.T) {
	_, link, target := codexSkillsFixture(t)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/elsewhere", link); err != nil {
		t.Fatal(err)
	}
	st, err := EnsureCodexSkillsLink(link, target, false)
	if err != nil || st != LinkRewritten {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if got, _ := os.Readlink(link); got != target {
		t.Fatalf("readlink=%q want %q", got, target)
	}
}

func TestEnsureCodexSkillsLink_RealDirUntouched(t *testing.T) {
	_, link, target := codexSkillsFixture(t)
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := EnsureCodexSkillsLink(link, target, false)
	if err != nil || st != LinkRealDir {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if fi, _ := os.Lstat(link); fi == nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("real dir must stay a real dir")
	}
}

func TestEnsureCodexSkillsLink_DryRunCreatesNothing(t *testing.T) {
	ws, link, target := codexSkillsFixture(t)
	st, err := EnsureCodexSkillsLink(link, target, true)
	if err != nil || st != LinkCreated {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if _, err := os.Lstat(filepath.Join(ws, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created .agents: %v", err)
	}
}

func TestRemoveCodexSkillsLink(t *testing.T) {
	_, link, target := codexSkillsFixture(t)
	if _, err := EnsureCodexSkillsLink(link, target, false); err != nil {
		t.Fatal(err)
	}
	if ok, err := RemoveCodexSkillsLink(link, true); err != nil || !ok {
		t.Fatalf("dry: ok=%v err=%v", ok, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("dry-run must not remove")
	}
	if ok, err := RemoveCodexSkillsLink(link, false); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("link should be gone")
	}
	// wrong target is kept
	if err := os.Symlink("/elsewhere", link); err != nil {
		t.Fatal(err)
	}
	if ok, err := RemoveCodexSkillsLink(link, false); err != nil || ok {
		t.Fatalf("wrong target: ok=%v err=%v", ok, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("foreign symlink must stay")
	}
}

// A link made by an earlier kit points at <home>/.claude/skills/znf; down still
// removes it.
func TestRemoveCodexSkillsLink_LegacyTarget(t *testing.T) {
	home := t.TempDir()
	link := filepath.Join(home, ".agents", "skills", "znf")
	if _, err := EnsureCodexSkillsLink(link, filepath.Join(home, ".claude", "skills", "znf"), false); err != nil {
		t.Fatal(err)
	}
	if ok, err := RemoveCodexSkillsLink(link, false); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasUserKitHooks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if ok, err := HasUserKitHooks(p); err != nil || ok {
		t.Fatalf("missing file: ok=%v err=%v", ok, err)
	}
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-own-hook"}]}]}}`)
	if ok, err := HasUserKitHooks(p); err != nil || ok {
		t.Fatalf("foreign hook only: ok=%v err=%v", ok, err)
	}
	write(`{"autoMemoryDirectory":"x","hooks":{"Stop":[{"hooks":[{"type":"command","command":"zenify hooks-run docs-sync"}]}]}}`)
	if ok, err := HasUserKitHooks(p); err != nil || !ok {
		t.Fatalf("kit hook: ok=%v err=%v", ok, err)
	}
	write(`{not json`)
	if _, err := HasUserKitHooks(p); err == nil {
		t.Fatal("malformed file must return an error")
	}
}
