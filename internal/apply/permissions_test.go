package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readAllow(t *testing.T, home string) []string {
	t.Helper()
	raw, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Permissions.Allow
}

func TestEnsureKitPermissions_UnionKeepsOrderAndDeny(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["Bash(git status)"],"deny":["Bash(rm *)"]},"model":"opus"}`)
	added, err := EnsureKitPermissions(home, false)
	if err != nil || added != len(KitAllowRules) {
		t.Fatalf("first run: added=%d err=%v", added, err)
	}
	added, err = EnsureKitPermissions(home, false)
	if err != nil || added != 0 {
		t.Fatalf("second run: added=%d err=%v", added, err)
	}
	allow := readAllow(t, home)
	if allow[0] != "Bash(git status)" {
		t.Fatalf("allow[0]=%q", allow[0])
	}
	for _, r := range KitAllowRules {
		n := 0
		for _, a := range allow {
			if a == r {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("rule %q appears %d times", r, n)
		}
	}
	raw, _ := os.ReadFile(settingsPath(home))
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	if string(root["model"]) != `"opus"` {
		t.Fatalf("model changed: %s", root["model"])
	}
	var perms map[string]json.RawMessage
	_ = json.Unmarshal(root["permissions"], &perms)
	if strings.Join(strings.Fields(string(perms["deny"])), "") != `["Bash(rm*)"]` {
		t.Fatalf("deny changed: %s", perms["deny"])
	}
}

func TestEnsureKitPermissions_NoFileCreates(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureKitPermissions(home, false); err != nil {
		t.Fatal(err)
	}
	if got := readAllow(t, home); !reflect.DeepEqual(got, KitAllowRules) {
		t.Fatalf("allow=%v", got)
	}
}

func TestEnsureKitPermissions_NonObjectPermissionsSkips(t *testing.T) {
	for _, body := range []string{`{"permissions":[]}`, `{"permissions":{"allow":"x"}}`} {
		home := t.TempDir()
		writeSettings(t, home, body)
		if _, err := EnsureKitPermissions(home, false); err == nil {
			t.Fatalf("%s: expected error", body)
		}
		raw, _ := os.ReadFile(settingsPath(home))
		if string(raw) != body {
			t.Fatalf("%s: file changed to %s", body, raw)
		}
	}
}

func TestEnsureKitPermissions_DryRunNoWrite(t *testing.T) {
	home := t.TempDir()
	body := `{"model":"opus"}`
	writeSettings(t, home, body)
	added, err := EnsureKitPermissions(home, true)
	if err != nil || added == 0 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if string(raw) != body {
		t.Fatalf("file changed: %s", raw)
	}
}

func TestEnsureKitPermissions_NoServerWideRule(t *testing.T) {
	for _, r := range KitAllowRules {
		if r == "mcp__playwright" || strings.Contains(r, "browser_run_code_unsafe") {
			t.Fatalf("forbidden rule %q", r)
		}
	}
}

func TestRemoveKitPermissions_KeepsPreexisting(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["mcp__playwright__browser_click","Bash(git status)"]}}`)
	if _, err := EnsureKitPermissions(home, false); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveKitPermissions(home, false)
	if err != nil || removed != len(KitAllowRules)-1 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	want := []string{"mcp__playwright__browser_click", "Bash(git status)"}
	if got := readAllow(t, home); !reflect.DeepEqual(got, want) {
		t.Fatalf("allow=%v", got)
	}
}

func TestRemoveKitPermissions_NoRecord(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["mcp__playwright__browser_click"]}}`)
	removed, err := RemoveKitPermissions(home, false)
	if removed != 0 || err == nil || !strings.Contains(err.Error(), "no kit-permissions record") {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
}

func TestEnsureKitPermissions_MalformedRecordErrorsUntouched(t *testing.T) {
	home := t.TempDir()
	body := `{"model":"opus"}`
	writeSettings(t, home, body)
	rec := kitPermissionsRecord(home)
	if err := os.MkdirAll(filepath.Dir(rec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rec, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureKitPermissions(home, false); err == nil {
		t.Fatal("expected error for malformed record")
	}
	if raw, _ := os.ReadFile(settingsPath(home)); string(raw) != body {
		t.Fatalf("settings changed: %s", raw)
	}
	if raw, _ := os.ReadFile(rec); string(raw) != "{not json" {
		t.Fatalf("record changed: %s", raw)
	}
}
