package wt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
)

// Upsert replaces the entry with the same Slug, or appends w when none matches.
func (s *StateFile) Upsert(w Worktree) {
	for i := range s.Worktrees {
		if s.Worktrees[i].Slug == w.Slug {
			s.Worktrees[i] = w
			return
		}
	}
	s.Worktrees = append(s.Worktrees, w)
}

// Remove drops the entry with the given slug. Returns whether one was removed.
func (s *StateFile) Remove(slug string) bool {
	for i := range s.Worktrees {
		if s.Worktrees[i].Slug == slug {
			s.Worktrees = append(s.Worktrees[:i], s.Worktrees[i+1:]...)
			return true
		}
	}
	return false
}

// writeStateAtomic writes s to <dir>/state.json via a temp file in the SAME
// directory + rename, so a reader never sees a half-written file and a crash
// leaves either the old file or the new one — never a truncated one. The temp
// file shares the dir so the rename stays on one filesystem (atomic).
func writeStateAtomic(dir string, s *StateFile) error {
	if s.Version == 0 {
		s.Version = 1
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("wt: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, "state.*.tmp")
	if err != nil {
		return fmt.Errorf("wt: temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, "state.json")); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("wt: rename state.json in %s: %w", dir, err)
	}
	return nil
}

// withStateLock serializes a read-modify-write of one repo's .wt/state.json.
// The lock is a flock on <repoRoot>/.wt/.zenify.lock (via internal/lock); a
// concurrent holder makes mutate return lock.ErrHeld rather than corrupt state.
func withStateLock(repoRoot string, pid int, host string, now int64, mutate func(*StateFile) error) error {
	dir := filepath.Join(repoRoot, ".wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("wt: mkdir %s: %w", dir, err)
	}
	h, err := lock.Acquire(dir, pid, host, now)
	if err != nil {
		return err
	}
	defer h.Release()
	st, err := ReadState(repoRoot)
	if err != nil {
		return err
	}
	if err := mutate(st); err != nil {
		return err
	}
	return writeStateAtomic(dir, st)
}

// SaveWorktree upserts w into <repoRoot>/.wt/state.json under the state lock.
func SaveWorktree(repoRoot string, w Worktree, pid int, host string, now int64) error {
	return withStateLock(repoRoot, pid, host, now, func(st *StateFile) error {
		st.Upsert(w)
		return nil
	})
}

// RemoveWorktree drops slug from state under the lock; reports whether it existed.
func RemoveWorktree(repoRoot, slug string, pid int, host string, now int64) (bool, error) {
	var removed bool
	err := withStateLock(repoRoot, pid, host, now, func(st *StateFile) error {
		removed = st.Remove(slug)
		return nil
	})
	return removed, err
}
