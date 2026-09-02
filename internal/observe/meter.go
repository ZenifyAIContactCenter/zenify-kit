package observe

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
)

// meterFile is the per-session tool-output accounting, written by the
// PostToolUse hook alongside count.json in the same session dir.
const meterFile = "meter.json"

// Meter is per-session tool-output accounting. It is passive: the PostToolUse
// hook records volume only — it cannot modify or truncate tool output, which is
// already fixed by the time PostToolUse fires (the hook is append-only).
type Meter struct {
	Calls map[string]int   `json:"calls"` // tool name -> invocation count
	Bytes map[string]int64 `json:"bytes"` // tool name -> total tool_response bytes
}

// meterPath is the meter file for a session (sits beside count.json + the lock).
func meterPath(base, sess string) string { return filepath.Join(sessDir(base, sess), meterFile) }

// readMeter mirrors readState: (Meter{}, true) when absent (fresh session),
// (m, true) on success, (Meter{}, false) on any other read/unmarshal error so
// the caller fails open rather than clobbering a corrupt file with a zero value.
func readMeter(path string) (Meter, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path computed internally from XDG state dir + sanitized session id
	if err != nil {
		if os.IsNotExist(err) {
			return Meter{}, true
		}
		return Meter{}, false
	}
	var m Meter
	if err := json.Unmarshal(b, &m); err != nil {
		return Meter{}, false
	}
	return m, true
}

func writeMeter(path string, m Meter) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadMeter returns a session's tool-output accounting for display, WITHOUT the
// lock (a read-only snapshot). Same ok semantics as LoadState: absent → fresh
// session (Meter{}, true); torn/corrupt read → (Meter{}, false).
func LoadMeter(sessionID string) (Meter, bool) {
	base, err := dir()
	if err != nil {
		return Meter{}, false
	}
	return readMeter(meterPath(base, sanitizeSession(sessionID)))
}

// TotalBytes sums metered response bytes across all tools.
func (m Meter) TotalBytes() int64 {
	var t int64
	for _, b := range m.Bytes {
		t += b
	}
	return t
}

// TotalCalls sums metered calls across all tools.
func (m Meter) TotalCalls() int {
	var t int
	for _, c := range m.Calls {
		t += c
	}
	return t
}

// Record adds one tool call of respBytes to the session meter. The
// read-modify-write runs under the same per-session flock as Bump (non-blocking
// with bounded retry); any error or persistent contention falls open silently so
// the PostToolUse hook never disrupts the tool it observes. Empty tool or
// negative size is ignored.
func Record(sessionID, tool string, respBytes int64, now time.Time) {
	if tool == "" || respBytes < 0 {
		return
	}
	base, err := dir()
	if err != nil {
		return
	}
	sess := sanitizeSession(sessionID)
	sd := sessDir(base, sess)
	if err := os.MkdirAll(sd, 0o750); err != nil {
		return
	}

	var h *lock.Handle
	for i := 0; i < lockRetries; i++ {
		h, err = lock.Acquire(sd, os.Getpid(), sess, now.Unix())
		if err == nil {
			break
		}
		if !errors.Is(err, lock.ErrHeld) {
			return // unexpected error → fail open
		}
		time.Sleep(lockBackoff)
	}
	if h == nil {
		return // still contended → skip this record, fail open
	}
	defer func() { _ = h.Release() }()

	m, ok := readMeter(meterPath(base, sess))
	if !ok {
		return // corrupt or unreadable → fail open
	}
	if m.Calls == nil {
		m.Calls = map[string]int{}
	}
	if m.Bytes == nil {
		m.Bytes = map[string]int64{}
	}
	m.Calls[tool]++
	m.Bytes[tool] += respBytes
	_ = writeMeter(meterPath(base, sess), m)
}
