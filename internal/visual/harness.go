package visual

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed harness
var harnessFS embed.FS

// WriteHarness ghi toàn cây harness embedded ra dir (phẳng, bỏ prefix "harness/").
// Dùng để mount vào container mỗi lần chạy nên determinism đồng nhất mọi repo.
func WriteHarness(dir string) error {
	return fs.WalkDir(harnessFS, "harness", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("harness", p)
		dst := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := harnessFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}
