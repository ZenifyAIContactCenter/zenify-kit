// Package visual builds and runs the Docker command that runs Playwright golden-diff for a
// target repo. Every shell-out goes through an injected Runner, so every path
// is unit-testable without touching docker (FR-1). The actual rendering happens in a
// pinned mcr.microsoft.com/playwright container so the baseline is portable cross-OS.
package visual

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

// Forwarding: visual_test.go (package visual) calls these names unqualified.
const PlaywrightVersion = pwdocker.PlaywrightVersion

func Image() string { return pwdocker.Image() }

// Options/RunConfig stay in package visual (visual_test.go uses them).
type Options = pwdocker.Options

type RunConfig struct {
	HarnessDir   string // tmp dir with the embedded harness written out (mount rw at /harness (Playwright writes .last-run.json to cwd))
	SnapshotsDir string // <repo>/.znf/visual (mount rw — holds routes.json + __snapshots__)
	Port         int    // dev-server port on the host
	Update       bool   // true = write baseline (--update-snapshots)
}

// BuildArgs builds the args for `docker run` (not including the word "docker"). Runner will call
// o.Runner("docker", args). Per-OS: Linux needs --add-host for host.docker.internal to
// resolve; Docker Desktop (darwin/windows) provides it already, so it is NOT added.
func BuildArgs(o Options, cfg RunConfig) []string {
	args := pwdocker.BaseArgs(o.GOOS)
	args = append(args,
		"-v", cfg.HarnessDir+":/harness",
		"-v", cfg.SnapshotsDir+":/harness/.znf/visual",
		"-e", fmt.Sprintf("BASE_URL=http://host.docker.internal:%d", cfg.Port),
	)
	args = append(args, pwdocker.AuthEnvArgs()...)
	// Browsers already exist in the image → npm install does NOT re-download them.
	args = append(args, "-e", "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	// `npx playwright test` in /harness cannot resolve @playwright/test (the image
	// only provides browsers, not the npm package on node's resolve path from an arbitrary cwd). Install
	// @playwright/test pinned from harness/package.json into /harness (mount rw, throwaway)
	// before running — the version is locked by package.json, browsers come from the image.
	cmd := "npm install --no-audit --no-fund --no-save --silent && npx playwright test"
	if cfg.Update {
		cmd += " --update-snapshots"
	}
	args = append(args, pwdocker.Image(), "sh", "-c", cmd)
	return args
}

// Check runs golden-diff through Runner. A Runner error (exit≠0) = mismatch/infrastructure;
// the caller maps it to an exit code.
func Check(o Options, cfg RunConfig) error {
	if err := o.Runner("docker", BuildArgs(o, cfg)); err != nil {
		return fmt.Errorf("visual check: %w", err)
	}
	return nil
}
