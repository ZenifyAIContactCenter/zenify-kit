package pwdocker

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata
var testFS embed.FS

func TestBaseArgs_LinuxAddsHost(t *testing.T) {
	got := strings.Join(BaseArgs("linux"), " ")
	if !strings.Contains(got, "--add-host=host.docker.internal:host-gateway") {
		t.Fatalf("linux must have --add-host, got %q", got)
	}
}

func TestBaseArgs_DarwinNoAddHost(t *testing.T) {
	got := strings.Join(BaseArgs("darwin"), " ")
	if strings.Contains(got, "--add-host") {
		t.Fatalf("darwin must NOT have --add-host, got %q", got)
	}
}

func TestImage_UsesVersion(t *testing.T) {
	if !strings.Contains(Image(), PlaywrightVersion) {
		t.Fatalf("Image %q must contain version %q", Image(), PlaywrightVersion)
	}
}

func TestWriteHarness_Flattens(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(testFS, "testdata", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker.txt")); err != nil {
		t.Fatalf("marker.txt must exist flattened: %v", err)
	}
}
