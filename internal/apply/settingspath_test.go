package apply

import (
	"encoding/json"
	"errors"
	"io/fs"
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

// Two workspaces on one machine keep separate kit records: rules recorded for
// one never read as "removed by the user" in the other, and removing them from
// one leaves the other's record alone.
func TestWorkspaceKitTargetRecordsPerWorkspace(t *testing.T) {
	home, wsA, wsB := t.TempDir(), t.TempDir(), t.TempDir()
	a, b := WorkspaceKitTarget(home, wsA), WorkspaceKitTarget(home, wsB)
	if a.permissionsRecord == b.permissionsRecord {
		t.Fatalf("shared record %q", a.permissionsRecord)
	}
	nA, err := EnsureKitPermissions(a, false)
	if err != nil || nA == 0 {
		t.Fatalf("A: n=%d err=%v", nA, err)
	}
	nB, err := EnsureKitPermissions(b, false)
	if err != nil || nB != nA {
		t.Fatalf("B: n=%d err=%v, want %d", nB, err, nA)
	}
	if _, err := RemoveKitPermissions(a, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.permissionsRecord); err != nil {
		t.Fatalf("B record gone after removing A: %v", err)
	}
	// A relative path names the same workspace as its absolute form.
	t.Chdir(wsA)
	if rel := WorkspaceKitTarget(home, "."); rel.permissionsRecord != a.permissionsRecord {
		t.Fatalf("relative ws record %q != %q", rel.permissionsRecord, a.permissionsRecord)
	}
}

// A kit rule the user removed at user level stays removed in the workspace:
// the first workspace ensure must not add it back.
func TestSeedWorkspaceRecordsKeepsUserRemovals(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	user := UserKitTarget(home)
	if _, err := EnsureKitPermissions(user, false); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureKitSandbox(user, false); err != nil {
		t.Fatal(err)
	}
	gone, goneSandbox := KitAllowRules[0], KitSandboxExcluded[0]
	b, _ := os.ReadFile(user.Settings)
	s := strings.Replace(string(b), `"`+gone+`",`, "", 1)
	s = strings.Replace(s, `"`+goneSandbox+`",`, "", 1)
	if s == string(b) {
		t.Fatalf("fixture: could not remove %q from %s", gone, b)
	}
	if err := os.WriteFile(user.Settings, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SeedWorkspaceRecords(home, ws); err != nil {
		t.Fatal(err)
	}
	wt := WorkspaceKitTarget(home, ws)
	if _, err := EnsureKitPermissions(wt, false); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureKitSandbox(wt, false); err != nil {
		t.Fatal(err)
	}
	local, _ := os.ReadFile(wt.Settings)
	if strings.Contains(string(local), `"`+gone+`"`) || strings.Contains(string(local), `"`+goneSandbox+`"`) {
		t.Fatalf("user-removed rule re-added in the workspace: %s", local)
	}
	if !strings.Contains(string(local), `"`+KitAllowRules[1]+`"`) {
		t.Fatalf("other kit rules missing: %s", local)
	}
	// a workspace that already has its record is left alone
	if err := SeedWorkspaceRecords(home, ws); err != nil {
		t.Fatal(err)
	}
	if n, err := RemoveKitPermissions(wt, false); err != nil || n != len(KitAllowRules)-1 {
		t.Fatalf("remove n=%d err=%v", n, err)
	}
}

// The migration removes the user-level kit rules and their record; what the
// user removed must still reach a workspace set up after it.
func TestKeepUserKitRemovalsAcrossMigration(t *testing.T) {
	home, wsB := t.TempDir(), t.TempDir()
	user := UserKitTarget(home)
	if _, err := EnsureKitPermissions(user, false); err != nil {
		t.Fatal(err)
	}
	gone := KitAllowRules[0]
	b, _ := os.ReadFile(user.Settings)
	if err := os.WriteFile(user.Settings, []byte(strings.Replace(string(b), `"`+gone+`",`, "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	err := KeepUserKitRemovals(home, func() error {
		_, err := RemoveKitPermissions(user, false)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(user.Settings); strings.Contains(string(after), KitAllowRules[1]) {
		t.Fatalf("kit rules left at user level: %s", after)
	}
	if err := SeedWorkspaceRecords(home, wsB); err != nil {
		t.Fatal(err)
	}
	wt := WorkspaceKitTarget(home, wsB)
	if _, err := EnsureKitPermissions(wt, false); err != nil {
		t.Fatal(err)
	}
	local, _ := os.ReadFile(wt.Settings)
	if strings.Contains(string(local), `"`+gone+`"`) || !strings.Contains(string(local), KitAllowRules[1]) {
		t.Fatalf("workspace B after migration: %s", local)
	}
	// The user adds the rule back by hand: a later migration must leave it.
	var root map[string]any
	b, _ = os.ReadFile(user.Settings)
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	perms, _ := root["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
		root["permissions"] = perms
	}
	allow, _ := perms["allow"].([]any)
	perms["allow"] = append(allow, gone)
	b, _ = json.Marshal(root)
	if err := os.WriteFile(user.Settings, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ = os.ReadFile(user.Settings); !strings.Contains(string(b), `"`+gone+`"`) {
		t.Fatalf("fixture did not re-add the rule: %s", b)
	}
	err = KeepUserKitRemovals(home, func() error {
		if _, err := RemoveKitPermissions(user, false); !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(user.Settings); !strings.Contains(string(after), `"`+gone+`"`) {
		t.Fatalf("the rule the user added back was removed: %s", after)
	}
}

// A removal that fails part-way must not lose what the user removed.
func TestKeepUserKitRemovalsSurvivesFailedRemove(t *testing.T) {
	home, wsB := t.TempDir(), t.TempDir()
	user := UserKitTarget(home)
	if _, err := EnsureKitPermissions(user, false); err != nil {
		t.Fatal(err)
	}
	gone := KitAllowRules[0]
	b, _ := os.ReadFile(user.Settings)
	if err := os.WriteFile(user.Settings, []byte(strings.Replace(string(b), `"`+gone+`",`, "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	err := KeepUserKitRemovals(home, func() error {
		if _, err := RemoveKitPermissions(user, false); err != nil {
			return err
		}
		return os.ErrPermission
	})
	if err == nil {
		t.Fatal("want the remove error back")
	}
	if err := SeedWorkspaceRecords(home, wsB); err != nil {
		t.Fatal(err)
	}
	wt := WorkspaceKitTarget(home, wsB)
	if _, err := EnsureKitPermissions(wt, false); err != nil {
		t.Fatal(err)
	}
	if local, _ := os.ReadFile(wt.Settings); strings.Contains(string(local), `"`+gone+`"`) {
		t.Fatalf("workspace B re-added a user-removed rule: %s", local)
	}
}
