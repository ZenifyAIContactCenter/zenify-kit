//go:build !windows

package docsview

import "os"

func (OSFS) Link(target, link string) error { return os.Symlink(target, link) }

func (OSFS) IsManagedLink(p string) (bool, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return false, err
	}
	return fi.Mode()&os.ModeSymlink != 0, nil
}
