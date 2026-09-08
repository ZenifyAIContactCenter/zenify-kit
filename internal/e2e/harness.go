// internal/e2e/harness.go
package e2e

import (
	"embed"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

//go:embed harness
var harnessFS embed.FS

// WriteHarness ghi cây harness e2e (fixtures/config/auth.setup/package.json) ra dir.
func WriteHarness(dir string) error { return pwdocker.WriteHarness(harnessFS, "harness", dir) }
