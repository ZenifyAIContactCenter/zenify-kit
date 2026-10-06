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
