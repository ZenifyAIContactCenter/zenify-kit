package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
			return nil, false, fmt.Errorf("guard install: settings.json không parse được: %w", err)
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

func newGuardCmd() *cobra.Command {
	c := &cobra.Command{Use: "guard", Short: "Quản lý git-guard hook"}
	install := &cobra.Command{
		Use:   "install",
		Short: "Đăng ký PreToolUse hook trỏ `zenify git-guard` trong ~/.claude/settings.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("guard install: không xác định được HOME: %w", err)
			}
			path := filepath.Join(home, ".claude", "settings.json")
			raw, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed config location under the user's own HOME, not attacker-controlled
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("guard install: đọc %s: %w", path, err)
			}
			out, changed, err := ensureGuardHook(raw)
			if err != nil {
				return err
			}
			if !changed {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "guard install: đã cấu hình sẵn (idempotent).")
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return fmt.Errorf("guard install: tạo thư mục %s: %w", filepath.Dir(path), err)
			}
			perm := os.FileMode(0o644)
			if fi, statErr := os.Stat(path); statErr == nil {
				perm = fi.Mode().Perm()
			}
			if err := writeFileAtomic(path, out, perm); err != nil {
				return fmt.Errorf("guard install: ghi %s: %w", path, err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "guard install: đã trỏ PreToolUse → zenify git-guard.")
			return nil
		},
	}
	c.AddCommand(install)
	return c
}
