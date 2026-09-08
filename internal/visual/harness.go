package visual

import (
	"embed"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

//go:embed harness
var harnessFS embed.FS

// WriteHarness giữ chữ ký cũ (harness_test.go package visual gọi không qualify) — forward.
func WriteHarness(dir string) error { return pwdocker.WriteHarness(harnessFS, "harness", dir) }
