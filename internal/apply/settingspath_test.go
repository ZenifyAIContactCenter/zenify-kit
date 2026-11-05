package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const localSettingsSeed = `{"autoMemoryDirectory":"/x","enabledPlugins":{"a@b":true},"env":{"FOO":"1"},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"my-own.sh"}]}]}}`

func TestEnsureOnLocalSettingsKeepsForeignKeys(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	path := WorkspaceLocalSettings(ws)
	if want := filepath.Join(ws, ".claude", "settings.local.json"); path != want {
		t.Fatalf("WorkspaceLocalSettings = %q, want %q", path, want)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(localSettingsSeed), 0o644); err != nil {
		t.Fatal(err)
	}
	target := WorkspaceKitTarget(home, ws)

	if _, err := EnsureGlobalHooks(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureReviewerEnv(path, false); err != nil {
		t.Fatal(err)
	}
	if n, err := EnsureKitAsk(target, false); err != nil || n == 0 {
		t.Fatalf("EnsureKitAsk n=%d err=%v", n, err)
	}
	mid, _ := os.ReadFile(path)
	if !strings.Contains(string(mid), "zenify hooks-run ") || !strings.Contains(string(mid), ReviewerEnv) {
		t.Fatalf("ensure wrote nothing to the local file: %s", mid)
	}
	// Nothing leaked to user scope.
	if _, err := os.Stat(UserSettings(home)); !os.IsNotExist(err) {
		t.Fatalf("user settings touched: %v", err)
	}

	if _, err := RemoveGlobalHooks(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveReviewerEnv(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveKitAsk(target, false); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "zenify hooks-run ") || strings.Contains(string(b), ReviewerEnv) {
		t.Fatalf("kit entries remain: %s", b)
	}
	var got struct {
		AutoMemoryDirectory string          `json:"autoMemoryDirectory"`
		EnabledPlugins      map[string]bool `json:"enabledPlugins"`
		Env                 map[string]string
		Hooks               map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.AutoMemoryDirectory != "/x" || !got.EnabledPlugins["a@b"] || got.Env["FOO"] != "1" {
		t.Fatalf("foreign keys lost: %s", b)
	}
	if len(got.Hooks["Stop"]) != 1 || got.Hooks["Stop"][0].Hooks[0].Command != "my-own.sh" {
		t.Fatalf("foreign hook lost: %s", b)
	}
}

func TestUserKitTargetRecordPathsUnchanged(t *testing.T) {
	home := "/h"
	tg := UserKitTarget(home)
	if tg.Settings != "/h/.claude/settings.json" {
		t.Errorf("Settings = %q", tg.Settings)
	}
	for name, got := range map[string]string{
		"/h/.zenify/playwright/kit-permissions.json": tg.permissionsRecord,
		"/h/.zenify/kit-ask.json":                    tg.askRecord,
		"/h/.zenify/playwright/kit-sandbox.json":     tg.sandboxRecord,
	} {
		if got != name {
			t.Errorf("record = %q, want %q", got, name)
		}
	}
}
