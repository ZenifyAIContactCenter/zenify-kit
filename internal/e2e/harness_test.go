// internal/e2e/harness_test.go
package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHarness_MaterializesFixtures(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"fixtures.ts", "playwright.config.ts", "package.json", "auth.setup.ts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s must exist: %v", f, err)
		}
	}
}

func TestHarness_AuthSetupAssertsLeftLogin(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "auth.setup.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "not.toHaveURL") {
		t.Fatal("auth.setup.ts must assert it left /login")
	}
}

// A failed login leaves error-context.md (an aria snapshot that prints input values)
// and, with tracing on, a trace that records fill() text: neither may carry the password.
func TestHarness_LoginFailureLeaksNoPassword(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	setup, _ := os.ReadFile(filepath.Join(dir, "auth.setup.ts"))
	src := string(setup)
	// The snapshot is taken when expect fails, so the field is emptied before it.
	clear, assert := strings.Index(src, "password.fill('', "), strings.Index(src, "not.toHaveURL")
	if clear < 0 || clear > assert {
		t.Error("auth.setup.ts must empty the password field before asserting it left /login")
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "playwright.config.ts"))
	if !strings.Contains(string(cfg), `name: 'setup', testDir: '.', testMatch: /auth\.setup\.ts/, use: { trace: 'off' }`) {
		t.Error("the setup project must run with trace off")
	}
}
