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
	Purpose string // short Vietnamese description of what the hook does //znf:allow-lang
}

// znfHookSpecs is the single source of truth for the hooks the kit wires into
// ~/.claude/settings.json (the command is the dispatcher form
// `zenify hooks-run <id>`). PreToolUse/PostToolUse matchers name the harness
// tool names: the subagent tool was `Task` and is `Agent` in current builds,
// so every subagent-scoped matcher (observe-count AND observe-meter) names both.
// read-guard matches only Read and write-guard matches Write|Edit|NotebookEdit:
// they are the only hooks allowed to deny (exit 2).
func znfHookSpecs() []hookSpec {
	return []hookSpec{
		{Event: "SessionStart", Matcher: "", ID: "session-start", Purpose: "Đồng bộ knowledge store, ghi lại rules/workspace mới nhất, nhắc update, in digest bootstrap nếu máy chưa có sentinel discipline"},                   //znf:allow-lang
		{Event: "SessionStart", Matcher: "", ID: "git-state", Purpose: "In báo cáo trạng thái git của workspace vào context đầu phiên"},                                                                                         //znf:allow-lang
		{Event: "SessionStart", Matcher: "", ID: "wt-report", Purpose: "In một dòng liệt kê worktree đã merge/rác mà `wt sweep --all` sẽ dọn"},                                                                                  //znf:allow-lang
		{Event: "Stop", Matcher: "", ID: "docs-sync", Purpose: "Đồng bộ knowledge store qua git (status → pull --rebase → push), fail-open"},                                                                                    //znf:allow-lang
		{Event: "Stop", Matcher: "", ID: "git-state-stop", Purpose: "Báo trạng thái git dạng cảnh báo đứng (systemMessage) khi phiên dừng"},                                                                                     //znf:allow-lang
		{Event: "PreToolUse", Matcher: "Task|Agent", ID: "observe-count", Purpose: "Đếm số lần dispatch subagent, cảnh báo khi vượt soft-cap"},                                                                                  //znf:allow-lang
		{Event: "PostToolUse", Matcher: "Task|Agent|Bash|WebFetch|WebSearch|Read", ID: "observe-meter", Purpose: "Ghi nhận việc dùng tool để đo usage"},                                                                         //znf:allow-lang
		{Event: "PreToolUse", Matcher: "Read", ID: "read-guard", Purpose: "Chặn Read file quá lớn (text > 200 KB không có offset+limit, PDF không có pages, ảnh > 300 KB) trước khi kết quả vào context"},                       //znf:allow-lang
		{Event: "PreToolUse", Matcher: "Write|Edit|NotebookEdit", ID: "write-guard", Purpose: "Chặn Write/Edit ra ngoài quy ước thư mục workspace (root ngoài allowlist, file ngay dưới docs/, docs/<tên lạ>) và chỉ chỗ đúng"}, //znf:allow-lang
	}
}

func (s hookSpec) command() string { return hookMarker + s.ID }

// HookSpec is the exported view of hookSpec for read-only consumers (docsgen).
type HookSpec = hookSpec

// HookSpecs returns the hooks the kit wires into ~/.claude/settings.json.
func HookSpecs() []HookSpec { return znfHookSpecs() }

// codexSubagentTool is the tool name Codex puts in PreToolUse/PostToolUse
// payloads when it spawns a subagent (measured in experiment E11, Codex 0.155.1).
// internal/cli/observe.go keeps a copy of this literal; keep them in sync.
const codexSubagentTool = "collaborationspawn_agent"

// codexEntries derives the Codex hook specs from the Claude ones: read-guard and
// write-guard are dropped (Codex has no matching Read/Write/Edit tools) and
// Claude subagent tool names in matchers are replaced by codexSubagentTool.
// ID and Purpose are kept.
func codexEntries(specs []hookSpec) []hookSpec {
	out := make([]hookSpec, 0, len(specs))
	for _, s := range specs {
		switch {
		case s.ID == "read-guard" || s.ID == "write-guard":
			continue
		case s.Matcher == "Task|Agent":
			s.Matcher = codexSubagentTool
		case s.Matcher == "Task|Agent|Bash|WebFetch|WebSearch|Read":
			s.Matcher = codexSubagentTool + "|Bash"
		}
		out = append(out, s)
	}
	return out
}

// CodexHookSpecs returns the hooks the kit wires into Codex (read-only view for docsgen).
func CodexHookSpecs() []HookSpec { return codexEntries(znfHookSpecs()) }

// Runtimes names the runtimes this hook is installed for.
func (s hookSpec) Runtimes() string {
	for _, c := range codexEntries([]hookSpec{s}) {
		if c.ID == s.ID {
			return "Claude, Codex"
		}
	}
	return "Claude"
}

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
// package (see writeEnv in subagentmodel.go, which reformats the whole file
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

// mergeOneHook ensures spec's command sits in the group whose matcher equals
// spec.Matcher. Before that, any znf entry with the same ID living under a
// DIFFERENT matcher of the same event is removed (matcher migration, W0
// FR-02.2); a group emptied that way is dropped. Foreign hooks are never
// touched. A migration counts once, as Updated. Returns true when anything
// changed.
func mergeOneHook(hooks map[string]any, spec hookSpec, ch *HookChanges) bool {
	return mergeCommand(hooks, spec.Event, spec.Matcher, spec.command(), func(cmd string) bool {
		return isZnfMarked(cmd) && znfID(cmd) == spec.ID
	}, ch)
}

