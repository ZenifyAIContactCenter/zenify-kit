// Package lock provides a best-effort advisory lock over a workspace directory
// so two `zenify` mutation runs cannot act on the same workspace at once.
//
// Exclusion is provided by an OS advisory lock (flock on unix, LockFileEx on
// Windows) held on an open file descriptor for the lifetime of the Handle. The
// kernel releases the lock automatically if the process dies, so there is no
// stale-lockfile class to reason about, and Release only ever unlocks the fd
// this process owns. The JSON body of the lockfile is a diagnostic sidecar
// (who holds it) recorded for the ErrHeld message; it is not the lock.
package lock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// lockName is the lockfile basename inside the workspace directory.
const lockName = ".zenify.lock"

// Info is the diagnostic content of a lockfile.
type Info struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	CreatedAt int64  `json:"created_at"` // unix seconds, injected by the caller
}

// ErrHeld reports that another process holds the lock.
var ErrHeld = errors.New("workspace is locked by another zenify process")

// Handle is an acquired lock. Call Release to free it. It holds the locked file
// descriptor open; releasing (or the process exiting) unlocks it.
type Handle struct {
	f    *os.File
	path string
}

// Acquire takes an exclusive advisory lock on the workspace lockfile. If another
// process already holds it, Acquire returns ErrHeld. now/pid/host are recorded
// in the lockfile as diagnostics only. The signature is unchanged from the
// file-lock version so callers and tests stay stable.
func Acquire(dir string, pid int, host string, now int64) (*Handle, error) {
	p := filepath.Join(dir, lockName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	locked, err := flockExclusive(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	if !locked {
		// Someone else holds it. Best-effort read of the sidecar for the message.
		held, rerr := readInfo(p)
		f.Close()
		if rerr == nil {
			return nil, fmt.Errorf("%w (pid %d on %s)", ErrHeld, held.PID, held.Host)
		}
		return nil, ErrHeld
	}

	// We hold the lock. Overwrite the sidecar with our diagnostics.
	body, err := json.Marshal(Info{PID: pid, Host: host, CreatedAt: now})
	if err != nil {
		flockUnlock(f)
		f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		flockUnlock(f)
		f.Close()
		return nil, err
	}
	if _, err := f.WriteAt(body, 0); err != nil {
		flockUnlock(f)
		f.Close()
		return nil, err
	}
	return &Handle{f: f, path: p}, nil
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

// Release unlocks and closes the held descriptor. A nil Handle releases nothing.
// The lockfile itself is intentionally left on disk: the flock, not the file's
// presence, is what excludes, so removing it would only reintroduce a race.
func (h *Handle) Release() error {
	if h == nil || h.f == nil {
		return nil
	}
	uerr := flockUnlock(h.f)
	cerr := h.f.Close()
	h.f = nil
	if uerr != nil {
		return uerr
	}
	return cerr
}
