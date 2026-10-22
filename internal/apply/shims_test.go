package apply

import (
	"os"
	"path/filepath"
	"testing"
)

func shimPath(home, name string) string { return filepath.Join(home, ".local", "bin", name) }

func TestInstallShimsWritesBoth(t *testing.T) {
	home := t.TempDir()
	written, skipped, err := InstallShims(home, false)
	if err != nil || len(skipped) != 0 || len(written) != 2 {
		t.Fatalf("written=%v skipped=%v err=%v", written, skipped, err)
	}
	for _, n := range []string{"wt", "pm"} {
		b, err := os.ReadFile(shimPath(home, n))
		if err != nil {
			t.Fatal(err)
		}
		want := "#!/bin/sh\n# managed by zenify\nexec zenify " + n + " \"$@\"\n"
		if string(b) != want {
			t.Errorf("%s content = %q, want %q", n, b, want)
		}
		fi, _ := os.Stat(shimPath(home, n))
		if fi.Mode().Perm() != 0o755 {
			t.Errorf("%s mode = %v", n, fi.Mode().Perm())
		}
	}
	written, skipped, err = InstallShims(home, false)
	if err != nil || len(written) != 0 || len(skipped) != 0 {
		t.Fatalf("second run: written=%v skipped=%v err=%v", written, skipped, err)
	}
}

func TestInstallShimsKeepsUserFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(shimPath(home, "wt")), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "#!/bin/sh\necho mine\n"
	if err := os.WriteFile(shimPath(home, "wt"), []byte(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	written, skipped, err := InstallShims(home, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || skipped[0] != shimPath(home, "wt") {
		t.Errorf("skipped = %v", skipped)
	}
	if len(written) != 1 || written[0] != shimPath(home, "pm") {
		t.Errorf("written = %v", written)
	}
	if b, _ := os.ReadFile(shimPath(home, "wt")); string(b) != mine {
		t.Errorf("user file changed: %q", b)
	}
}

func TestRemoveShimsOnlyMarked(t *testing.T) {
	home := t.TempDir()
	if _, _, err := InstallShims(home, false); err != nil {
		t.Fatal(err)
	}
	mine := "#!/bin/sh\necho mine\n"
	if err := os.WriteFile(shimPath(home, "wt"), []byte(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveShims(home, false)
	if err != nil || len(removed) != 1 || removed[0] != shimPath(home, "pm") {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(shimPath(home, "pm")); !os.IsNotExist(err) {
		t.Errorf("pm still exists: %v", err)
	}
	if b, _ := os.ReadFile(shimPath(home, "wt")); string(b) != mine {
		t.Errorf("user file changed: %q", b)
	}
}

func TestShimsDryRunTouchesNothing(t *testing.T) {
	home := t.TempDir()
	written, _, err := InstallShims(home, true)
	if err != nil || len(written) != 2 {
		t.Fatalf("written=%v err=%v", written, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Errorf("dry-run created files: %v", err)
	}
	if _, _, err := InstallShims(home, false); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveShims(home, true)
	if err != nil || len(removed) != 2 {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	for _, n := range []string{"wt", "pm"} {
		if _, err := os.Stat(shimPath(home, n)); err != nil {
			t.Errorf("dry-run removed %s", n)
		}
	}
}
