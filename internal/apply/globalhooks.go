// internal/apply/globalhooks.go
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const hookMarker = "zenify hooks-run "

// hookSpec describes one znf hook to wire into ~/.claude/settings.json.
type hookSpec struct {
	Event   string // e.g. "SessionStart", "Stop", "PreToolUse", "PostToolUse"
	Matcher string // "" for events without a matcher (SessionStart/Stop)
	ID      string // hooks-run dispatch id, e.g. "docs-sync"
}

// znfHookSpecs mirrors internal/plugin/assets/znf/hooks/hooks.json, but the
// command is the external dispatcher form `zenify hooks-run <id>`.
func znfHookSpecs() []hookSpec {
	return []hookSpec{
		{Event: "SessionStart", Matcher: "", ID: "session-start"},
		{Event: "Stop", Matcher: "", ID: "docs-sync"},
		{Event: "PreToolUse", Matcher: "Task", ID: "observe-count"},
		{Event: "PostToolUse", Matcher: "Task|Bash|WebFetch|WebSearch|Read", ID: "observe-meter"},
	}
}

func (s hookSpec) command() string { return hookMarker + s.ID }

// HookChanges reports what EnsureGlobalHooks did (or would do in dryRun).
type HookChanges struct {
	Added     int
	Updated   int
	Unchanged int
	Skipped   bool // settings.json malformed => left untouched
}

func (c HookChanges) Total() int { return c.Added + c.Updated + c.Unchanged }

// EnsureGlobalHooks merges the znf hook entries into <home>/.claude/settings.json.
// Idempotent by marker; atomic write; fail-open on malformed input.
//
// Only the "hooks" subtree is decoded and normalized — every other top-level
// key (permissions, env, model, ...) is kept as its raw json.RawMessage
// instead of being round-tripped through map[string]any. That preserves each
// foreign value's content AND its internal key order exactly as written (a map
// round-trip would alphabetize the nested keys via encoding/json's map-key sort,
// silently reordering e.g. the inside of "permissions"). The top-level keys are
// still emitted in sorted order (root is itself a map), and the file's
// whitespace/indentation is normalized on every write, same as the rest of this
// package (see mergeSettingsKeys, apply.go:226, which reformats the whole file
// by design) — so this is foreign-value content-and-order preservation, not
// byte-for-byte identity or top-level key-order preservation.
func EnsureGlobalHooks(home string, dryRun bool) (HookChanges, error) {
	path := filepath.Join(home, ".claude", "settings.json")

	root := map[string]json.RawMessage{}
	mode := os.FileMode(0o644)
	existing, readErr := os.ReadFile(path) //nolint:gosec // G304 -- path is ~/.claude/settings.json, computed from the caller's home dir, not user input
	switch {
	case readErr == nil:
		if err := json.Unmarshal(existing, &root); err != nil {
			// Malformed: do NOT touch. Caller fail-opens.
			return HookChanges{Skipped: true}, fmt.Errorf("settings.json malformed, skipping hook wiring: %w", err)
		}
		if fi, statErr := os.Stat(path); statErr == nil {
			mode = fi.Mode().Perm()
		}
	case !os.IsNotExist(readErr):
		return HookChanges{Skipped: true}, fmt.Errorf("read settings.json: %w", readErr)
	}

	hooks := map[string]any{}
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			// Non-object "hooks" (string/array/number): do NOT touch. Caller fail-opens.
			return HookChanges{Skipped: true}, fmt.Errorf("settings.json \"hooks\" is not an object, skipping hook wiring: %w", err)
		}
		if hooks == nil {
			// "hooks": null unmarshals without error but clears the map — treat the
			// same as a non-object value rather than silently replacing it.
			return HookChanges{Skipped: true}, fmt.Errorf("settings.json \"hooks\" is null, skipping hook wiring")
		}
	}

	var ch HookChanges
	for _, spec := range znfHookSpecs() {
		mergeOneHook(hooks, spec, &ch)
	}

	if dryRun {
		return ch, nil
	}
	if ch.Added == 0 && ch.Updated == 0 {
		return ch, nil // nothing to write => byte-identical (idempotent)
	}

	hooksOut, err := marshalNoEscape(hooks)
	if err != nil {
		return ch, err
	}
	root["hooks"] = json.RawMessage(bytes.TrimSpace(hooksOut))

	out, err := marshalNoEscape(root)
	if err != nil {
		return ch, err
	}
	if err := writeAtomic(path, out, mode); err != nil {
		return ch, err
	}
	return ch, nil
}

// mergeOneHook ensures one (event,matcher,command) exists exactly once.
// Returns true if it mutated the tree.
func mergeOneHook(hooks map[string]any, spec hookSpec, ch *HookChanges) bool {
	groups, _ := hooks[spec.Event].([]any)

	// Find a group matching this matcher.
	var group map[string]any
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if matcherOf(gm) == spec.Matcher {
			group = gm
			break
		}
	}
	if group == nil {
		group = map[string]any{}
		if spec.Matcher != "" {
			group["matcher"] = spec.Matcher
		}
		group["hooks"] = []any{}
		groups = append(groups, group)
		hooks[spec.Event] = groups
	}

	inner, _ := group["hooks"].([]any)
	want := spec.command()

	// Look for an existing znf-marked command in this group.
	for i, h := range inner {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := hm["command"].(string)
		if !isZnfMarked(cmd) {
			continue // foreign hook: never touch
		}
		// Same dispatch id? (compare the id token, tolerate flag drift)
		if znfID(cmd) == spec.ID {
			if cmd == want {
				ch.Unchanged++
				return false
			}
			hm["command"] = want
			hm["type"] = "command"
			inner[i] = hm
			group["hooks"] = inner
			ch.Updated++
			return true
		}
	}

	inner = append(inner, map[string]any{"type": "command", "command": want})
	group["hooks"] = inner
	ch.Added++
	return true
}

func matcherOf(group map[string]any) string {
	m, _ := group["matcher"].(string)
	return m
}

func isZnfMarked(cmd string) bool {
	return len(cmd) >= len(hookMarker) && cmd[:len(hookMarker)] == hookMarker
}

// znfID returns the dispatch id token following the marker (up to first space).
func znfID(cmd string) string {
	if !isZnfMarked(cmd) {
		return ""
	}
	rest := cmd[len(hookMarker):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == ' ' {
			return rest[:i]
		}
	}
	return rest
}

// marshalNoEscape mirrors apply.go byte-write: indent + no HTML escaping.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeAtomic writes via temp file + rename in the same dir, chmod'd to mode
// so a pre-existing file's permissions survive (os.CreateTemp defaults to
// 0600, which would otherwise silently narrow e.g. an existing 0644 file).
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
