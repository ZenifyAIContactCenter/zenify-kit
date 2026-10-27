//go:build !windows

package hub

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// readComm returns the command name of pid from /proc; it errors where /proc
// has no such entry (macOS, or the process is gone). Tests replace it.
var readComm = func(pid int) (string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm") //nolint:gosec // G304 -- pid is an int
	return strings.TrimSpace(string(b)), err
}

// processAlive reports whether pid runs and, where /proc exposes its name, is
// a tmux process (a recycled pid belongs to something else). Signal 0 decides
// whether it runs: no error, or EPERM (another user's process), means it does.
// When the name cannot be read, the signal result stands.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	comm, err := readComm(pid)
	if err != nil {
		return true
	}
	return strings.HasPrefix(comm, "tmux")
}
