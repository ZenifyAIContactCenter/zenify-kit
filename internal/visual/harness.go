package visual

import (
	"embed"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

//go:embed harness
var harnessFS embed.FS

// WriteHarness keeps the old signature (harness_test.go in package visual calls it unqualified) — forwards.
func WriteHarness(dir string) error { return pwdocker.WriteHarness(harnessFS, "harness", dir) }
