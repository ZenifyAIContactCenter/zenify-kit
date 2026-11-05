//go:build !windows

package e2e

import (
	"errors"
	"syscall"
)

// processAlive: signal 0 probes without delivering; EPERM still means it runs.
func processAlive(pid int) (alive, known bool) {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM), true
}
