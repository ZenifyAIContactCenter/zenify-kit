// Package visual dựng và chạy lệnh Docker chạy Playwright golden-diff cho một
// target repo. Mọi shell-out đi qua một Runner được inject nên mọi path
// unit-test được mà không chạm docker (FR-1). Việc render thật diễn ra trong
// container mcr.microsoft.com/playwright pinned để baseline portable cross-OS.
package visual

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

// Forwarding: visual_test.go (package visual) gọi các tên này không qualify.
const PlaywrightVersion = pwdocker.PlaywrightVersion

func Image() string { return pwdocker.Image() }

// Options/RunConfig giữ nguyên trong package visual (visual_test.go dùng).
type Options = pwdocker.Options

type RunConfig struct {
	HarnessDir   string // tmp dir đã ghi harness embedded (mount rw vào /harness (Playwright ghi .last-run.json vào cwd))
	SnapshotsDir string // <repo>/.znf/visual (mount rw — chứa routes.json + __snapshots__)
	Port         int    // port dev-server trên host
	Update       bool   // true = ghi baseline (--update-snapshots)
}

// BuildArgs dựng args cho `docker run` (không gồm chữ "docker"). Runner sẽ gọi
// o.Runner("docker", args). Per-OS: Linux cần --add-host để host.docker.internal
// giải được; Docker Desktop (darwin/windows) cung cấp sẵn nên KHÔNG thêm.
func BuildArgs(o Options, cfg RunConfig) []string {
	args := pwdocker.BaseArgs(o.GOOS)
	args = append(args,
		"-v", cfg.HarnessDir+":/harness",
		"-v", cfg.SnapshotsDir+":/harness/.znf/visual",
		"-e", fmt.Sprintf("BASE_URL=http://host.docker.internal:%d", cfg.Port),
	)
	args = append(args, pwdocker.AuthEnvArgs()...)
	// Browsers đã có sẵn trong image → npm install KHÔNG tải lại browser.
	args = append(args, "-e", "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	// `npx playwright test` trong /harness không resolve được @playwright/test (image
	// chỉ cấp browsers, không cấp npm package trên đường node resolve từ cwd tuỳ ý). Cài
	// @playwright/test pinned từ harness/package.json vào /harness (mount rw, throwaway)
	// rồi mới chạy — version khoá bởi package.json, browsers lấy từ image.
	cmd := "npm install --no-audit --no-fund --no-save --silent && npx playwright test"
	if cfg.Update {
		cmd += " --update-snapshots"
	}
	args = append(args, pwdocker.Image(), "sh", "-c", cmd)
	return args
}

// Check chạy golden-diff qua Runner. Lỗi Runner (exit≠0) = mismatch/hạ tầng;
// caller map sang exit code.
func Check(o Options, cfg RunConfig) error {
	if err := o.Runner("docker", BuildArgs(o, cfg)); err != nil {
		return fmt.Errorf("visual check: %w", err)
	}
	return nil
}
