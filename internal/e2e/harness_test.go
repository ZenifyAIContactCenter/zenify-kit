// internal/e2e/harness_test.go
package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteHarness_MaterializesFixtures(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHarness(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"fixtures.ts", "playwright.config.ts", "package.json", "auth.setup.ts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s phải tồn tại: %v", f, err)
		}
	}
}
