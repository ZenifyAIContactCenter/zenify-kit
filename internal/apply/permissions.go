// internal/apply/permissions.go
package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KitAllowRules are the tool rules the kit unions into permissions.allow of
// ~/.claude/settings.json so Playwright UI-verify and `zenify e2e|visual`
// run without permission prompts. No server-wide rule and no
// browser_run_code_unsafe on purpose.
var KitAllowRules = []string{
	"mcp__playwright__browser_navigate",
	"mcp__playwright__browser_snapshot",
	"mcp__playwright__browser_take_screenshot",
	"mcp__playwright__browser_click",
	"mcp__playwright__browser_type",
	"mcp__playwright__browser_evaluate",
	"mcp__playwright__browser_console_messages",
	"mcp__playwright__browser_wait_for",
	"mcp__playwright__browser_set_storage_state",
	"Bash(zenify e2e *)",
	"Bash(zenify visual *)",
	"Bash(zenify ui-verify *)",
}

// KitSandboxExcluded are the commands the kit unions into
// sandbox.excludedCommands: `zenify e2e login` writes ~/.zenify and reaches the
// local web app, both of which the Bash sandbox denies, so without this the
// verifier would have to ask for the sandbox to be disabled. Only login: other
// e2e subcommands run repo journey code and stay sandboxed. Inert when the
// sandbox is off.
var KitSandboxExcluded = []string{
	"zenify e2e login *",
}

// kitList is one settings array the kit unions entries into, with the record
// of what it added so removal never touches an entry the user already had.
type kitList struct {
	outer, inner string
	rules        []string
	record       func(home string) string
}

var (
	allowList   = kitList{"permissions", "allow", KitAllowRules, kitPermissionsRecord}
	sandboxList = kitList{"sandbox", "excludedCommands", KitSandboxExcluded, kitSandboxRecord}
)

func kitPermissionsRecord(home string) string {
	return filepath.Join(home, ".zenify", "playwright", "kit-permissions.json")
}

func kitSandboxRecord(home string) string {
	return filepath.Join(home, ".zenify", "playwright", "kit-sandbox.json")
}

// listOf decodes root[l.outer] one level and its l.inner array. A non-object
// outer or non-array inner is an error (caller leaves the file alone).
func (l kitList) listOf(root map[string]json.RawMessage) (map[string]json.RawMessage, []string, error) {
	obj := map[string]json.RawMessage{}
	if raw, ok := root[l.outer]; ok {
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return nil, nil, fmt.Errorf("settings.json %q is not an object, skipping %s", l.outer, l.outer)
		}
	}
	var list []string
	if raw, ok := obj[l.inner]; ok {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, nil, fmt.Errorf("settings.json \"%s.%s\" is not a string array, skipping %s", l.outer, l.inner, l.outer)
		}
	}
	return obj, list, nil
}

func (l kitList) write(path string, root, obj map[string]json.RawMessage, list []string, mode os.FileMode) error {
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	if list == nil {
		list = []string{}
	}
	a, err := json.Marshal(list)
	if err != nil {
		return err
	}
	obj[l.inner] = a
	p, err := marshalNoEscape(obj)
	if err != nil {
		return err
	}
	root[l.outer] = json.RawMessage(p)
	out, err := marshalNoEscape(root)
	if err != nil {
		return err
	}
	return writeAtomic(path, out, mode)
}

func (l kitList) readRecord(home string) ([]string, error) {
	raw, err := os.ReadFile(l.record(home)) //nolint:gosec // G304 -- fixed path under the caller's home dir
	if err != nil {
		return nil, err
	}
	var rules []string
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("%s record malformed: %w", filepath.Base(l.record(home)), err)
	}
	return rules, nil
}

// EnsureKitPermissions unions KitAllowRules into permissions.allow, keeping
// existing entries and order, and records the rules it added. Returns the
// number added; nothing is written when it is 0 or dryRun.
func EnsureKitPermissions(home string, dryRun bool) (int, error) {
	return allowList.ensure(home, dryRun)
}

// EnsureKitSandbox unions KitSandboxExcluded into sandbox.excludedCommands,
// with the same record and user-removal rules as EnsureKitPermissions.
func EnsureKitSandbox(home string, dryRun bool) (int, error) {
	return sandboxList.ensure(home, dryRun)
}

// RemoveKitPermissions removes only the rules the kit recorded adding.
// Without a record it removes nothing and returns an error.
func RemoveKitPermissions(home string, dryRun bool) (int, error) {
	return allowList.remove(home, dryRun)
}

// RemoveKitSandbox removes only the excluded commands the kit recorded adding.
func RemoveKitSandbox(home string, dryRun bool) (int, error) {
	return sandboxList.remove(home, dryRun)
}

func (l kitList) ensure(home string, dryRun bool) (int, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return 0, err
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	obj, list, err := l.listOf(root)
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, a := range list {
		have[a] = true
	}
	prev, err := l.readRecord(home)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	// An entry already in the record but absent from the list was removed by
	// the user on purpose: never re-add it.
	recorded := map[string]bool{}
	for _, r := range prev {
		recorded[r] = true
	}
	var added []string
	for _, r := range l.rules {
		if !have[r] && !recorded[r] {
			added = append(added, r)
		}
	}
	if len(added) == 0 || dryRun {
		return len(added), nil
	}
	// Record first so a settings change is never unrecorded (the kit could
	// never remove it). If the settings write then fails, restore the previous
	// record: a leftover record would make the next run treat every new entry
	// as user-removed and never add it.
	seen := map[string]bool{}
	var rec []string
	for _, r := range append(prev, added...) {
		if !seen[r] {
			seen[r] = true
			rec = append(rec, r)
		}
	}
	recJSON, err := json.Marshal(rec)
	if err != nil {
		return 0, err
	}
	if err := writeAtomic(l.record(home), recJSON, 0o644); err != nil {
		return 0, err
	}
	if err := l.write(path, root, obj, append(list, added...), mode); err != nil {
		if prev == nil {
			_ = os.Remove(l.record(home))
		} else if pj, merr := json.Marshal(prev); merr == nil {
			_ = writeAtomic(l.record(home), pj, 0o644)
		}
		return 0, err
	}
	return len(added), nil
}

func (l kitList) remove(home string, dryRun bool) (int, error) {
	rec, err := l.readRecord(home)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("no %s record", strings.TrimSuffix(filepath.Base(l.record(home)), ".json"))
		}
		return 0, err
	}
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return 0, err
	}
	if root == nil {
		return 0, nil
	}
	obj, list, err := l.listOf(root)
	if err != nil {
		return 0, err
	}
	drop := map[string]bool{}
	for _, r := range rec {
		drop[r] = true
	}
	kept := []string{}
	removed := 0
	for _, a := range list {
		if drop[a] {
			removed++
			continue
		}
		kept = append(kept, a)
	}
	if dryRun {
		return removed, nil
	}
	if removed > 0 {
		if err := l.write(path, root, obj, kept, mode); err != nil {
			return 0, err
		}
	}
	_ = os.Remove(l.record(home))
	return removed, nil
}
