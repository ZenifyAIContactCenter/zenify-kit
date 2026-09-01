// Package plugin nhúng cây plugin `znf` và materialize nó vào ~/.claude/skills/znf,
// theo dõi bằng managed.Manifest riêng để refresh tôn trọng sửa tay của user.
package plugin

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

//go:embed all:assets/znf
var assets embed.FS

const embedRoot = "assets/znf"

type Result struct {
	Written []string
	Kept    []string
	Skipped []string
}

func DefaultDest() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "skills", "znf"), nil
}

func DefaultManifest() (string, error) {
	dest, err := DefaultDest()
	if err != nil {
		return "", err
	}
	return filepath.Join(dest, ".manifest.json"), nil
}

// Sync ghi mọi file trong embed ra destRoot, record vào manifest tại manifestPath.
// Additive: chỉ ghi TRONG destRoot. Refresh-safe qua managed.DecideRefresh.
func Sync(destRoot, manifestPath string) (Result, error) {
	var res Result
	m, err := managed.Load(manifestPath)
	if err != nil {
		return res, err
	}
	err = fs.WalkDir(assets, embedRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, embedRoot+"/")
		target := filepath.Join(destRoot, rel)
		content, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil { //nolint:gosec // G304 -- target computed từ destRoot nội bộ
			switch m.DecideRefresh(target, existing) {
			case managed.DecisionKeepModified:
				res.Kept = append(res.Kept, target)
				return nil
			case managed.DecisionUpdate:
				if managed.Fingerprint(existing) == managed.Fingerprint(content) {
					res.Skipped = append(res.Skipped, target)
					return nil
				}
			case managed.DecisionKeepUserAdded:
				// target trên đĩa nhưng không trong manifest → ghi đè an toàn (nằm trong znf/).
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
