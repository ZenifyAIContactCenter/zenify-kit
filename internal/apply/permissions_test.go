package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
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

func TestEnsureKitPermissions_RespectsUserRemoval(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{}`)
	if _, err := EnsureKitPermissions(home, false); err != nil {
		t.Fatal(err)
	}
	const removed = "Bash(zenify visual *)"
	var kept []string
	for _, a := range readAllow(t, home) {
		if a != removed {
			kept = append(kept, a)
		}
	}
	body, _ := json.Marshal(map[string]any{"permissions": map[string]any{"allow": kept}})
	writeSettings(t, home, string(body))
	added, err := EnsureKitPermissions(home, false)
	if err != nil || added != 0 {
		t.Fatalf("added=%d err=%v, want 0", added, err)
	}
	got := readAllow(t, home)
	if !reflect.DeepEqual(got, kept) {
		t.Fatalf("allow changed: %v", got)
	}
	rec, err := allowList.readRecord(home)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rec {
		if r == removed {
			found = true
		}
	}
	if !found {
		t.Fatalf("record lost %q: %v", removed, rec)
	}
}

func TestEnsureKitPermissions_SettingsWriteFailureRestoresRecord(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("relies on chmod denying writes")
	}
	for _, withPrev := range []bool{false, true} {
		home := t.TempDir()
		writeSettings(t, home, `{}`)
		rec := kitPermissionsRecord(home)
		var before []byte
		if withPrev {
			if err := os.MkdirAll(filepath.Dir(rec), 0o755); err != nil {
				t.Fatal(err)
			}
			before = []byte(`["Bash(zenify e2e *)"]`)
			if err := os.WriteFile(rec, before, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		dir := filepath.Dir(settingsPath(home))
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		_, err := EnsureKitPermissions(home, false)
		_ = os.Chmod(dir, 0o755)
		if err == nil {
			t.Fatalf("withPrev=%v: want settings write error", withPrev)
		}
		got, rerr := os.ReadFile(rec)
		if withPrev {
			if rerr != nil || string(got) != string(before) {
				t.Fatalf("record not restored: %q %v", got, rerr)
			}
		} else if !os.IsNotExist(rerr) {
			t.Fatalf("record should be absent, got %q %v", got, rerr)
		}
	}
}

func TestKitSandbox_UnionKeepsSandboxThenRemovesOnlyKit(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"sandbox":{"enabled":true,"excludedCommands":["git *"]}}`)
	added, err := EnsureKitSandbox(home, false)
	if err != nil || added != len(KitSandboxExcluded) {
		t.Fatalf("added=%d err=%v", added, err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	var doc struct {
		Sandbox struct {
			Enabled  bool     `json:"enabled"`
			Excluded []string `json:"excludedCommands"`
		} `json:"sandbox"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	want := slices.Concat([]string{"git *"}, KitSandboxExcluded)
	if !doc.Sandbox.Enabled || !reflect.DeepEqual(doc.Sandbox.Excluded, want) {
		t.Fatalf("sandbox=%+v", doc.Sandbox)
	}
	if _, err := os.Stat(kitPermissionsRecord(home)); !os.IsNotExist(err) {
		t.Fatalf("sandbox ensure must not touch the allow record: %v", err)
	}
	removed, err := RemoveKitSandbox(home, false)
	if err != nil || removed != len(KitSandboxExcluded) {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	raw, _ = os.ReadFile(settingsPath(home))
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if want := []string{"git *"}; !reflect.DeepEqual(doc.Sandbox.Excluded, want) {
		t.Fatalf("after remove: %v", doc.Sandbox.Excluded)
	}
}

func TestKitSandbox_NonObjectSandboxSkips(t *testing.T) {
	home := t.TempDir()
	body := `{"sandbox":true}`
	writeSettings(t, home, body)
	if _, err := EnsureKitSandbox(home, false); err == nil {
		t.Fatal("expected error for non-object sandbox")
	}
	if raw, _ := os.ReadFile(settingsPath(home)); string(raw) != body {
		t.Fatalf("settings changed: %s", raw)
	}
}

func TestKitAllowRules_BrokerMCPServers(t *testing.T) {
	for _, want := range []string{"mcp__zenify-grafana__*", "mcp__zenify-posthog__*"} {
		found := false
		for _, r := range KitAllowRules {
			found = found || r == want
		}
		if !found {
			t.Errorf("KitAllowRules lacks %q", want)
		}
	}
}

func TestKitStackEntries(t *testing.T) {
	subs := []string{"init", "add", "checkout", "rebase", "push", "submit", "view"}
	for _, sub := range subs {
		if r := "Bash(gh stack " + sub + " *)"; !slices.Contains(KitAllowRules, r) {
			t.Errorf("KitAllowRules missing %s", r)
		}
		if r := "gh stack " + sub + " *"; !slices.Contains(KitSandboxExcluded, r) {
			t.Errorf("KitSandboxExcluded missing %s", r)
		}
	}
	if !slices.Contains(KitAllowRules, "Bash(gh stack --help)") {
		t.Error("KitAllowRules missing Bash(gh stack --help)")
	}
	for _, r := range slices.Concat(KitAllowRules, KitSandboxExcluded) {
		if r == "Bash(gh stack *)" || r == "gh stack *" {
			t.Errorf("blanket stack entry %q auto-allows every subcommand", r)
		}
	}
}

// The broker commands run without a prompt and outside the sandbox (they reach
// the broker and the DB); no blanket zenify entry is listed.
func TestKitBrokerEntries(t *testing.T) {
	for _, c := range []string{"zenify db-read *", "zenify redis-read *", "zenify data run *"} {
		if !slices.Contains(KitAllowRules, "Bash("+c+")") {
			t.Errorf("KitAllowRules missing Bash(%s)", c)
		}
		if !slices.Contains(KitSandboxExcluded, c) {
			t.Errorf("KitSandboxExcluded missing %s", c)
		}
	}
	for _, r := range slices.Concat(KitAllowRules, KitSandboxExcluded) {
		if r == "Bash(zenify *)" || r == "zenify *" {
			t.Errorf("blanket entry %q auto-allows every zenify command", r)
		}
	}
}

func TestKitAsk_UnionThenRemovesOnlyKit(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"permissions":{"allow":["Bash(ls *)"],"ask":["Bash(rm *)"]}}`)
	added, err := EnsureKitAsk(home, false)
	if err != nil || added != len(KitAskRules) {
		t.Fatalf("added=%d err=%v", added, err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if want := slices.Concat([]string{"Bash(rm *)"}, KitAskRules); !reflect.DeepEqual(doc.Permissions.Ask, want) {
		t.Fatalf("ask=%v", doc.Permissions.Ask)
	}
	if want := []string{"Bash(ls *)"}; !reflect.DeepEqual(doc.Permissions.Allow, want) {
		t.Fatalf("ask ensure changed allow: %v", doc.Permissions.Allow)
	}
	removed, err := RemoveKitAsk(home, false)
	if err != nil || removed != len(KitAskRules) {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	raw, _ = os.ReadFile(settingsPath(home))
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bash(rm *)"}; !reflect.DeepEqual(doc.Permissions.Ask, want) {
		t.Fatalf("after remove: %v", doc.Permissions.Ask)
	}
}

// Claude Code reads * in a Bash rule as any text; a prd write in either flag
// order (root flags too) must hit an ask rule, a stg write or prd dry-run none.
func TestKitAskRules_MatchProdWritesOnly(t *testing.T) {
	var res []*regexp.Regexp
	for _, r := range KitAskRules {
		body := strings.TrimSuffix(strings.TrimPrefix(r, "Bash("), ")")
		res = append(res, regexp.MustCompile("^"+strings.ReplaceAll(regexp.QuoteMeta(body), `\*`, ".*")+"$"))
	}
	hit := func(cmd string) bool {
		return slices.ContainsFunc(res, func(re *regexp.Regexp) bool { return re.MatchString(cmd) })
	}
	for _, c := range []string{
		"zenify data run --prd --write -- node /w/docs/scripts/x.js",
		"zenify data run --write --prd -- node /w/docs/scripts/x.js",
		"zenify data run --prd --lumi --write -- node x.js",
		"zenify --no-color data run --prd --write -- node x.js",
		"zenify data run --prd --write=true -- node x.js",
	} {
		if !hit(c) {
			t.Errorf("prd write %q prompts nothing", c)
		}
	}
	for _, c := range []string{
		"zenify data run --stg --write -- node x.js",
		"zenify data run --prd -- node x.js --dry-run",
		"zenify db-read --prd collections",
	} {
		if hit(c) {
			t.Errorf("%q prompts, want no ask", c)
		}
	}
}
