// internal/apply/model.go
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultModel is the model the znf workflow is calibrated for. The skills,
// review doctrine, and the code-reviewer agent (znf/agents/code-reviewer.md)
// all assume Opus 4.8; onboarding pins it so a freshly-onboarded workspace runs
// the workflow on the model it was built for. It is a *default* — a per-session
// /model or a --model flag still overrides it (both outrank settings.json).
const DefaultModel = "claude-opus-4-8"

// EnsureWorkspaceModel enforces `"model": DefaultModel` in
// <workspace>/.claude/settings.json. Scoped to the workspace (not global
// ~/.claude), so it never touches a teammate's other projects. Enforced, not
// create-if-absent: it overwrites a differing model on every apply, since the
// workflow only works on DefaultModel — but it is idempotent (no write when the
// value already matches, so a re-run is byte-identical).
//
// Only the top-level "model" key is set; every other key (hooks, permissions,
// ...) is preserved as its raw json.RawMessage exactly as written, mirroring
// EnsureGlobalHooks' foreign-key handling. Atomic write; fail-open on malformed
// input (the caller warns and continues). Returns whether it changed the file.
func EnsureWorkspaceModel(workspace string, dryRun bool) (changed bool, err error) {
	path := filepath.Join(workspace, ".claude", "settings.json")

	root := map[string]json.RawMessage{}
	mode := os.FileMode(0o644)
	existing, readErr := os.ReadFile(path) //nolint:gosec // G304 -- path is <workspace>/.claude/settings.json, computed from the workspace root, not user input
	switch {
	case readErr == nil:
		if err := json.Unmarshal(existing, &root); err != nil {
			// Malformed: do NOT touch. Caller fail-opens.
			return false, fmt.Errorf("settings.json malformed, skipping model pin: %w", err)
		}
		if fi, statErr := os.Stat(path); statErr == nil {
			mode = fi.Mode().Perm()
		}
	case !os.IsNotExist(readErr):
		return false, fmt.Errorf("read settings.json: %w", readErr)
	}

	want, err := json.Marshal(DefaultModel) // `"claude-opus-4-8"`
	if err != nil {
		return false, err
	}
	if cur, ok := root["model"]; ok && bytes.Equal(bytes.TrimSpace(cur), want) {
		return false, nil // already pinned — idempotent, no write
	}

	if dryRun {
		return true, nil
	}

	root["model"] = json.RawMessage(want)
	out, err := marshalNoEscape(root)
	if err != nil {
		return false, err
	}
	if err := writeAtomic(path, out, mode); err != nil {
		return false, err
	}
	return true, nil
}
