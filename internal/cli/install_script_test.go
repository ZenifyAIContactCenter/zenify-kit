package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallSh_PathLineIdempotentAndFooter runs scripts/install.sh with a
// fake release: ZENIFY_INSTALL_FROM points at a tarball we build here, so no
// network. SC-8: one PATH line after two runs, and the last two stdout lines.
func TestInstallSh_PathLineIdempotentAndFooter(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("requires sh")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("requires tar")
	}
	home := t.TempDir()
	fake := filepath.Join(t.TempDir(), "zenify")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncase \"$1\" in version) echo 'zenify v9.9.9';; skills) exit 0;; esac\n"), 0o755); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	tar := filepath.Join(t.TempDir(), "zenify_test.tar.gz")
	if o, err := exec.Command("tar", "-czf", tar, "-C", filepath.Dir(fake), "zenify").CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, o)
	}
	dest := filepath.Join(home, ".local", "bin")
	run := func() string {
		cmd := exec.Command("sh", filepath.Join("..", "..", "scripts", "install.sh"))
		cmd.Env = []string{"HOME=" + home, "SHELL=/bin/zsh", "PATH=/usr/bin:/bin", "ZENIFY_BIN=" + dest, "ZENIFY_INSTALL_FROM=" + tar}
		o, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("install.sh: %v\n%s", err, o)
		}
		return string(o)
	}
	out := run()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 || lines[len(lines)-2] != "Installed zenify v9.9.9" || lines[len(lines)-1] != "Next: zenify up" {
		t.Fatalf("footer wrong:\n%s", out)
	}
	run()
	rc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	want := `export PATH='` + dest + `':"$PATH" # zenify-kit`
	if strings.Count(string(rc), want) != 1 {
		t.Fatalf(".zshrc after two runs:\n%s", rc)
	}
}

// F4: a ZENIFY_BIN containing a single quote must be refused before any
// install step runs, since the profile PATH line single-quotes it verbatim.
func TestInstallSh_RefusesQuoteInZenifyBin(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("requires sh")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("requires tar")
	}
	home := t.TempDir()
	fake := filepath.Join(t.TempDir(), "zenify")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'zenify v9.9.9'\n"), 0o755); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	tar := filepath.Join(t.TempDir(), "zenify_test.tar.gz")
	if o, err := exec.Command("tar", "-czf", tar, "-C", filepath.Dir(fake), "zenify").CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, o)
	}
	dest := filepath.Join(home, "a'b")
	cmd := exec.Command("sh", filepath.Join("..", "..", "scripts", "install.sh"))
	cmd.Env = []string{"HOME=" + home, "SHELL=/bin/zsh", "PATH=/usr/bin:/bin", "ZENIFY_BIN=" + dest, "ZENIFY_INSTALL_FROM=" + tar}
	o, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install.sh should have refused a quoted ZENIFY_BIN, got:\n%s", o)
	}
	if !strings.Contains(string(o), "must not contain quotes or newlines") {
		t.Fatalf("unexpected error output:\n%s", o)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("install.sh must not have created %s before refusing", dest)
	}
}
