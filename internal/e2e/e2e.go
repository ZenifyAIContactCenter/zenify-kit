// Package e2e dựng và chạy journey Playwright functional trong Docker pinned.
// Khác visual (golden-diff), e2e mount thư mục journey repo-owned (.znf/e2e) và
// chạy spec do repo sở hữu, import fixtures generic từ harness embedded.
package e2e

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

type RunConfig struct {
	HarnessDir string // tmp dir đã ghi harness embedded (fixtures + config + auth.setup)
	E2EDir     string // <repo>/.znf/e2e (mount ro-ish: chứa e2e.config.json + *.spec.ts)
	Port       int    // port dev-server host
}

// BuildArgs dựng args `docker run` cho e2e (chưa gồm "docker").
func BuildArgs(o pwdocker.Options, cfg RunConfig) []string {
	args := pwdocker.BaseArgs(o.GOOS)
	args = append(args,
		"-v", cfg.HarnessDir+":/harness",
		"-v", cfg.E2EDir+":/harness/.znf/e2e",
		"-e", fmt.Sprintf("BASE_URL=http://host.docker.internal:%d", cfg.Port),
		"-e", "STORAGE_STATE=/tmp/znf-e2e-storage.json",
	)
	args = append(args, pwdocker.AuthEnvArgs()...)
	args = append(args, "-e", "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	cmd := "npm install --no-audit --no-fund --no-save --silent && npx playwright test"
	args = append(args, pwdocker.Image(), "sh", "-c", cmd)
	return args
}

// Check chạy journey qua Runner. Exit≠0 = một scenario fail (assert/hạ tầng).
func Check(o pwdocker.Options, cfg RunConfig) error {
	if err := o.Runner("docker", BuildArgs(o, cfg)); err != nil {
		return fmt.Errorf("e2e run: %w", err)
	}
	return nil
}
