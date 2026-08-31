package managed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotAndRestore(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "conf.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapRoot := filepath.Join(dir, ".snap")

	snap, err := Snapshot("run-001", []string{target}, snapRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// mutate the live file
	if err := os.WriteFile(target, []byte("v2-broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restore(snap); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "v1" {
		t.Errorf("after restore = %q, want v1", got)
	}
}

func TestSnapshot_SkipsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	snapRoot := filepath.Join(dir, ".snap")
	// file does not exist -> must not error
	if _, err := Snapshot("run-002", []string{filepath.Join(dir, "nope.txt")}, snapRoot); err != nil {
		t.Errorf("snapshot of missing file errored: %v", err)
	}
}
