// internal/apply/subagentmodel.go
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SubagentModelEnv is the Claude Code env var that sets the default model for
// every subagent whose dispatch omits `model`. Frontmatter and an explicit
// `model` on the dispatch still win, so this is a floor for the forgotten case,
// not a cap: the review engine keeps passing `opus` where a tier demands it.
const SubagentModelEnv = "CLAUDE_CODE_SUBAGENT_MODEL"

// SubagentModelDefault is the value the kit writes. Measured 2026-09-18: 19% of
// dispatches omitted `model` and inherited the Opus main session.
const SubagentModelDefault = "sonnet"

// EnsureSubagentModelEnv sets env.CLAUDE_CODE_SUBAGENT_MODEL in
// <home>/.claude/settings.json — only when the key is absent, so a teammate's
// own choice is never overwritten. Every other key, inside "env" and at the
// top level, is preserved as raw JSON (same contract as EnsureGlobalHooks).
// Atomic write; fail-open on malformed input. Returns whether it wrote.
func EnsureSubagentModelEnv(home string, dryRun bool) (bool, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return false, err
	}
	env, err := envOf(root)
	if err != nil {
		return false, err
	}
	if _, ok := env[SubagentModelEnv]; ok {
		return false, nil // user (or an earlier run) already decided
	}
	if dryRun {
		return true, nil
	}
	val, _ := json.Marshal(SubagentModelDefault)
	env[SubagentModelEnv] = json.RawMessage(val)
	return true, writeEnv(path, root, env, mode)
}

// RemoveSubagentModelEnv reverses EnsureSubagentModelEnv: it removes the key
// only when its value is still the kit default, so a value the user changed
// stays. An "env" object left empty is dropped. Returns whether it removed.
func RemoveSubagentModelEnv(home string, dryRun bool) (bool, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return false, err
	}
	if root == nil {
		return false, nil // no settings file → nothing to remove
	}
	env, err := envOf(root)
	if err != nil {
		return false, err
	}
	cur, ok := env[SubagentModelEnv]
	if !ok {
		return false, nil
	}
	want, _ := json.Marshal(SubagentModelDefault)
	if !bytes.Equal(bytes.TrimSpace(cur), want) {
		return false, nil // user changed it → theirs now
	}
	if dryRun {
		return true, nil
	}
	delete(env, SubagentModelEnv)
	return true, writeEnv(path, root, env, mode)
}

// readSettingsRoot decodes the top level of a settings file into raw messages.
// A missing file yields (nil, 0644, nil) so callers can distinguish "absent"
// from "empty"; a malformed file is an error (fail-open at the caller).
func readSettingsRoot(path string) (map[string]json.RawMessage, os.FileMode, error) {
	mode := os.FileMode(0o644)
	existing, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed ~/.claude path, computed from the caller's home dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, mode, nil
		}
		return nil, mode, fmt.Errorf("read settings.json: %w", err)
	}
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, mode, fmt.Errorf("settings.json malformed, skipping env: %w", err)
	}
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	return root, mode, nil
}

// envOf decodes root["env"] one level, keeping each value raw. A non-object
// "env" is an error so the caller leaves the file alone.
func envOf(root map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	env := map[string]json.RawMessage{}
	raw, ok := root["env"]
	if !ok {
		return env, nil
	}
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		return nil, fmt.Errorf("settings.json \"env\" is not an object, skipping env")
	}
	return env, nil
}

func writeEnv(path string, root map[string]json.RawMessage, env map[string]json.RawMessage, mode os.FileMode) error {
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	if len(env) == 0 {
		delete(root, "env")
	} else {
		envOut, err := marshalNoEscape(env)
		if err != nil {
			return err
		}
		root["env"] = json.RawMessage(bytes.TrimSpace(envOut))
	}
	out, err := marshalNoEscape(root)
	if err != nil {
		return err
	}
	return writeAtomic(path, out, mode)
}
