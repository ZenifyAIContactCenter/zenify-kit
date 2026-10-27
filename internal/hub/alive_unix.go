//go:build !windows

package hub

import (
	"errors"
	"os"
	"syscall"
)

// processAlive sends signal 0 to pid: no error, or EPERM (the process exists
// but belongs to another user), means it runs.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
