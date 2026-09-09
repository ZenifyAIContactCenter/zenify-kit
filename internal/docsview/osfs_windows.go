//go:build windows

package docsview

import (
	"os"
	"os/exec"
)

// Link creates a DIRECTORY JUNCTION (mklink /J) — no admin/Developer-Mode needed, unlike a
// symbolic link. mklink arg order: mklink /J <LinkName> <Target>.
func (OSFS) Link(target, link string) error {
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
}

// IsManagedLink: catches BOTH ModeSymlink and ModeIrregular because Go's junction
// classification shifts across versions (go#23684 surrogate-bit) — check both to stay
// robust across Go versions.
func (OSFS) IsManagedLink(p string) (bool, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return false, err
	}
	return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0, nil
}
