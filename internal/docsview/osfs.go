package docsview

import (
	"os"
	"path/filepath"
)

// OSFS is the real FS. The OS-specific part (Link, IsManagedLink) lives in
// osfs_unix.go / osfs_windows.go.
type OSFS struct{}

func (OSFS) ReadDir(p string) ([]os.DirEntry, error) { return os.ReadDir(p) }
func (OSFS) MkdirAll(p string, m os.FileMode) error  { return os.MkdirAll(p, m) }
func (OSFS) Remove(p string) error                   { return os.Remove(p) } // junction: only removes the link

// SameTarget verifies OS-agnostically: resolves both symlink and junction, then compares.
// Error (dead link / missing target) → false,err → caller treats it as needing recreate/prune.
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
