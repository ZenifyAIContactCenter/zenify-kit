package plugin

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

//go:embed all:assets/coding
var codingAssets embed.FS

const codingRoot = "assets/coding"

// CodingSkills lists the skill directory names under assets/coding (dynamic, unregistered).
func CodingSkills() []string {
	entries, err := fs.ReadDir(codingAssets, codingRoot)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// InstallCoding materializes ONLY the skills named in `skills` from assets/coding
// into destRoot, additive/refresh-safe (like Sync). Skips names that don't exist.
func InstallCoding(destRoot, manifestPath string, skills []string) (Result, error) {
	var res Result
	want := map[string]bool{}
	for _, s := range skills {
		want[s] = true
	}
	m, err := managed.Load(manifestPath)
	if err != nil {
		return res, err
	}
	err = fs.WalkDir(codingAssets, codingRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, codingRoot+"/")
		top := rel
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			top = rel[:i]
		}
		if !want[top] {
			return nil
		}
		target := filepath.Join(destRoot, rel)
		content, err := codingAssets.ReadFile(p)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil { //nolint:gosec // G304
			switch m.DecideRefresh(target, existing) {
			case managed.DecisionKeepModified, managed.DecisionKeepUserAdded:
				res.Kept = append(res.Kept, target)
				return nil
			case managed.DecisionUpdate:
				if managed.Fingerprint(existing) == managed.Fingerprint(content) {
					res.Skipped = append(res.Skipped, target)
					return nil
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
		if err := m.Record(target); err != nil {
			return err
		}
		res.Written = append(res.Written, target)
		return nil
	})
	if err != nil {
		return res, err
	}
	if err := m.Save(manifestPath); err != nil {
		return res, err
	}
	return res, nil
}

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
			return res, err
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
