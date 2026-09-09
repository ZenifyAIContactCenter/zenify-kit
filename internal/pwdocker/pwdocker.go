// Package pwdocker holds the Docker+Playwright primitives shared between visual
// (golden-diff) and e2e (functional): version pin, image, per-OS args, and
// materializing the embedded harness. Kept in one place so the two capabilities
// don't drift on version.
package pwdocker

import (
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// PlaywrightVersion locks step between the Docker image and @playwright/test in
// harness/package.json. Bump both at the same time.
const PlaywrightVersion = "v1.55.0"

// Image returns the pinned Playwright image tag (-noble = Ubuntu 24.04).
func Image() string { return "mcr.microsoft.com/playwright:" + PlaywrightVersion + "-noble" }

// Options gathers the injected seams so every path is unit-testable without touching docker.
type Options struct {
	Runner func(name string, args []string) error
	Getenv func(string) string
	GOOS   string
	Stdout io.Writer
}

// BaseArgs starts the `docker run` args (not including "docker"): run, self-remove, cwd /harness.
// Linux needs --add-host for host.docker.internal to resolve; Docker Desktop already provides it, so it is NOT added.
func BaseArgs(goos string) []string {
	args := []string{"run", "--rm", "-w", "/harness"}
	if goos != "darwin" && goos != "windows" {
		args = append(args, "--add-host=host.docker.internal:host-gateway")
	}
	return args
}

// AuthEnvArgs passes E2E credentials through into the container (values read from the host environment).
func AuthEnvArgs() []string {
	return []string{"-e", "E2E_DOMAIN", "-e", "E2E_EMAIL", "-e", "E2E_PASSWORD"}
}

// WriteHarness writes the embedded tree (rooted at `root`) out to dir, flattened (dropping the root prefix).
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
