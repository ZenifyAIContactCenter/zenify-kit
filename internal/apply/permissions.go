// internal/apply/permissions.go
package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

// kitPermissionsRecord lists the rules the kit actually added, so removal
// never touches a rule the user already had.
func kitPermissionsRecord(home string) string {
	return filepath.Join(home, ".zenify", "playwright", "kit-permissions.json")
}

// allowOf decodes root["permissions"] one level and its "allow" array. A
// non-object permissions or non-array allow is an error (caller leaves the
// file alone).
func allowOf(root map[string]json.RawMessage) (map[string]json.RawMessage, []string, error) {
	perms := map[string]json.RawMessage{}
	if raw, ok := root["permissions"]; ok {
		if err := json.Unmarshal(raw, &perms); err != nil || perms == nil {
			return nil, nil, errors.New("settings.json \"permissions\" is not an object, skipping permissions")
		}
	}
	var allow []string
	if raw, ok := perms["allow"]; ok {
		if err := json.Unmarshal(raw, &allow); err != nil {
			return nil, nil, errors.New("settings.json \"permissions.allow\" is not a string array, skipping permissions")
		}
	}
	return perms, allow, nil
}

func writeAllow(path string, root, perms map[string]json.RawMessage, allow []string, mode os.FileMode) error {
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	if allow == nil {
		allow = []string{}
	}
	a, err := json.Marshal(allow)
	if err != nil {
		return err
	}
	perms["allow"] = a
	p, err := marshalNoEscape(perms)
	if err != nil {
		return err
	}
	root["permissions"] = json.RawMessage(p)
	out, err := marshalNoEscape(root)
	if err != nil {
		return err
	}
	return writeAtomic(path, out, mode)
}

func readRecord(home string) ([]string, error) {
	raw, err := os.ReadFile(kitPermissionsRecord(home)) //nolint:gosec // G304 -- fixed path under the caller's home dir
	if err != nil {
		return nil, err
	}
	var rules []string
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("kit-permissions record malformed: %w", err)
	}
	return rules, nil
}

// EnsureKitPermissions unions KitAllowRules into permissions.allow, keeping
// existing entries and order, and records the rules it added. Returns the
// number added; nothing is written when it is 0 or dryRun.
func EnsureKitPermissions(home string, dryRun bool) (int, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return 0, err
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	perms, allow, err := allowOf(root)
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, a := range allow {
		have[a] = true
	}
	var added []string
	for _, r := range KitAllowRules {
		if !have[r] {
			added = append(added, r)
		}
	}
	if len(added) == 0 || dryRun {
		return len(added), nil
	}
	// Record first: a record without a settings change is harmless, the
	// reverse would leave rules the kit can never remove.
	prev, _ := readRecord(home)
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
	if err := writeAtomic(kitPermissionsRecord(home), recJSON, 0o644); err != nil {
		return 0, err
	}
	return len(added), writeAllow(path, root, perms, append(allow, added...), mode)
}

// RemoveKitPermissions removes only the rules the kit recorded adding.
// Without a record it removes nothing and returns an error.
func RemoveKitPermissions(home string, dryRun bool) (int, error) {
	rec, err := readRecord(home)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, errors.New("no kit-permissions record")
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
	perms, allow, err := allowOf(root)
	if err != nil {
		return 0, err
	}
	drop := map[string]bool{}
	for _, r := range rec {
		drop[r] = true
	}
	kept := []string{}
	removed := 0
	for _, a := range allow {
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
		if err := writeAllow(path, root, perms, kept, mode); err != nil {
			return 0, err
		}
	}
	_ = os.Remove(kitPermissionsRecord(home))
	return removed, nil
}
