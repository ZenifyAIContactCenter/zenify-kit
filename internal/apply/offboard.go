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

// HookRemoval mô tả kết quả gỡ znf hook.
type HookRemoval struct {
	Removed int
	Skipped bool // settings.json malformed → để nguyên
}

// RemoveGlobalHooks gỡ mọi hook entry do zenify wire (command bắt đầu hookMarker)
// khỏi <home>/.claude/settings.json. Đảo ngược EnsureGlobalHooks: giữ nguyên mọi
// foreign hook và mọi top-level key khác; nhóm/nhánh rỗng sau khi gỡ thì bỏ.
// Atomic write; fail-open (malformed → Skipped, không sửa). Trong dryRun chỉ đếm.
func RemoveGlobalHooks(home string, dryRun bool) (HookRemoval, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	existing, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed ~/.claude path, not externally-tainted
	if err != nil {
		if os.IsNotExist(err) {
			return HookRemoval{}, nil // không có gì để gỡ
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
					continue // gỡ
				}
				kept = append(kept, h)
			}
			if len(kept) == 0 {
				continue // nhóm rỗng → bỏ
			}
			gm["hooks"] = kept
			newGroups = append(newGroups, gm)
		}
		if len(newGroups) == 0 {
			delete(hooks, event) // event rỗng → bỏ
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

// RemoveExclude gỡ đúng dòng ".worktrees/" khỏi repo .git/info/exclude, giữ mọi
// dòng khác. Trả (true) nếu dòng có mặt và (đã) gỡ. File không có dòng → (false,nil).
// File trống sau khi gỡ vẫn để lại (không xoá file git-local). dryRun → không ghi.
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

// RemoveOwnedSettings dùng DecideRefresh để quyết mỗi settings.local.json owned:
// unchanged (khớp fingerprint) → xoá; modified (user sửa, vd secret) → giữ; entry
// không có/đĩa vắng → skip. dryRun → không xoá thật.
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
