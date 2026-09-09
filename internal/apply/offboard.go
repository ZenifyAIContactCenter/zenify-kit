// internal/apply/offboard.go
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// HookRemoval describes the outcome of removing znf hooks.
type HookRemoval struct {
	Removed int
	Skipped bool // settings.json malformed → left untouched
}

// RemoveGlobalHooks removes every hook entry zenify wired (command starting with
// hookMarker) from <home>/.claude/settings.json. Reverses EnsureGlobalHooks:
// keeps every foreign hook and every other top-level key intact; a group/event
// left empty after removal is dropped. Atomic write; fail-open (malformed →
// Skipped, no edit). In dryRun it only counts.
func RemoveGlobalHooks(home string, dryRun bool) (HookRemoval, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	existing, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed ~/.claude path, not externally-tainted
	if err != nil {
		if os.IsNotExist(err) {
			return HookRemoval{}, nil // nothing to remove
		}
		return HookRemoval{Skipped: true}, fmt.Errorf("read settings.json: %w", err)
	}
	var mode os.FileMode = 0o644
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}

	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(existing, &root); err != nil {
		return HookRemoval{Skipped: true}, fmt.Errorf("settings.json malformed, skipping: %w", err)
	}
	raw, ok := root["hooks"]
	if !ok {
		return HookRemoval{}, nil
	}
	hooks := map[string]any{}
	if err := json.Unmarshal(raw, &hooks); err != nil || hooks == nil {
		return HookRemoval{Skipped: true}, fmt.Errorf("settings.json \"hooks\" not an object, skipping")
	}

	removed := 0
	for event, v := range hooks {
		groups, _ := v.([]any)
		newGroups := make([]any, 0, len(groups))
		for _, g := range groups {
			gm, ok := g.(map[string]any)
			if !ok {
				newGroups = append(newGroups, g)
				continue
			}
			inner, _ := gm["hooks"].([]any)
			kept := make([]any, 0, len(inner))
			for _, h := range inner {
				hm, ok := h.(map[string]any)
				if !ok {
					kept = append(kept, h)
					continue
				}
				cmd, _ := hm["command"].(string)
				if isZnfMarked(cmd) {
					removed++
					continue // remove
				}
				kept = append(kept, h)
			}
			if len(kept) == 0 {
				continue // empty group → drop
			}
			gm["hooks"] = kept
			newGroups = append(newGroups, gm)
		}
		if len(newGroups) == 0 {
			delete(hooks, event) // empty event → drop
		} else {
			hooks[event] = newGroups
		}
	}

	if dryRun || removed == 0 {
		return HookRemoval{Removed: removed}, nil
	}

	hooksOut, err := marshalNoEscape(hooks)
	if err != nil {
		return HookRemoval{Removed: removed}, err
	}
	root["hooks"] = json.RawMessage(bytes.TrimSpace(hooksOut))
	out, err := marshalNoEscape(root)
	if err != nil {
		return HookRemoval{Removed: removed}, err
	}
	if err := writeAtomic(path, out, mode); err != nil {
		return HookRemoval{Removed: removed}, err
	}
	return HookRemoval{Removed: removed}, nil
}

// RemoveExclude removes exactly the ".worktrees/" line from the repo's
// .git/info/exclude, keeping every other line. Returns (true) if the line was
// present and (was) removed. File without the line → (false, nil). A file left
// empty after removal is still kept (the git-local file itself is never
// deleted). dryRun → no write.
func RemoveExclude(repoDir string, dryRun bool) (bool, error) {
	excl := filepath.Join(repoDir, ".git", "info", "exclude")
	b, err := os.ReadFile(excl) //nolint:gosec // G304 -- path computed from repoDir, not externally-tainted
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var mode os.FileMode = 0o644
	if fi, statErr := os.Stat(excl); statErr == nil {
		mode = fi.Mode().Perm()
	}
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	removed := false
	for _, ln := range lines {
		if strings.TrimSpace(ln) == ".worktrees/" {
			removed = true
			continue
		}
		out = append(out, ln)
	}
	if !removed || dryRun {
		return removed, nil
	}
	if err := writeAtomic(excl, []byte(strings.Join(out, "\n")), mode); err != nil {
		return removed, err
	}
	return removed, nil
}

// RemoveOwnedSettings uses DecideRefresh to decide each owned settings.local.json:
// unchanged (fingerprint matches) → remove; modified (user edited it, e.g. a
// secret) → keep; entry absent/file missing → skip. dryRun → no actual removal.
func RemoveOwnedSettings(settingsPath string, owned *managed.Manifest, dryRun bool) (string, error) {
	if _, ok := owned.Get(settingsPath); !ok {
		return "skipped (not owned)", nil
	}
	b, err := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path from ownership manifest, not externally-tainted
	if err != nil {
		if os.IsNotExist(err) {
			return "skipped (gone)", nil
		}
		return "", err
	}
	switch owned.DecideRefresh(settingsPath, b) {
	case managed.DecisionUpdate:
		if dryRun {
			return "removed", nil
		}
		if err := os.Remove(settingsPath); err != nil {
			return "", err
		}
		return "removed", nil
	default: // DecisionKeepModified / DecisionKeepUserAdded
		return "kept (modified)", nil
	}
}
