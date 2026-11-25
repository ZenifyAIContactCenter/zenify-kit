//go:build windows

package infra

import (
	"errors"
	"path/filepath"
	"runtime"
)

const goos = runtime.GOOS

func parentDir(p string) string { return filepath.Dir(p) }

func statfsFree(string) (uint64, error) {
	return 0, errors.New("infra: disk probe is not supported on windows")
}
