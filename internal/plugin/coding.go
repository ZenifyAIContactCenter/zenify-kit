package plugin

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// PruneCoding removes the leg-1 coding skills an earlier `zenify skills install`
// recorded in manifestPath. Those skills ship inside the znf plugin since
// 2026-09-20, so a per-repo copy is a duplicate registry entry. Same contract
// as Sync's prune (sync.go): only recorded paths under destRoot are touched, a
// user-modified file is kept, an unrecorded file is never considered. An
// absent manifest is a no-op; an emptied manifest is deleted.
func PruneCoding(destRoot, manifestPath string) (Result, error) {
	var res Result
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return res, nil
	}
	m, err := managed.Load(manifestPath)
	if err != nil {
		return res, err
	}
	destRootClean := filepath.Clean(destRoot)
	paths := make([]string, 0, len(m.Entries))
	for p := range m.Entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, err := filepath.Rel(destRootClean, p)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue // outside destRoot: inert, never deleted, never forgotten
		}
		if existing, rerr := os.ReadFile(p); rerr == nil { //nolint:gosec // G304 -- path comes from our own manifest, checked to sit under destRoot
			if m.DecideRefresh(p, existing) == managed.DecisionKeepModified {
				res.Kept = append(res.Kept, p)
				continue
			}
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			continue // entry stays in the manifest; a later run retries it
		}
		delete(m.Entries, p)
		res.Removed = append(res.Removed, p)
		for dir := filepath.Dir(p); dir != destRootClean; dir = filepath.Dir(dir) {
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) > 0 {
				break
			}
			if err := os.Remove(dir); err != nil {
				break
			}
		}
	}
	if len(m.Entries) == 0 {
		if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
			return res, err
		}
		return res, nil
	}
	return res, m.Save(manifestPath)
}
