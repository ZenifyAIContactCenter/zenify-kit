//go:build !windows

package lock

import (
	"errors"
	"os"
	"syscall"
)

// flockExclusive takes a non-blocking exclusive flock on f. It returns
// (true, nil) when the lock is taken, (false, nil) when another process holds
// it, and (false, err) on an unexpected error.
func flockExclusive(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

// flockUnlock releases the flock held on f.
func flockUnlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
