//go:build unix

package infra

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
)

const goos = runtime.GOOS

func parentDir(p string) string { return filepath.Dir(p) }

func statfsFree(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("infra: statfs %s: %w", path, err)
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec // block counts are non-negative
}
