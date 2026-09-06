//go:build windows

package docsview

import (
	"os"
	"os/exec"
)

// Link tạo DIRECTORY JUNCTION (mklink /J) — không cần admin/Developer-Mode, khác symbolic link.
// Thứ tự arg mklink: mklink /J <LinkName> <Target>.
func (OSFS) Link(target, link string) error {
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
}

// IsManagedLink: bắt CẢ ModeSymlink lẫn ModeIrregular vì Go phân loại junction đổi qua các
// bản (go#23684 surrogate-bit) — check cả hai để robust theo phiên bản Go.
func (OSFS) IsManagedLink(p string) (bool, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return false, err
	}
	return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0, nil
}
