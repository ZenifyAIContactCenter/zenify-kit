//go:build !windows

package cli

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockFile takes an exclusive advisory lock on f without blocking. It returns
// errLockHeld when another open file description holds the lock.
func tryLockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // G115 -- fd fits in int
	if errors.Is(err, unix.EWOULDBLOCK) {
		return errLockHeld
	}
	return err
}

// unlockFile releases the lock taken by tryLockFile.
func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:gosec // G115 -- fd fits in int
}
