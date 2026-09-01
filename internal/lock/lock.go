// Package lock provides a best-effort advisory lock over a workspace directory
// so two `zenify` mutation runs cannot act on the same workspace at once. The
// lock is a JSON lockfile recording the holding PID; a lock whose PID is no
// longer alive is treated as stale and replaced.
package lock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockName is the lockfile basename inside the workspace directory.
const lockName = ".zenify.lock"

// Info is the content of a lockfile.
type Info struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	CreatedAt int64  `json:"created_at"` // unix seconds, injected by the caller
}

// ErrHeld reports that another live process holds the lock.
var ErrHeld = errors.New("workspace is locked by another zenify process")

// Handle is an acquired lock. Call Release to free it.
type Handle struct {
	path string
}

// Acquire creates the workspace lockfile atomically. If a lockfile already
// exists and its recorded PID is still alive (and is not pid), Acquire returns
// ErrHeld. A stale lock (dead PID), our own leftover, or a corrupt lockfile is
// removed and replaced. now is the unix timestamp to record, injected so tests
// stay deterministic.
func Acquire(dir string, pid int, host string, now int64) (*Handle, error) {
	p := filepath.Join(dir, lockName)
	body, err := json.Marshal(Info{PID: pid, Host: host, CreatedAt: now})
	if err != nil {
		return nil, err
	}

	h, err := create(p, body)
	if err == nil {
		return h, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	// A lockfile already exists — decide whether it is stale.
	held, rerr := readInfo(p)
	if rerr == nil && held.PID != pid && processAlive(held.PID) {
		return nil, fmt.Errorf("%w (pid %d on %s)", ErrHeld, held.PID, held.Host)
	}
	// Stale, our own leftover, or corrupt: replace it.
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	h, err = create(p, body)
	if errors.Is(err, os.ErrExist) {
		return nil, ErrHeld // lost a race to another acquirer
	}
	return h, err
}

// create writes body to a new lockfile at p using O_CREATE|O_EXCL, so an
// existing file yields os.ErrExist rather than a silent overwrite.
func create(p string, body []byte) (*Handle, error) {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_, werr := f.Write(body)
	cerr := f.Close()
	if werr != nil {
		os.Remove(p)
		return nil, werr
	}
	if cerr != nil {
		os.Remove(p)
		return nil, cerr
	}
	return &Handle{path: p}, nil
}

func readInfo(p string) (Info, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return Info{}, err
	}
	var i Info
	if err := json.Unmarshal(b, &i); err != nil {
		return Info{}, err
	}
	return i, nil
}

// processAlive reports whether a process with the given PID exists. On Unix it
// sends signal 0, which probes existence without affecting the process; EPERM
// means the process exists but is owned by another user.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

// Release removes the lockfile. A nil Handle releases nothing.
func (h *Handle) Release() error {
	if h == nil {
		return nil
	}
	return os.Remove(h.path)
}
