package observe

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
)

// deniedSuffix marks a meter key (e.g. "Read:denied") as bytes that never
// entered context — a hook denied the call before the tool ran. These are
// tracked for visibility but excluded from the totals that drive advise()
// and the statusline/zenify-cost tool-out figures.
const deniedSuffix = ":denied"

func isDenied(tool string) bool { return strings.HasSuffix(tool, deniedSuffix) }

// meterFile is the per-session tool-output accounting, written by the
// PostToolUse hook alongside count.json in the same session dir.
const meterFile = "meter.json"

// Meter is per-session tool-output accounting. It is passive: the PostToolUse
// hook records volume only — it cannot modify or truncate tool output, which is
// already fixed by the time PostToolUse fires (the hook is append-only).
type Meter struct {
	Calls map[string]int   `json:"calls"` // tool name -> invocation count
	Bytes map[string]int64 `json:"bytes"` // tool name -> total tool_response bytes
	// LastWarnCall is TotalCalls() at the last advice emitted; the debounce
	// compares against it so a burst of large results yields one line, not N.
	LastWarnCall int `json:"last_warn_call,omitempty"`
	// HeavyAsked is set once the session-heavy advice has been emitted. The
	// heavy branch then stays silent: measured 2026-09-19, a nag repeated every
	// 5 calls was ignored all day, so the one message now asks the user instead.
	HeavyAsked bool `json:"heavy_asked,omitempty"`
}

// Advice is what the PostToolUse hook may add to the agent's context after a
// tool call. Empty Message = say nothing (the common case).
type Advice struct {
	Message string
}

// Thresholds behind Advice. Measured 2026-09-18 on one workspace: 25 tool
// results over 50 KB (max 680 KB) in 30 days, each resident in context for the
// rest of its session; median context per turn 184k of a 200k window.
const (
	LargeResultBytes  = 50_000    // one tool_response above this → "route it to a file"
	HeavySessionBytes = 2_000_000 // session total above this → ask the user once (see HeavyAsked)
	warnDebounceCalls = 5         // at most one Advice per this many tool calls
)

// advise decides the Advice for a meter that has just absorbed a call of
// respBytes for tool. It mutates m.LastWarnCall when it speaks.
func advise(m *Meter, tool string, respBytes int64) Advice {
	total := m.TotalCalls()
	if m.LastWarnCall > 0 && total-m.LastWarnCall < warnDebounceCalls {
		return Advice{}
	}
	var msg string
	switch {
	case respBytes > LargeResultBytes:
		msg = fmt.Sprintf("znf meter: that %s result was %d KB and now stays in context for the rest of the session. "+
			"Send output this size to a file first, then read only the part you need.", tool, respBytes/1000)
	case m.TotalBytes() > HeavySessionBytes && !m.HeavyAsked:
		msg = fmt.Sprintf("znf meter: tool output this session has passed %d MB — the context is heavy. "+
			"Finish the current step to the nearest file boundary (spec, plan, ledger), then call AskUserQuestion with two options: "+
			"(a) /clear and re-enter through that file path — default; (b) continue in this session. "+
			"This is the only time the meter will say this.",
			m.TotalBytes()/1_000_000)
		m.HeavyAsked = true
	default:
		return Advice{}
	}
	m.LastWarnCall = total
	return Advice{Message: msg}
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

// TotalBytes sums metered response bytes across all tools, excluding denied
// keys (bytes that were never read into context — see isDenied).
func (m Meter) TotalBytes() int64 {
	var t int64
	for tool, b := range m.Bytes {
		if isDenied(tool) {
			continue
		}
		t += b
	}
	return t
}

// TotalCalls sums metered calls across all tools, excluding denied keys.
func (m Meter) TotalCalls() int {
	var t int
	for tool, c := range m.Calls {
		if isDenied(tool) {
			continue
		}
		t += c
	}
	return t
}

// DeniedCalls sums metered calls across denied keys only.
func (m Meter) DeniedCalls() int {
	var t int
	for tool, c := range m.Calls {
		if isDenied(tool) {
			t += c
		}
	}
	return t
}

// Record adds one tool call of respBytes to the session meter and returns the
// Advice (usually empty) the hook should surface. The read-modify-write runs
// under the same per-session flock as Bump (non-blocking with bounded retry);
// any error or persistent contention falls open silently — empty Advice, no
// write — so the PostToolUse hook never disrupts the tool it observes. Empty
// tool or negative size is ignored.
func Record(sessionID, tool string, respBytes int64, now time.Time) Advice {
	if tool == "" || respBytes < 0 {
		return Advice{}
	}
	base, err := dir()
	if err != nil {
		return Advice{}
	}
	sess := sanitizeSession(sessionID)
	sd := sessDir(base, sess)
	if err := os.MkdirAll(sd, 0o750); err != nil {
		return Advice{}
	}

	var h *lock.Handle
	for i := 0; i < lockRetries; i++ {
		h, err = lock.Acquire(sd, os.Getpid(), sess, now.Unix())
		if err == nil {
			break
		}
		if !errors.Is(err, lock.ErrHeld) {
			return Advice{} // unexpected error → fail open
		}
		time.Sleep(lockBackoff)
	}
	if h == nil {
		return Advice{} // still contended → skip this record, fail open
	}
	defer func() { _ = h.Release() }()

	m, ok := readMeter(meterPath(base, sess))
	if !ok {
		return Advice{} // corrupt or unreadable → fail open
	}
	if m.Calls == nil {
		m.Calls = map[string]int{}
	}
	if m.Bytes == nil {
		m.Bytes = map[string]int64{}
	}
	m.Calls[tool]++
	m.Bytes[tool] += respBytes
	if isDenied(tool) {
		// Denied bytes never entered context: record for visibility, but skip
		// advise() entirely so they cannot flip HeavyAsked or debounce the
		// next real warning.
		_ = writeMeter(meterPath(base, sess), m)
		return Advice{}
	}
	adv := advise(&m, tool, respBytes)
	_ = writeMeter(meterPath(base, sess), m)
	return adv
}
