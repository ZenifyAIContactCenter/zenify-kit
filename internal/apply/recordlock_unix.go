//go:build !windows

package apply

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFileBlocking takes an exclusive advisory lock on f, waiting for it.
func lockFileBlocking(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX) //nolint:gosec // G115 -- fd fits in int
}

// unlockFileBlocking releases the lock taken by lockFileBlocking.
func unlockFileBlocking(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:gosec // G115 -- fd fits in int
}
