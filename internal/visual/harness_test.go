package visual

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHarness_MaterializesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatalf("WriteHarness: %v", err)
	}
	for _, f := range []string{"playwright.config.ts", "visual.spec.ts", "auth.setup.ts", "package.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestHarness_DeterminismMarkers(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	spec := readFile(t, filepath.Join(dir, "visual.spec.ts"))
	for _, m := range []string{"toHaveScreenshot", "animations: 'disabled'", "caret: 'hide'", "document.fonts.ready", "routes.json"} {
		if !strings.Contains(spec, m) {
			t.Errorf("visual.spec.ts missing determinism/routing marker %q", m)
		}
	}
	setup := readFile(t, filepath.Join(dir, "auth.setup.ts"))
	if !strings.Contains(setup, "storageState") {
		t.Error("auth.setup.ts must write storageState")
	}
}

func TestHarness_VersionLockstep(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	pkg := readFile(t, filepath.Join(dir, "package.json"))
	// PlaywrightVersion là "v1.55.0"; package.json dùng dạng không "v".
	want := strings.TrimPrefix(PlaywrightVersion, "v")
	if !strings.Contains(pkg, want) {
		t.Errorf("package.json phải pin @playwright/test %s (lockstep image), got:\n%s", want, pkg)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}
