// Package visual dựng và chạy lệnh Docker chạy Playwright golden-diff cho một
// target repo. Mọi shell-out đi qua một Runner được inject nên mọi path
// unit-test được mà không chạm docker (FR-1). Việc render thật diễn ra trong
// container mcr.microsoft.com/playwright pinned để baseline portable cross-OS.
package visual

import (
	"fmt"
	"io"
)

// PlaywrightVersion khoá lockstep giữa image Docker và @playwright/test trong
// harness/package.json (FR-2). Bump cả hai cùng lúc, không thì baseline lệch.
const PlaywrightVersion = "v1.55.0"

// Image trả tag image Playwright pinned (biến thể -noble = Ubuntu 24.04).
func Image() string { return "mcr.microsoft.com/playwright:" + PlaywrightVersion + "-noble" }

type Options struct {
	Runner func(name string, args []string) error
	Getenv func(string) string
	GOOS   string
	Stdout io.Writer
}

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
	args := []string{"run", "--rm", "-w", "/harness"}
	if o.GOOS != "darwin" && o.GOOS != "windows" {
		args = append(args, "--add-host=host.docker.internal:host-gateway")
	}
	args = append(args,
		"-v", cfg.HarnessDir+":/harness",
		"-v", cfg.SnapshotsDir+":/harness/.znf/visual",
		"-e", fmt.Sprintf("BASE_URL=http://host.docker.internal:%d", cfg.Port),
		"-e", "E2E_DOMAIN", "-e", "E2E_EMAIL", "-e", "E2E_PASSWORD",
		// Browsers đã có sẵn trong image → npm install KHÔNG tải lại browser.
		"-e", "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
	)
	// `npx playwright test` trong /harness không resolve được @playwright/test (image
	// chỉ cấp browsers, không cấp npm package trên đường node resolve từ cwd tuỳ ý). Cài
	// @playwright/test pinned từ harness/package.json vào /harness (mount rw, throwaway)
	// rồi mới chạy — version khoá bởi package.json, browsers lấy từ image.
	cmd := "npm install --no-audit --no-fund --no-save --silent && npx playwright test"
	if cfg.Update {
		cmd += " --update-snapshots"
	}
	args = append(args, Image(), "sh", "-c", cmd)
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
