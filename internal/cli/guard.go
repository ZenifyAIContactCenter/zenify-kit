package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

const guardCommand = "zenify git-guard"
const legacyGuard = "guard-git-deploy.sh"

// ensureGuardHook ensures hooks.PreToolUse has an entry with matcher "Bash"
// running `zenify git-guard`. It replaces an existing entry that points at
// the legacy bash guard script, leaves everything else untouched, and is
// idempotent — calling it again once the hook is present reports
// changed=false. Broken JSON input is an error; nothing is overwritten.
func ensureGuardHook(raw []byte) ([]byte, bool, error) {
	root := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			return nil, false, fmt.Errorf("guard install: could not parse settings.json: %w", err)
		}
	}

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		root["hooks"] = hooks
	}
	pre, _ := hooks["PreToolUse"].([]any)

	newEntry := map[string]any{
		"matcher": "Bash",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": guardCommand,
		}},
	}

	found := false
	changed := false
	newPre := make([]any, 0, len(pre)+1)
	for _, e := range pre {
		entry, _ := e.(map[string]any)
		if entry == nil {
			newPre = append(newPre, e)
			continue
		}
		hs, _ := entry["hooks"].([]any)
		newHs := make([]any, 0, len(hs))
		for _, h := range hs {
			hm, _ := h.(map[string]any)
			if hm == nil {
				newHs = append(newHs, h)
				continue
			}
			cmd, _ := hm["command"].(string)
			if cmd == guardCommand {
				found = true
			}
			if strings.Contains(cmd, legacyGuard) {
				// Splice out only this individual legacy hook — keep any
				// sibling hooks in the same entry, and keep the entry's
				// original matcher untouched.
				changed = true
				continue
			}
			newHs = append(newHs, h)
		}
		if len(newHs) == 0 {
			// Dropping the legacy hook left this entry with zero hooks —
			// drop the whole entry rather than leaving a matcher with an
			// empty hooks array.
			continue
		}
		entry["hooks"] = newHs
		newPre = append(newPre, entry)
	}
	pre = newPre
	if !found {
		pre = append(pre, newEntry)
		changed = true
	}
	hooks["PreToolUse"] = pre

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), changed, nil
}

// writeFileAtomic writes data to a temp file in the same directory as path,
// then renames it over path, so a crash mid-write never leaves a truncated
// file at the destination.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
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
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// installGuard wires the git-guard hook into <home>/.claude/settings.json.
// Shared by `guard install`, ensureWorkspace (up + SessionStart) and the
// doctor git-guard Fix. changed=false when the hook is already present.
func installGuard(home string) (bool, error) {
	path := guardSettingsPath(home)
	raw, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed config location under the user's own HOME, not attacker-controlled
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("guard install: reading %s: %w", path, err)
	}
	out, changed, err := ensureGuardHook(raw)
	if err != nil || !changed {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return false, fmt.Errorf("guard install: creating directory %s: %w", filepath.Dir(path), err)
	}
	perm := os.FileMode(0o644)
	if fi, statErr := os.Stat(path); statErr == nil {
		perm = fi.Mode().Perm()
	}
	if err := writeFileAtomic(path, out, perm); err != nil {
		return false, fmt.Errorf("guard install: writing %s: %w", path, err)
	}
	return true, nil
}

// guardInstalled reports whether settings.json already runs `zenify git-guard`
// as a PreToolUse hook. Read-only; for the doctor check.
func guardInstalled(home string) (bool, error) {
	raw, err := os.ReadFile(guardSettingsPath(home)) //nolint:gosec // G304 -- fixed config location under the user's own HOME, not attacker-controlled
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, changed, err := ensureGuardHook(raw)
	return err == nil && !changed, err
}

func guardSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

func newGuardCmd() *cobra.Command {
	c := &cobra.Command{Use: "guard", Short: "Quản lý git-guard hook"} //znf:allow-lang
	install := &cobra.Command{
		Use:   "install",
		Short: "Đăng ký PreToolUse hook trỏ `zenify git-guard` trong ~/.claude/settings.json", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("guard install: could not determine HOME: %w", err)
			}
			changed, err := installGuard(home)
			if err != nil {
				return err
			}
			u := uiOut(cmd)
			if !changed {
				u.Step(ui.StatusOK, "guard install: already configured (idempotent).", "")
				return nil
			}
			u.Step(ui.StatusOK, "guard install: wired PreToolUse → zenify git-guard.", "")
			return nil
		},
	}
	c.AddCommand(install)
	return c
}
