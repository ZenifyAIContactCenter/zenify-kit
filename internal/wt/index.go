package wt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
)

// withIndexLock serializes read-modify-write of the global XDG index. The lock
// lives in the index's own directory so writes from two different repos cannot
// race. A concurrent holder returns lock.ErrHeld rather than corrupting it.
func withIndexLock(pid int, host string, now int64, mutate func(map[string][]string)) error {
	p, err := IndexPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("wt: mkdir %s: %w", dir, err)
	}
	h, err := lock.Acquire(dir, pid, host, now)
	if err != nil {
		return err
	}
	defer h.Release()
	idx, err := ReadIndex()
	if err != nil {
		return err
	}
	mutate(idx)
	return writeIndexAtomic(p, idx)
}

// writeIndexAtomic writes the index via temp-file + rename in the same dir.
func writeIndexAtomic(path string, idx map[string][]string) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "wt-index.*.tmp")
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
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("wt: rename %s: %w", path, err)
	}
	return nil
}

// IndexUpsert adds slug to repoRoot's entry in the global index, idempotently.
func IndexUpsert(repoRoot, slug string, pid int, host string, now int64) error {
	return withIndexLock(pid, host, now, func(idx map[string][]string) {
		for _, s := range idx[repoRoot] {
			if s == slug {
				return
			}
		}
		idx[repoRoot] = append(idx[repoRoot], slug)
	})
}

// IndexRemove drops slug from repoRoot's entry; removes the repo key when empty.
func IndexRemove(repoRoot, slug string, pid int, host string, now int64) error {
	return withIndexLock(pid, host, now, func(idx map[string][]string) {
		cur := idx[repoRoot]
		out := cur[:0:0]
		for _, s := range cur {
			if s != slug {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			delete(idx, repoRoot)
			return
		}
		idx[repoRoot] = out
	})
}
