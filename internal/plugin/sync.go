// Package plugin embeds the `znf` plugin tree and materializes it into ~/.claude/skills/znf,
// tracked via a dedicated managed.Manifest so refresh respects the user's manual edits.
package plugin

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

//go:embed all:assets/znf
var assets embed.FS

const embedRoot = "assets/znf"

type Result struct {
	Written []string
	Kept    []string
	Skipped []string
	Removed []string // recorded files the embed no longer ships (pruned)
}

func DefaultDest() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "skills", "znf"), nil
}

func DefaultManifest() (string, error) {
	dest, err := DefaultDest()
	if err != nil {
		return "", err
	}
	return filepath.Join(dest, ".manifest.json"), nil
}

// Sync writes every file in the embed out to destRoot, recording it in the manifest at manifestPath.
// Writes and prunes only WITHIN destRoot; prune touches recorded files only. Refresh-safe via managed.DecideRefresh.
func Sync(destRoot, manifestPath string) (Result, error) {
	var res Result
	m, err := managed.Load(manifestPath)
	if err != nil {
		return res, err
	}
	present := map[string]bool{}
	err = fs.WalkDir(assets, embedRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, embedRoot+"/")
		target := filepath.Join(destRoot, rel)
		present[target] = true
		content, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil { //nolint:gosec // G304 -- target computed from destRoot internally
			switch m.DecideRefresh(target, existing) {
			case managed.DecisionKeepModified:
				res.Kept = append(res.Kept, target)
				return nil
			case managed.DecisionUpdate:
				if managed.Fingerprint(existing) == managed.Fingerprint(content) {
					res.Skipped = append(res.Skipped, target)
					return nil
				}
			case managed.DecisionKeepUserAdded:
				// file exists at target but was not recorded by us → the user placed it in znf/ themselves;
				// additive (FR-M2A-02): KEEP it as-is, don't overwrite.
				res.Kept = append(res.Kept, target)
				return nil
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
	// Prune (W0 FR-06.4): a file we recorded in an earlier sync that the embed
	// no longer contains is dead — remove it and forget it. Only paths under
	// destRoot are considered, and only recorded ones: a user-placed file is
	// never recorded, so it is never touched.
	destRootClean := filepath.Clean(destRoot)
	for path := range m.Entries {
		if present[path] {
			continue
		}
		rel, err := filepath.Rel(destRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			// Outside destRoot: leave the entry inert rather than deleting the
			// record for a file we refuse to touch — never repaired, but also
			// never silently forgotten.
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			continue
		}
		delete(m.Entries, path)
		res.Removed = append(res.Removed, path)
		// A prune can leave an empty parent directory behind (e.g. hooks/ after
		// its last file goes) — clean those up too, never above destRoot.
		for dir := filepath.Dir(path); dir != destRootClean; dir = filepath.Dir(dir) {
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) > 0 {
				break
			}
			if err := os.Remove(dir); err != nil {
				break
			}
		}
	}
	sort.Strings(res.Removed)
	m.Version = version.Current()
	if err := m.Save(manifestPath); err != nil {
		return res, err
	}
	return res, nil
}
