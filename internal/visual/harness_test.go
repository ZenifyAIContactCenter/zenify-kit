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
	// PlaywrightVersion is "v1.64.1"; package.json uses the form without "v".
	want := strings.TrimPrefix(PlaywrightVersion, "v")
	if !strings.Contains(pkg, want) {
		t.Errorf("package.json must pin @playwright/test %s (lockstep image), got:\n%s", want, pkg)
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

func TestHarness_AuthSetupAssertsLeftLogin(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "auth.setup.ts")), "not.toHaveURL") {
		t.Fatal("auth.setup.ts must assert it left /login")
	}
}

// error-context.md of a failed login is an aria snapshot that prints input values.
func TestHarness_LoginFailureLeaksNoPassword(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	src := readFile(t, filepath.Join(dir, "auth.setup.ts"))
	// The snapshot is taken when the browser context closes, also after a timeout:
	// the page fixture's teardown (after use) empties the field before that close.
	use, clear, body := strings.Index(src, "await use(page);"), strings.Index(src, "set.call(el, '')"), strings.Index(src, "setup('authenticate'")
	if !strings.Contains(src, "base.extend(") || use < 0 || clear < use || clear > body {
		t.Error("auth.setup.ts must empty the password in the page fixture teardown")
	}
	// A failed fill() prints the value it was filling (measured on 1.64.1).
	if !strings.Contains(src, "(value withheld)") {
		t.Error("auth.setup.ts must rethrow a failed password fill without its value")
	}
	// process.env set in a test file reaches the journey workers too (measured on 1.64.1).
	if strings.Contains(src, "PLAYWRIGHT_NO_COPY_PROMPT") {
		t.Error("auth.setup.ts must not turn snapshots off by env: it leaks into journeys")
	}
}
