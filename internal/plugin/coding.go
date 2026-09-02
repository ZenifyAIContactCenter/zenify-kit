package plugin

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

//go:embed all:assets/coding
var codingAssets embed.FS

const codingRoot = "assets/coding"

// CodingSkills liệt kê tên thư mục skill dưới assets/coding (động, không đăng ký).
func CodingSkills() []string {
	entries, err := fs.ReadDir(codingAssets, codingRoot)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// InstallCoding materialize CHỈ các skill có tên trong `skills` từ assets/coding
// vào destRoot, additive/refresh-safe (giống Sync). Bỏ qua tên không tồn tại.
func InstallCoding(destRoot, manifestPath string, skills []string) (Result, error) {
	var res Result
	want := map[string]bool{}
	for _, s := range skills {
		want[s] = true
	}
	m, err := managed.Load(manifestPath)
	if err != nil {
		return res, err
	}
	err = fs.WalkDir(codingAssets, codingRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, codingRoot+"/")
		top := rel
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			top = rel[:i]
		}
		if !want[top] {
			return nil
		}
		target := filepath.Join(destRoot, rel)
		content, err := codingAssets.ReadFile(p)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil { //nolint:gosec // G304
			switch m.DecideRefresh(target, existing) {
			case managed.DecisionKeepModified, managed.DecisionKeepUserAdded:
				res.Kept = append(res.Kept, target)
				return nil
			case managed.DecisionUpdate:
				if managed.Fingerprint(existing) == managed.Fingerprint(content) {
					res.Skipped = append(res.Skipped, target)
					return nil
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
		if err := m.Record(target); err != nil {
			return err
		}
		res.Written = append(res.Written, target)
		return nil
	})
	if err != nil {
		return res, err
	}
	if err := m.Save(manifestPath); err != nil {
		return res, err
	}
	return res, nil
}
