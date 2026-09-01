package managed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotAndRestore(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "conf.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapRoot := filepath.Join(dir, ".snap")

	snap, err := Snapshot("run-001", []string{target}, snapRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// mutate the live file
	if err := os.WriteFile(target, []byte("v2-broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(snap); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(target) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
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

func TestRestore_AllOrNothing_OnUnwritableTarget(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()

	// Two targets in two subdirs; one subdir will be made unwritable.
	okDir := filepath.Join(dir, "ok")
	badDir := filepath.Join(dir, "bad")
	for _, d := range []string{okDir, badDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	okFile := filepath.Join(okDir, "a.txt")
	badFile := filepath.Join(badDir, "b.txt")
	if err := os.WriteFile(okFile, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badFile, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	snapRoot := filepath.Join(dir, ".snap")
	snap, err := Snapshot("run-x", []string{okFile, badFile}, snapRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Mutate both live files, then make badDir unwritable so WriteFileAtomic fails there.
	if err := os.WriteFile(okFile, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badFile, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o500); err != nil { //nolint:gosec // G302 -- intentionally unwritable test directory (owner r-x, needed to enter/list); this is what the test exercises
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }() //nolint:gosec // G302 -- restoring a test directory to a normal, non-secret permission so t.TempDir cleanup can traverse and remove it

	if err := Restore(snap); err == nil {
		t.Fatal("expected Restore to fail on unwritable target")
	}
	// All-or-nothing: the writable file must NOT be left in the restored ("v1") state.
	got, _ := os.ReadFile(okFile) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if string(got) != "v2" {
		t.Errorf("okFile = %q, want v2 (rolled back to pre-restore state)", got)
	}
}

func TestRestore_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "conf.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := Snapshot("run-y", []string{target}, snapRoot(dir))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := os.WriteFile(target, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(snap); err != nil {
		t.Fatalf("restore: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".zenify-restore-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestRestore_PreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "conf.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Give the destination a non-default mode that differs from both 0o644
	// and the 0o600 that os.CreateTemp would use, so a silent narrowing is caught.
	if err := os.Chmod(target, 0o640); err != nil { //nolint:gosec // G302 -- deliberately non-0600 test fixture: this test asserts Restore PRESERVES an arbitrary existing mode rather than narrowing it, so the value must differ from 0600
		t.Fatal(err)
	}
	snap, err := Snapshot("run-z", []string{target}, snapRoot(dir))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Mutate the live file's content; os.WriteFile does not change the mode
	// of an existing file, so the destination's mode stays 0o640 going into Restore.
	if err := os.WriteFile(target, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(snap); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(target) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if string(got) != "v1" {
		t.Errorf("after restore content = %q, want v1", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("after restore mode = %o, want 0640 (preserved, not narrowed to temp-file default 0600)", info.Mode().Perm())
	}
}

func snapRoot(dir string) string { return filepath.Join(dir, ".snap") }
