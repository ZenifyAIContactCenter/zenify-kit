// Package e2e builds and runs functional Playwright journeys in a pinned Docker image.
// Unlike visual (golden-diff), e2e mounts the repo-owned journey directory (.znf/e2e) and
// runs the repo-owned spec, importing generic fixtures from the embedded harness.
package e2e

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

type RunConfig struct {
	HarnessDir string // tmp dir the embedded harness was written to (fixtures + config + auth.setup)
	E2EDir     string // <repo>/.znf/e2e (mount ro-ish: holds e2e.config.json + *.spec.ts)
	Port       int    // port dev-server host
}

// BuildArgs builds the `docker run` args for e2e (not including "docker").
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

// Check runs the journey via Runner. Exit≠0 = a scenario failed (assertion/infra).
func Check(o pwdocker.Options, cfg RunConfig) error {
	if err := o.Runner("docker", BuildArgs(o, cfg)); err != nil {
		return fmt.Errorf("e2e run: %w", err)
	}
	return nil
}
