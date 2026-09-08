// Package pwdocker giữ các primitive Docker+Playwright dùng chung giữa visual
// (golden-diff) và e2e (functional): pin version, image, arg per-OS, và materialize
// harness embed. Tách ra một nơi để hai capability không lệch version.
package pwdocker

import (
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// PlaywrightVersion khoá lockstep giữa image Docker và @playwright/test trong
// harness/package.json. Bump cả hai cùng lúc.
const PlaywrightVersion = "v1.55.0"

// Image trả tag image Playwright pinned (-noble = Ubuntu 24.04).
func Image() string { return "mcr.microsoft.com/playwright:" + PlaywrightVersion + "-noble" }

// Options gom seam inject để mọi path unit-test được mà không chạm docker.
type Options struct {
	Runner func(name string, args []string) error
	Getenv func(string) string
	GOOS   string
	Stdout io.Writer
}

// BaseArgs mở đầu args `docker run` (chưa gồm "docker"): chạy, tự xoá, cwd /harness.
// Linux cần --add-host để host.docker.internal giải được; Docker Desktop có sẵn nên KHÔNG thêm.
func BaseArgs(goos string) []string {
	args := []string{"run", "--rm", "-w", "/harness"}
	if goos != "darwin" && goos != "windows" {
		args = append(args, "--add-host=host.docker.internal:host-gateway")
	}
	return args
}

// AuthEnvArgs passthrough credential E2E vào container (giá trị lấy từ môi trường host).
func AuthEnvArgs() []string {
	return []string{"-e", "E2E_DOMAIN", "-e", "E2E_EMAIL", "-e", "E2E_PASSWORD"}
}

// WriteHarness ghi cây embed (rooted ở `root`) ra dir, phẳng (bỏ prefix root).
func WriteHarness(fsys embed.FS, root, dir string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		dst := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := fsys.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}
