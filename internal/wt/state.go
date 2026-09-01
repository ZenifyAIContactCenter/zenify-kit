package wt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Worktree is one entry in .wt/state.json. Ports is the allocated set (one for a
// normal repo, a range for a monorepo); PortBase is the low end of a range.
type Worktree struct {
	Slug      string `json:"slug"`
	Type      string `json:"type"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
	Ports     []int  `json:"ports"`
	PortBase  int    `json:"portBase"`
	DevPid    *int   `json:"devPid"`
}

// StateFile is the per-repo .wt/state.json. git is the source of truth for a
// worktree's existence; this file is a rebuildable cache of metadata.
type StateFile struct {
	Version   int        `json:"version"`
	Worktrees []Worktree `json:"worktrees"`
}

// ReadState loads <repoRoot>/.wt/state.json. A missing file is NOT an error — a
// repo with no wt worktrees yet reads as an empty state.
func ReadState(repoRoot string) (*StateFile, error) {
	path := filepath.Join(repoRoot, ".wt", "state.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &StateFile{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("wt: read %s: %w", path, err)
	}
	var s StateFile
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("wt: %s is not valid JSON: %w", path, err)
	}
	return &s, nil
}

// Find returns the worktree with the given slug, or false.
func (s *StateFile) Find(slug string) (Worktree, bool) {
	for _, w := range s.Worktrees {
		if w.Slug == slug {
			return w, true
		}
	}
	return Worktree{}, false
}

// IndexPath returns $XDG_STATE_HOME/zenify/wt-index.json, falling back to
// ~/.local/state per the XDG base-dir spec.
func IndexPath() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "zenify", "wt-index.json"), nil
}

// ReadIndex loads the global index mapping repo-abs-path → []slug. A missing
// index reads as empty.
func ReadIndex() (map[string][]string, error) {
	path, err := IndexPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string][]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("wt: read %s: %w", path, err)
	}
	var m map[string][]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("wt: %s is not valid JSON: %w", path, err)
	}
	return m, nil
}
