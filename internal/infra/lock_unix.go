//go:build unix

package infra

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockHost takes the host-wide lock of `zenify infra up/down`
// (~/.zenify/infra/infra.lock, flock): two runs would race on the same temp
// worktrees and root files. A held lock is an error naming the file; the lock
// dies with the process, so a crash never leaves it stuck.
func LockHost(home string) (unlock func() error, err error) {
	dir := infraDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("infra lock: mkdir %s: %w", dir, err)
	}
	p := filepath.Join(dir, "infra.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304 -- fixed name under the kit's home dir
	if err != nil {
		return nil, fmt.Errorf("infra lock: open %s: %w", p, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // G115 -- a file descriptor fits in int
		cerr := f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.Join(fmt.Errorf("another zenify infra up/down is running (lock %s)", p), cerr)
		}
		return nil, errors.Join(fmt.Errorf("infra lock: flock %s: %w", p, err), cerr)
	}
	return func() error {
		uerr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:gosec // G115 -- a file descriptor fits in int
		return errors.Join(uerr, f.Close())
	}, nil
}
