package docsview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFSEnsureViewIntegration(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	view := filepath.Join(root, "view")

	if err := os.MkdirAll(filepath.Join(store, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	specFile := filepath.Join(store, "specs", "a.md")
	if err := os.WriteFile(specFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	notes := EnsureView(OSFS{}, store, view)
	for _, n := range notes {
		t.Logf("note: %s", n)
	}

	// view/specs/a.md must be readable, resolving down to store.
	got, err := os.ReadFile(filepath.Join(view, "specs", "a.md"))
	if err != nil {
		t.Fatalf("read view/specs/a.md: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content mismatch: got %q", got)
	}

	// view/.config must NOT exist (only non-dot top-level dirs get linked).
	if _, err := os.Stat(filepath.Join(view, ".config")); !os.IsNotExist(err) {
		t.Fatalf("view/.config must not exist, got err=%v", err)
	}

	// SameTarget(view/specs, store/specs) == true.
	same, err := (OSFS{}).SameTarget(filepath.Join(view, "specs"), filepath.Join(store, "specs"))
	if err != nil {
		t.Fatalf("SameTarget err: %v", err)
	}
	if !same {
		t.Fatalf("SameTarget: expected true")
	}
}
