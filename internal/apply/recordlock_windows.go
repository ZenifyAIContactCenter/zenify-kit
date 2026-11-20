//go:build windows

package apply

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFileBlocking takes an exclusive lock on f, waiting for it.
func lockFileBlocking(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, new(windows.Overlapped))
}

// unlockFileBlocking releases the lock taken by lockFileBlocking.
func unlockFileBlocking(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}
