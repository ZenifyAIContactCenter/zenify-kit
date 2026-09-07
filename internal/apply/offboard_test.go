package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// SC-22: gỡ znf hook, giữ foreign hook + top-level key khác.
func TestRemoveGlobalHooks_RemovesZnfKeepsForeign(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	writeJSONFile(t, settings, map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash"}},
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "zenify hooks-run session-start"},
			}}},
			"Stop": []any{map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "zenify hooks-run docs-sync"},
				map[string]any{"type": "command", "command": "my-own-hook --foo"},
			}}},
		},
	})

	res, err := RemoveGlobalHooks(home, false)
	if err != nil {
		t.Fatalf("RemoveGlobalHooks: %v", err)
	}
	if res.Removed != 2 {
		t.Errorf("Removed = %d, want 2", res.Removed)
	}

	b, _ := os.ReadFile(settings) //nolint:gosec // G304 -- test-local path under t.TempDir, not externally-tainted
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["permissions"]; !ok {
		t.Error("permissions phải giữ nguyên")
	}
	hooks := root["hooks"].(map[string]any)
	if _, ok := hooks["SessionStart"]; ok {
		t.Error("SessionStart chỉ có znf hook → nhóm rỗng phải bị bỏ")
	}
	stop := hooks["Stop"].([]any)
	inner := stop[0].(map[string]any)["hooks"].([]any)
	if len(inner) != 1 {
		t.Fatalf("Stop phải còn đúng 1 hook foreign, có %d", len(inner))
	}
	if cmd := inner[0].(map[string]any)["command"].(string); cmd != "my-own-hook --foo" {
		t.Errorf("hook foreign bị đổi: %q", cmd)
	}
}

// SC-23: gỡ đúng dòng .worktrees/ trong exclude.
func TestRemoveExclude_RemovesOnlyWorktreesLine(t *testing.T) {
	repoDir := t.TempDir()
	excl := filepath.Join(repoDir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excl), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excl, []byte("node_modules/\n.worktrees/\n.DS_Store\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveExclude(repoDir, false)
	if err != nil || !removed {
		t.Fatalf("RemoveExclude removed=%v err=%v", removed, err)
	}
	b, _ := os.ReadFile(excl) //nolint:gosec // G304 -- test-local path under t.TempDir, not externally-tainted
	if string(b) != "node_modules/\n.DS_Store\n" {
		t.Errorf("exclude sai sau gỡ: %q", b)
	}
}

// SC-24: giữ settings user đã sửa, xoá skeleton chưa đụng.
func TestRemoveOwnedSettings_KeepsModifiedRemovesUnchanged(t *testing.T) {
	dir := t.TempDir()

	// (a) unchanged skeleton — fingerprint khớp manifest → xoá.
	clean := filepath.Join(dir, "clean", ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clean, []byte(`{"env":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// (b) modified — trên đĩa khác nội dung đã fingerprint → giữ.
	dirty := filepath.Join(dir, "dirty", ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(dirty), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirty, []byte(`{"env":{"MONGO_URL":"secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	owned := &managed.Manifest{Entries: map[string]managed.Entry{}}
	_ = owned.Record(clean)                                                                              // fingerprint = nội dung hiện tại (unchanged)
	owned.Entries[dirty] = managed.Entry{Path: dirty, SHA256: managed.Fingerprint([]byte(`{"env":{}}`))} // fingerprint CŨ (khác đĩa)

	if act, _ := RemoveOwnedSettings(clean, owned, false); act != "removed" {
		t.Errorf("clean action = %q, want removed", act)
	}
	if _, err := os.Stat(clean); err == nil {
		t.Error("clean skeleton phải bị xoá")
	}
	if act, _ := RemoveOwnedSettings(dirty, owned, false); act != "kept (modified)" {
		t.Errorf("dirty action = %q, want kept (modified)", act)
	}
	if _, err := os.Stat(dirty); err != nil {
		t.Error("dirty (user-modified) phải được giữ")
	}
}
