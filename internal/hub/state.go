// Package hub holds the agent-state model behind `zenify hub`: a tmux sidebar
// that shows coding agents and the worktree task each one works on.
package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AgentState is the lifecycle state of one coding agent.
type AgentState string

const (
	Working AgentState = "working"
	Blocked AgentState = "blocked"
	Done    AgentState = "done" // finished, not yet seen
	Idle    AgentState = "idle"
)

// rank orders states for roll-up: higher wins.
func rank(s AgentState) int {
	switch s {
	case Blocked:
		return 3
	case Working:
		return 2
	case Done:
		return 1
	}
	return 0
}

// Rollup returns the highest-priority state: blocked > working > done > idle.
// No states yields Idle.
func Rollup(states ...AgentState) AgentState {
	best := Idle
	for _, s := range states {
		if rank(s) > rank(best) {
			best = s
		}
	}
	return best
}

// Agent is the persisted record of one agent session.
type Agent struct {
	SessionID string     `json:"session_id"`
	Pane      string     `json:"pane"`
	State     AgentState `json:"state"`
	Slug      string     `json:"slug"`
	Title     string     `json:"title"`
	UpdatedAt int64      `json:"updated_at"`
}

// Dir is the hub data directory under home.
func Dir(home string) string { return filepath.Join(home, ".zenify", "hub") }

// AgentsDir holds one JSON file per agent session.
func AgentsDir(home string) string { return filepath.Join(Dir(home), "agents") }

func agentPath(home, sessionID string) string {
	return filepath.Join(AgentsDir(home), sessionID+".json")
}

// readAgent loads an agent file; a missing file yields (nil, nil).
func readAgent(home, sessionID string) (*Agent, error) {
	b, err := os.ReadFile(agentPath(home, sessionID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agent %s: %w", sessionID, err)
	}
	var a Agent
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("decode agent %s: %w", sessionID, err)
	}
	return &a, nil
}

// writeAgent writes the agent file atomically (temp file + rename, mode 0600).
func writeAgent(home string, a *Agent) error {
	b, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode agent: %w", err)
	}
	if err := writeFileAtomic(AgentsDir(home), agentPath(home, a.SessionID), ".agent-*.tmp", b); err != nil {
		return fmt.Errorf("write agent %s: %w", a.SessionID, err)
	}
	return nil
}

// writeFileAtomic writes b to path (inside dir, created 0700 if missing) via a
// temp file named by pattern plus rename, so readers never see a partial file.
func writeFileAtomic(dir, path, pattern string, b []byte) error {
	return writeFileAtomicIf(dir, path, pattern, b, nil)
}

// removeTemp deletes a temp file; tests replace it to inject a failure.
var removeTemp = os.Remove

// writeFileAtomicIf is writeFileAtomic that runs check (when non-nil) right
// before the rename; a check error aborts the write and is returned as is
// (wrapped with the cleanup error when the temp file could not be removed).
func writeFileAtomicIf(dir, path, pattern string, b []byte, check func() error) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	cleanup := func(cause error) error {
		if cerr := tmp.Close(); cerr != nil {
			cause = errors.Join(cause, cerr)
		}
		if rerr := os.Remove(tmp.Name()); rerr != nil {
			cause = errors.Join(cause, rerr)
		}
		return cause
	}
	if err := tmp.Chmod(0o600); err != nil {
		return cleanup(fmt.Errorf("chmod temp file: %w", err))
	}
	if _, err := tmp.Write(b); err != nil {
		return cleanup(fmt.Errorf("write temp file: %w", err))
	}
	if err := tmp.Close(); err != nil {
		if rerr := os.Remove(tmp.Name()); rerr != nil {
			err = errors.Join(err, rerr)
		}
		return fmt.Errorf("close temp file: %w", err)
	}
	if check != nil {
		if err := check(); err != nil {
			if rerr := removeTemp(tmp.Name()); rerr != nil {
				// Not the bare sentinel: callers matching it would swallow the leftover temp file.
				return fmt.Errorf("remove temp file after %v: %w", err, rerr)
			}
			return err
		}
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		if rerr := os.Remove(tmp.Name()); rerr != nil {
			err = errors.Join(err, rerr)
		}
		return fmt.Errorf("rename %s: %w", filepath.Base(path), err)
	}
	return nil
}

// errAgentChanged aborts a MarkSeen write whose agent file changed after it was read.
var errAgentChanged = errors.New("agent file changed")

// markSeenHook runs between MarkSeen's read and write; tests use it to land a
// concurrent hook report.
var markSeenHook = func() {}

// MarkSeen flips a Done agent to Idle (the user looked at it). Other states
// and unknown sessions are left untouched. It is a compare-and-set on the
// file's mtime: when a hook report rewrites the file between the read and the
// rename, MarkSeen rereads once; if it changes again, the newer report wins and
// MarkSeen gives up without error.
func MarkSeen(home, sessionID string) error {
	for attempt := 0; attempt < 2; attempt++ {
		err := markSeenOnce(home, sessionID)
		if !errors.Is(err, errAgentChanged) {
			return err
		}
	}
	return nil
}

// markSeenOnce is one read-check-write attempt of MarkSeen.
func markSeenOnce(home, sessionID string) error {
	path := agentPath(home, sessionID)
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat agent %s: %w", sessionID, err)
	}
	a, err := readAgent(home, sessionID)
	if err != nil {
		return err
	}
	if a == nil || a.State != Done {
		return nil
	}
	a.State = Idle
	b, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode agent: %w", err)
	}
	markSeenHook()
	unchanged := func() error {
		now, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return errAgentChanged
		}
		if err != nil {
			return fmt.Errorf("stat agent %s: %w", sessionID, err)
		}
		if !os.SameFile(fi, now) || !now.ModTime().Equal(fi.ModTime()) {
			return errAgentChanged
		}
		return nil
	}
	err = writeFileAtomicIf(AgentsDir(home), path, ".agent-*.tmp", b, unchanged)
	if err != nil && !errors.Is(err, errAgentChanged) {
		return fmt.Errorf("write agent %s: %w", sessionID, err)
	}
	return err
}