// mergeCommand is mergeOneHook generalized: want is the command to ensure under
// event/matcher, and isSame reports whether an existing command is the same
// logical hook as want (so a drifted command is updated in place and a copy under
// another matcher is migrated). Foreign commands never satisfy isSame.
func mergeCommand(hooks map[string]any, event, matcher, want string, isSame func(cmd string) bool, ch *HookChanges) bool {
	groups, _ := hooks[event].([]any)

	groups, migrated := pruneStaleMatcherGroups(groups, matcher, isSame)
	if migrated {
		hooks[event] = groups
	}

	// Find a group matching this matcher.
	var group map[string]any
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if matcherOf(gm) == matcher {
			group = gm
			break
		}
	}
	if group == nil {
		group = map[string]any{}
		if matcher != "" {
			group["matcher"] = matcher
		}
		group["hooks"] = []any{}
		groups = append(groups, group)
		hooks[event] = groups
	}

	inner, _ := group["hooks"].([]any)

	// Look for an existing znf-marked command in this group.
	for i, h := range inner {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := hm["command"].(string)
		// Same hook? (hooks-run: compare the id token, tolerate flag drift)
		if isSame(cmd) {
			if cmd == want {
				if migrated {
					ch.Updated++
					return true
				}
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
	if migrated {
		ch.Updated++
	} else {
		ch.Added++
	}
	return true
}

// pruneStaleMatcherGroups removes the entry isSame recognizes from every group of
// the event whose matcher differs from matcher. Groups left empty by that
// removal are dropped; groups that were already empty, and every foreign
// hook, are left exactly as found. Reports whether anything was removed.
func pruneStaleMatcherGroups(groups []any, matcher string, isSame func(cmd string) bool) ([]any, bool) {
	out := make([]any, 0, len(groups))
	removed := false
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok || matcherOf(gm) == matcher {
			out = append(out, g)
			continue
		}
		inner, _ := gm["hooks"].([]any)
		kept := make([]any, 0, len(inner))
		for _, h := range inner {
			if hm, ok := h.(map[string]any); ok {
				cmd, _ := hm["command"].(string)
				if isSame(cmd) {
					removed = true
					continue
				}
			}
			kept = append(kept, h)
		}
		if len(kept) != len(inner) {
			if len(kept) == 0 {
				continue // group emptied by the migration → drop it
			}
			gm["hooks"] = kept
		}
		out = append(out, gm)
	}
	return out, removed
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

// codexGuardCommand is the git-guard hook command; it mirrors guardCommand in
// internal/cli/guard.go (apply cannot import cli) — keep them in sync.
const codexGuardCommand = "zenify git-guard"

// isCodexKitCommand reports whether cmd is a hook the kit wires into Codex:
// a hooks-run dispatcher entry or the git-guard entry.
func isCodexKitCommand(cmd string) bool {
	return isZnfMarked(cmd) || cmd == codexGuardCommand
}

// EnsureCodexHooks merges the Codex hook entries (codexEntries plus a git-guard
// PreToolUse/Bash entry) into <codexHome>/hooks.json. Same merge rules as
// EnsureGlobalHooks: idempotent, atomic, foreign keys/entries preserved,
// fail-open (malformed → Skipped, file untouched). codexHome is created 0o700
// when missing; dryRun writes nothing, not even the directory.
func EnsureCodexHooks(codexHome string, dryRun bool) (HookChanges, error) {
	path := filepath.Join(codexHome, "hooks.json")

	root := map[string]json.RawMessage{}
	mode := os.FileMode(0o644)
	existing, readErr := os.ReadFile(path) //nolint:gosec // G304 -- path is <codexHome>/hooks.json, computed from the caller's codex home, not user input
	switch {
	case readErr == nil:
		if err := json.Unmarshal(existing, &root); err != nil {
			return HookChanges{Skipped: true}, fmt.Errorf("hooks.json malformed, skipping hook wiring: %w", err)
		}
		if fi, statErr := os.Stat(path); statErr == nil {
			mode = fi.Mode().Perm()
		}
	case !os.IsNotExist(readErr):
		return HookChanges{Skipped: true}, fmt.Errorf("read hooks.json: %w", readErr)
	}

	hooks := map[string]any{}
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return HookChanges{Skipped: true}, fmt.Errorf("hooks.json \"hooks\" is not an object, skipping hook wiring: %w", err)
		}
		if hooks == nil {
			return HookChanges{Skipped: true}, fmt.Errorf("hooks.json \"hooks\" is null, skipping hook wiring")
		}
	}

	var ch HookChanges
	for _, spec := range codexEntries(znfHookSpecs()) {
		mergeOneHook(hooks, spec, &ch)
	}
	mergeCommand(hooks, "PreToolUse", "Bash", codexGuardCommand, func(cmd string) bool { return cmd == codexGuardCommand }, &ch)

	if dryRun || (ch.Added == 0 && ch.Updated == 0) {
		return ch, nil
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
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return ch, err
	}
	if err := writeAtomic(path, out, mode); err != nil {
		return ch, err
	}
	return ch, nil
}

// CountCodexKitHooks counts the kit-wired hook commands in <codexHome>/hooks.json
// (same recognition as RemoveCodexHooks). Missing file → 0, nil.
func CountCodexKitHooks(codexHome string) (int, error) {
	b, err := os.ReadFile(filepath.Join(codexHome, "hooks.json")) //nolint:gosec // G304 -- path computed from codexHome
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var root struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		return 0, err
	}
	n := 0
	for _, groups := range root.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if isCodexKitCommand(h.Command) {
					n++
				}
			}
		}
	}
	return n, nil
}
