package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// Fixture: three files under dest. "a" is recorded and untouched → removed;
// "b" is recorded but edited after install → kept; "c" is not recorded → never touched.
func setupPruneFixture(t *testing.T) (dest, man string) {
	t.Helper()
	dest = t.TempDir()
	man = filepath.Join(dest, ".manifest.json")
	m := &managed.Manifest{Entries: map[string]managed.Entry{}}
	for _, name := range []string{"a", "b", "c"} {
		p := filepath.Join(dest, name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("body "+name), 0o600); err != nil {
			t.Fatal(err)
		}
		if name != "c" {
			if err := m.Record(p); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dest, "b", "SKILL.md"), []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(man); err != nil {
		t.Fatal(err)
	}
	return dest, man
}

func TestPruneCoding_RemovesRecordedKeepsModifiedIgnoresUnrecorded(t *testing.T) {
	dest, man := setupPruneFixture(t)
	res, err := PruneCoding(dest, man)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "a")); !os.IsNotExist(err) {
		t.Errorf("a/ must be removed (dir cleaned), err=%v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "b", "SKILL.md")); err != nil || string(b) != "edited" {
		t.Errorf("b/SKILL.md (user-modified) must be kept: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(dest, "c", "SKILL.md")); err != nil {
		t.Errorf("c/SKILL.md (unrecorded) must be untouched: %v", err)
	}
	if len(res.Removed) != 1 || len(res.Kept) != 1 {
		t.Errorf("Removed=%v Kept=%v; want 1 and 1", res.Removed, res.Kept)
	}
	m, err := managed.Load(man)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get(filepath.Join(dest, "a", "SKILL.md")); ok {
		t.Error("manifest must forget a/SKILL.md")
	}
	if _, ok := m.Get(filepath.Join(dest, "b", "SKILL.md")); !ok {
		t.Error("manifest must still record b/SKILL.md (kept)")
	}
}

func TestPruneCoding_DeletesEmptyManifest(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	p := filepath.Join(dest, "x", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &managed.Manifest{Entries: map[string]managed.Entry{}}
	if err := m.Record(p); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(man); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneCoding(dest, man); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(man); !os.IsNotExist(err) {
		t.Errorf("empty manifest must be deleted, err=%v", err)
	}
	// second run: nothing to do, no error
	res, err := PruneCoding(dest, man)
	if err != nil || len(res.Removed) != 0 {
		t.Errorf("second run: err=%v Removed=%v", err, res.Removed)
	}
}

func TestPruneCoding_NeverLeavesDest(t *testing.T) {
	dest := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("o"), 0o600); err != nil {
		t.Fatal(err)
	}
	man := filepath.Join(dest, ".manifest.json")
	m := &managed.Manifest{Entries: map[string]managed.Entry{}}
	if err := m.Record(outside); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(man); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneCoding(dest, man); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("file outside dest must never be removed: %v", err)
	}
	if _, err := os.Stat(man); err != nil {
		t.Errorf("manifest with an inert outside entry must be kept: %v", err)
	}
}
