//go:build windows

package e2e

import (
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// removeOnUnlock is false: a deleted file that another process still holds open blocks
// re-creating its name until every handle closes.
const removeOnUnlock = false

// lockRange is the locked byte, at 4 GiB: far past the PID, so the PID stays readable
// (LockFileEx ranges are mandatory) and locking past EOF is allowed.
func lockRange() *windows.Overlapped { return &windows.Overlapped{OffsetHigh: 1} }

// tryLockFile takes an exclusive LockFileEx without blocking; busy = another holder has it.
func tryLockFile(f *os.File) (busy bool, err error) {
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, lockRange())
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true, nil
	}
	return false, err
}

func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, lockRange())
}

// InheritLock is a no-op: windows children cannot inherit the handle (no ExtraFiles),
// so the lock lives with this process only.
func InheritLock(*exec.Cmd) {}
