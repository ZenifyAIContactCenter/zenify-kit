//go:build windows

package infra

import (
	"errors"
	"runtime"
)

const goos = runtime.GOOS

func statfsFree(string) (uint64, error) {
	return 0, errors.New("infra: disk probe is not supported on windows")
}
