//go:build !windows

package e2e

import (
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

// removeOnUnlock: unlink the lock file on release (older binaries wait while it exists).
const removeOnUnlock = true

// tryLockFile takes an exclusive flock without blocking; busy = another holder has it.
func tryLockFile(f *os.File) (busy bool, err error) {
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // G115 -- fd fits int
	if errors.Is(err, unix.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}

func unlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) } //nolint:gosec // G115 -- fd fits int

// InheritLock hands the harness locks this process holds to child c, so npm/npx keep
// the harness locked if zenify is killed while they still run.
func InheritLock(c *exec.Cmd) {
	heldMu.Lock()
	defer heldMu.Unlock()
	c.ExtraFiles = append(c.ExtraFiles, heldLocks...)
}
