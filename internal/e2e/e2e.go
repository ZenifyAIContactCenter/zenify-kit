// Package e2e builds and runs functional Playwright journeys in a pinned Docker image.
// Unlike visual (golden-diff), e2e mounts the repo-owned journey directory (.znf/e2e) and
// runs the repo-owned spec, importing generic fixtures from the embedded harness.
package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

type RunConfig struct {
	HarnessDir string // tmp dir the embedded harness was written to (fixtures + config + auth.setup)
	E2EDir     string // <repo>/.znf/e2e (mount ro-ish: holds e2e.config.json + *.spec.ts)
	Port       int    // port dev-server host
	BaseURL    string // origin to test instead of the host dev server (must pass CheckBaseURL)
}

// BuildArgs builds the `docker run` args for e2e (not including "docker").
func BuildArgs(o pwdocker.Options, cfg RunConfig) []string {
	args := pwdocker.BaseArgs(o.GOOS)
	baseURL := fmt.Sprintf("http://host.docker.internal:%d", cfg.Port)
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	args = append(args,
		"-v", cfg.HarnessDir+":/harness",
		"-v", cfg.E2EDir+":/harness/.znf/e2e",
		"-e", "BASE_URL="+baseURL,
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

// origin reduces a URL to scheme://host[:port], host lowercased; "" when it is not an http(s) URL.
func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + strings.ToLower(u.Host)
}

// CheckBaseURL allows rawURL only when its origin is listed in the repo's
// <e2eDir>/e2e.config.json "baseUrls" (path ignored, host case-insensitive).
func CheckBaseURL(e2eDir, rawURL string) error {
	b, err := os.ReadFile(filepath.Join(e2eDir, "e2e.config.json")) //nolint:gosec // G304 -- e2eDir is the repo's own .znf/e2e dir, not user input
	if err != nil {
		return exitcode.New(exitcode.BadArgs, fmt.Errorf("read e2e.config.json: %w", err))
	}
	var cfg struct {
		BaseURLs []string `json:"baseUrls"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return exitcode.New(exitcode.BadArgs, fmt.Errorf("parse e2e.config.json: %w", err))
	}
	want := origin(rawURL)
	allowed := make([]string, 0, len(cfg.BaseURLs))
	for _, a := range cfg.BaseURLs {
		if o := origin(a); o != "" {
			allowed = append(allowed, o)
			if o == want {
				return nil
			}
		}
	}
	return exitcode.New(exitcode.BadArgs,
		fmt.Errorf("--base-url %q is not allowed; e2e.config.json baseUrls allows: [%s]", rawURL, strings.Join(allowed, ", ")))
}
