package docsview

import (
	"os"
	"path/filepath"
)

// OSFS là FS thật. Phần OS-specific (Link, IsManagedLink) ở osfs_unix.go / osfs_windows.go.
type OSFS struct{}

func (OSFS) ReadDir(p string) ([]os.DirEntry, error) { return os.ReadDir(p) }
func (OSFS) MkdirAll(p string, m os.FileMode) error  { return os.MkdirAll(p, m) }
func (OSFS) Remove(p string) error                   { return os.Remove(p) } // junction: chỉ gỡ link

// SameTarget verify OS-agnostic: resolve cả symlink lẫn junction rồi so.
// Lỗi (link chết / target mất) → false,err → caller coi như cần tạo lại/prune.
func (OSFS) SameTarget(link, target string) (bool, error) {
	lr, err := filepath.EvalSymlinks(link)
	if err != nil {
		return false, err
	}
	tr, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false, err
	}
	return lr == tr, nil
}
