package observe

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
)

const (
	defaultCap  = 10
	capEnv      = "ZENIFY_DISPATCH_SOFTCAP"
	pruneMaxAge = 7 * 24 * time.Hour
	lockRetries = 25
	lockBackoff = 2 * time.Millisecond
)

// ResolveCap reads ZENIFY_DISPATCH_SOFTCAP. Unset or unparseable → defaultCap.
// A returned value <= 0 means the feature is disabled; the caller short-circuits.
func ResolveCap(getenv func(string) string) int {
	v := getenv(capEnv)
	if v == "" {
		return defaultCap
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultCap
	}
	return n
}

// dir returns $XDG_STATE_HOME/zenify/observe, falling back to
// ~/.local/state/zenify/observe per the XDG base-dir spec (mirrors wt.IndexPath).
func dir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "zenify", "observe"), nil
}

// sanitizeSession keeps only filename-safe chars so a hostile session_id cannot
// escape the state dir. Empty result → "unknown".
func sanitizeSession(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

// sessDir is the per-session directory (holds count.json + the lock file).
func sessDir(base, sess string) string { return filepath.Join(base, sess) }

// sessPath is the counter file for a session.
func sessPath(base, sess string) string { return filepath.Join(sessDir(base, sess), "count.json") }

// readState returns (State{}, true) when the file does not exist (a normal
// fresh session), (st, true) on a successful read, and (State{}, false) on any
// other read error or a JSON-unmarshal error (corruption/unreadable) — the
// caller must fail open on false rather than proceed with a zero State.
func readState(path string) (State, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path computed internally from XDG state dir + sanitized session id
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, true
		}
		return State{}, false
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, false
	}
	return st, true
}

func writeState(path string, st State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadState returns a session's counter for display, WITHOUT taking the lock —
// a read-only snapshot. ok is false only on an unexpected read/unmarshal error
// (a torn mid-write read); an absent file is a fresh session and returns
// (State{}, true). The statusline shows nothing rather than a wrong number when
// ok is false.
func LoadState(sessionID string) (State, bool) {
	base, err := dir()
	if err != nil {
		return State{}, false
	}
	return readState(sessPath(base, sanitizeSession(sessionID)))
}

// Bump loads the session counter, applies one dispatch, persists it, prunes
// stale sessions, and returns the Decision. The read-modify-write runs under a
// per-session flock (internal/lock, non-blocking) with bounded retry; any error
// or persistent contention falls open to Decision{} so the hook never blocks.
// softCap must be > 0.
func Bump(sessionID string, softCap int, now time.Time) Decision {
	base, err := dir()
	if err != nil {
		return Decision{}
	}
	sess := sanitizeSession(sessionID)
	sd := sessDir(base, sess)
	if err := os.MkdirAll(sd, 0o750); err != nil {
		return Decision{}
	}

	var h *lock.Handle
	for i := 0; i < lockRetries; i++ {
		h, err = lock.Acquire(sd, os.Getpid(), sess, now.Unix())
		if err == nil {
			break
		}
		if !errors.Is(err, lock.ErrHeld) {
			return Decision{} // unexpected error → fail-open
		}
		time.Sleep(lockBackoff)
	}
	if h == nil {
		return Decision{} // still contended → skip this increment, fail-open
	}
	defer func() { _ = h.Release() }()

	st, ok := readState(sessPath(base, sess))
	if !ok {
		return Decision{} // corrupt or unreadable state file → fail open
	}
	st, dec := Evaluate(st, softCap)
	if err := writeState(sessPath(base, sess), st); err != nil {
		return Decision{} // could not persist → never warn on this dispatch
	}
	prune(base, now)
	return dec
}

// prune removes session dirs whose count.json is older than pruneMaxAge. Best
// effort: any error is swallowed so it never affects the main path.
func prune(base string, now time.Time) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := sessPath(base, e.Name())
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) > pruneMaxAge {
			_ = os.RemoveAll(sessDir(base, e.Name()))
		}
	}
}
