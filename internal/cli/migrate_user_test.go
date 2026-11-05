package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

// fakeClaude is a stateful stand-in for the claude CLI: user-scope MCP servers
// exist until removed, the plugin is enabled at local scope once installed,
// and failInstall makes `plugin install` fail. calls logs every invocation.
type fakeClaude struct {
	ws          string
	failInstall bool
	installed   bool
	mktAdded    bool
	failUpdate  bool // `plugin update` fails even when installed
	failAddJSON bool // `mcp add-json` fails
	failRemove  bool // `mcp remove` fails with a real error
	userMCP     map[string]bool
	calls       []string
}

func newFakeClaude(ws string) *fakeClaude {
	return &fakeClaude{ws: ws, userMCP: map[string]bool{"zenify-grafana": true, "zenify-posthog": true, "zenify-plane": true}}
}

func (f *fakeClaude) run(_ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch {
	case len(args) >= 3 && args[0] == "mcp" && args[1] == "get":
		return []byte(fmt.Sprintf("No MCP server named %q found", args[2])), fmt.Errorf("exit status 1")
	case len(args) >= 2 && args[0] == "mcp" && args[1] == "add-json":
		if f.failAddJSON {
			return []byte("boom"), fmt.Errorf("exit status 1")
		}
	case len(args) >= 3 && args[0] == "mcp" && args[1] == "remove":
		if f.failRemove {
			return []byte("boom"), fmt.Errorf("exit status 1")
		}
		if f.userMCP[args[2]] {
			delete(f.userMCP, args[2])
			return nil, nil
		}
		return []byte(fmt.Sprintf("No MCP server named %q in user scope", args[2])), fmt.Errorf("exit status 1")
	case len(args) >= 2 && args[0] == "plugin" && args[1] == "list":
		if !f.installed {
			return []byte("[]"), nil
		}
		b, _ := json.Marshal([]map[string]any{{"id": plugin.PluginID, "scope": "local", "enabled": true, "projectPath": f.ws}})
		return b, nil
	// Measured on 2.1.318: both updates exit 1 before the add / install.
	case len(args) >= 3 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "update":
		if !f.mktAdded {
			return []byte("Marketplace not found"), fmt.Errorf("exit status 1")
		}
	case len(args) >= 3 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "add":
		f.mktAdded = true
	case len(args) >= 2 && args[0] == "plugin" && args[1] == "update":
		if !f.installed || f.failUpdate {
			return []byte("Plugin not found"), fmt.Errorf("exit status 1")
		}
	case len(args) >= 2 && args[0] == "plugin" && args[1] == "install":
		if f.failInstall {
			return []byte("boom"), fmt.Errorf("exit status 1")
		}
		f.installed = true
	}
	return nil, nil
}

func (f *fakeClaude) userRemoves() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "mcp remove ") && strings.HasSuffix(c, " -s user") {
			out = append(out, c)
		}
	}
	return out
}

// useFakeClaude routes pluginRunner to f for the test.
func useFakeClaude(t *testing.T, f *fakeClaude) {
	t.Helper()
	orig := pluginRunner
	pluginRunner = func(string) func(string, ...string) ([]byte, error) { return f.run }
	t.Cleanup(func() { pluginRunner = orig })
}

func readJSON(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func writeSettingsJSON(t *testing.T, p string, m map[string]any) {
	t.Helper()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkfile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedUserInstall builds the user-level install an older kit left behind,
// through the kit's own writers, plus personal entries that must survive.
func seedUserInstall(t *testing.T, home, ws string) {
	t.Helper()
	mkfile(t, filepath.Join(home, ".claude", "skills", "znf", ".manifest.json"), "{}")
	mkfile(t, filepath.Join(home, ".claude", "skills", "znf", "cook", "SKILL.md"), "kit")
	mkfile(t, filepath.Join(home, ".claude", "skills", "cook", "SKILL.md"), "mine")
	settings := apply.UserSettings(home)
	if _, err := apply.EnsureGlobalHooks(settings, false); err != nil {
		t.Fatal(err)
	}
	if _, err := installGuard(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := apply.EnsureReviewerEnv(settings, false); err != nil {
		t.Fatal(err)
	}
	if _, err := apply.EnsureKitPermissions(apply.UserKitTarget(home), false); err != nil {
		t.Fatal(err)
	}
	if _, err := apply.EnsureKitDeny(home, ws, false); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, settings)
	m["env"].(map[string]any)["MINE"] = "1"
	perms := m["permissions"].(map[string]any)
	perms["allow"] = append(perms["allow"].([]any), "Bash(my-tool *)")
	hooks := m["hooks"].(map[string]any)
	hooks["Stop"] = append(asSlice(hooks["Stop"]), map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "~/bin/my-own.sh"}}})
	writeSettingsJSON(t, settings, m)

	// Codex: the kit skills link, kit hooks next to a personal one, kit agents.
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".claude", "skills", "znf"), filepath.Join(home, ".agents", "skills", "znf")); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(home, ".codex")
	if _, err := apply.EnsureCodexHooks(codexHome, false); err != nil {
		t.Fatal(err)
	}
	hp := filepath.Join(codexHome, "hooks.json")
	h := readJSON(t, hp)
	ch := h["hooks"].(map[string]any)
	ch["Stop"] = append(asSlice(ch["Stop"]), map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "~/bin/my-codex.sh"}}})
	writeSettingsJSON(t, hp, h)
	if ar, err := plugin.SyncCodexAgents(codexHome, false); err != nil || len(ar.Written) == 0 {
		t.Fatalf("seed codex agents: %v %+v", err, ar)
	}
}

func asSlice(v any) []any { s, _ := v.([]any); return s }

func TestMigrateUserInstalls_RemovesKitOnly(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	seedUserInstall(t, home, ws)
	deny, _ := apply.KitDenyRule(ws)
	f := newFakeClaude(ws)

	removed, err := migrateUserInstalls(home, codexHome, userKitMCPs, f.run, false)
	if err != nil {
		t.Fatalf("migrateUserInstalls: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("nothing reported removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf")); !os.IsNotExist(err) {
		t.Errorf("skills/znf still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "cook", "SKILL.md")); err != nil {
		t.Errorf("personal skill gone: %v", err)
	}
	raw, _ := os.ReadFile(apply.UserSettings(home))
	s := string(raw)
	for _, gone := range []string{"zenify hooks-run", "zenify git-guard", apply.ReviewerEnv, apply.KitAllowRules[0]} {
		if strings.Contains(s, gone) {
			t.Errorf("settings.json still has %q:\n%s", gone, s)
		}
	}
	for _, kept := range []string{"my-own.sh", `"MINE"`, "Bash(my-tool *)", deny} {
		if !strings.Contains(s, kept) {
			t.Errorf("settings.json lost %q:\n%s", kept, s)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "znf")); !os.IsNotExist(err) {
		t.Errorf("~/.agents/skills/znf still there: %v", err)
	}
	hooks, _ := os.ReadFile(filepath.Join(codexHome, "hooks.json"))
	if strings.Contains(string(hooks), "zenify hooks-run") || !strings.Contains(string(hooks), "my-codex.sh") {
		t.Errorf("codex hooks.json after migrate:\n%s", hooks)
	}
	if agents, _ := filepath.Glob(filepath.Join(codexHome, "agents", "*.toml")); len(agents) > 0 {
		t.Errorf("kit codex agents left: %v", agents)
	}
	got := f.userRemoves()
	sort.Strings(got)
	want := []string{"mcp remove zenify-grafana -s user", "mcp remove zenify-plane -s user", "mcp remove zenify-posthog -s user"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("user MCP removes = %v, want %v", got, want)
	}
}

func TestMigrateUserInstalls_SkipsDirWithoutManifest(t *testing.T) {
	home := t.TempDir()
	mkfile(t, filepath.Join(home, ".claude", "skills", "znf", "SKILL.md"), "mine")
	f := newFakeClaude(t.TempDir())
	if _, err := migrateUserInstalls(home, filepath.Join(home, ".codex"), userKitMCPs, f.run, false); err != nil {
		t.Fatalf("migrateUserInstalls: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf", "SKILL.md")); err != nil {
		t.Errorf("skills/znf without kit manifest was removed: %v", err)
	}
}

// Dry-run lists what would go and changes nothing, never running claude.
func TestMigrateUserInstalls_DryRunRemovesNothing(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	seedUserInstall(t, home, ws)
	before, _ := os.ReadFile(apply.UserSettings(home))
	removed, err := migrateUserInstalls(home, filepath.Join(home, ".codex"), userKitMCPs, nil, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(removed) < 4 {
		t.Errorf("dry run listed %v", removed)
	}
	after, _ := os.ReadFile(apply.UserSettings(home))
	if string(before) != string(after) {
		t.Error("dry run changed settings.json")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf")); err != nil {
		t.Errorf("dry run removed skills/znf: %v", err)
	}
}

// upFixture isolates HOME, ZENIFY_HOME and CODEX_HOME, seeds the old user
// install and returns what runApply needs.
func upFixture(t *testing.T) (home, ws string, m *manifest.Manifest, plans []reconcile.RepoPlan) {
	t.Helper()
	t.Setenv("ZENIFY_HOME", t.TempDir())
	home, ws = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(filepath.Join(ws, "svc", ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	seedUserInstall(t, home, ws)
	m = &manifest.Manifest{Org: "MyOrg", Repos: []manifest.Repo{{Name: "svc", Path: "svc"}}}
	plans = []reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}}
	return home, ws, m, plans
}

// SC-6: a failed local plugin install keeps every user-level install.
func TestUp_LocalFailureKeepsUserInstalls(t *testing.T) {
	home, ws, m, plans := upFixture(t)
	f := newFakeClaude(ws)
	f.failInstall = true
	useFakeClaude(t, f)
	var errOut strings.Builder
	err := runApply(io.Discard, &errOut, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{})
	if err == nil {
		t.Fatal("runApply succeeded although plugin install failed")
	}
	if !strings.Contains(errOut.String(), "giữ nguyên bản cài cấp user vì bước local lỗi: ") { //znf:allow-lang
		t.Errorf("stderr lacks the keep message: %q", errOut.String())
	}
	if got := f.userRemoves(); len(got) > 0 {
		t.Errorf("user MCP removed despite the failure: %v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf")); err != nil {
		t.Errorf("skills/znf removed despite the failure: %v", err)
	}
	if b, _ := os.ReadFile(apply.UserSettings(home)); !strings.Contains(string(b), "zenify hooks-run") {
		t.Error("user hooks removed despite the failure")
	}
	// The user hooks stay, so the local copies must go or every hook runs twice.
	if b, _ := os.ReadFile(apply.WorkspaceLocalSettings(ws)); strings.Contains(string(b), "zenify hooks-run") || strings.Contains(string(b), "zenify git-guard") {
		t.Errorf("local hooks left beside the user hooks: %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, ".codex", "hooks.json")); strings.Contains(string(b), "zenify hooks-run") {
		t.Errorf("local codex hooks left beside the user hooks: %s", b)
	}
}

// A machine already migrated has only the local hooks: a later failing up must keep them.
func TestUp_LocalFailureAfterMigrationKeepsLocalHooks(t *testing.T) {
	_, ws, m, plans := upFixture(t)
	f := newFakeClaude(ws)
	useFakeClaude(t, f)
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("first runApply: %v", err)
	}
	if err := os.Remove(pluginStampPath(ws)); err != nil {
		t.Fatal(err)
	}
	f.failUpdate = true
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err == nil {
		t.Fatal("second runApply succeeded although plugin update failed")
	}
	b, _ := os.ReadFile(apply.WorkspaceLocalSettings(ws))
	if !strings.Contains(string(b), "zenify hooks-run") || !strings.Contains(string(b), "zenify git-guard") {
		t.Errorf("local hooks removed on a migrated machine: %s", b)
	}
}

// snapshotTree maps every regular file and symlink under roots to its content.
func snapshotTree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range roots {
		_ = filepath.Walk(r, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				dst, _ := os.Readlink(p)
				out[p] = "-> " + dst
				return nil
			}
			b, _ := os.ReadFile(p)
			out[p] = string(b)
			return nil
		})
	}
	return out
}

// SC-5: the second up changes no file and leaves no user-level install.
func TestUp_TwiceNoChange(t *testing.T) {
	home, ws, m, plans := upFixture(t)
	useLocalMCPs(t, true)
	f := newFakeClaude(ws)
	useFakeClaude(t, f)
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("first runApply: %v", err)
	}
	roots := []string{filepath.Join(home, ".claude"), filepath.Join(home, ".agents"), filepath.Join(home, ".codex"), filepath.Join(ws, ".claude"), filepath.Join(ws, ".codex")}
	before := snapshotTree(t, roots...)
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("second runApply: %v", err)
	}
	after := snapshotTree(t, roots...)
	for p, v := range after {
		if before[p] != v {
			t.Errorf("second up changed %s", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("second up removed %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf")); !os.IsNotExist(err) {
		t.Errorf("skills/znf still there: %v", err)
	}
	if len(f.userMCP) > 0 {
		t.Errorf("user MCP left: %v", f.userMCP)
	}
	if removed, err := migrateUserInstalls(home, filepath.Join(home, ".codex"), userKitMCPs, f.run, false); err != nil || len(removed) > 0 {
		t.Errorf("third migrate removed %v, err %v", removed, err)
	}
}

// N1: the migration fails after removing the user Claude hooks but before the
// user Codex hooks; the workspace Codex hooks must go so Codex runs each kit
// hook once, while the workspace Claude hooks stay (the user ones are gone).
func TestUp_MigrationFailureKeepsHooksSingle(t *testing.T) {
	home, ws, m, plans := upFixture(t)
	useLocalMCPs(t, true)
	f := newFakeClaude(ws)
	f.failRemove = true
	useFakeClaude(t, f)
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err == nil {
		t.Fatal("runApply succeeded although mcp remove failed")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".codex", "hooks.json")); !strings.Contains(string(b), "zenify hooks-run") {
		t.Fatalf("precondition: user codex hooks should remain after the failed migration: %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, ".codex", "hooks.json")); strings.Contains(string(b), "zenify hooks-run") {
		t.Errorf("workspace codex hooks left beside the user ones: %s", b)
	}
	if b, _ := os.ReadFile(apply.UserSettings(home)); strings.Contains(string(b), "zenify hooks-run") {
		t.Fatalf("precondition: user claude hooks should be gone: %s", b)
	}
	if b, _ := os.ReadFile(apply.WorkspaceLocalSettings(ws)); !strings.Contains(string(b), "zenify hooks-run") || !strings.Contains(string(b), "zenify git-guard") {
		t.Errorf("workspace claude hooks removed although the user ones are gone: %s", b)
	}
}

// useLocalMCPs makes up register all three kit MCPs at local scope: a trusted
// manifest with a broker and Plane, claude and uvx on PATH, and a Plane PAT
// unless pat is false.
func useLocalMCPs(t *testing.T, pat bool) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "repos.yaml")
	y := noBrokerYAML + "broker:\n  url: https://broker.example\nplane:\n  url: https://plane.example\n  workspace: ws\n"
	if err := os.WriteFile(p, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	useTrustedManifest(t, p)
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(n string) (string, error) { return "/fake/" + n, nil }
	if pat {
		t.Setenv(planePATKey, "pat")
	} else {
		t.Setenv(planePATKey, "")
	}
}

// A kit MCP whose local registration was skipped (here: no Plane PAT) keeps
// its user-scope copy; the ones registered locally lose theirs.
func TestUp_SkippedLocalMCPKeepsUserMCP(t *testing.T) {
	_, ws, m, plans := upFixture(t)
	useLocalMCPs(t, false)
	f := newFakeClaude(ws)
	useFakeClaude(t, f)
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	got := f.userRemoves()
	sort.Strings(got)
	want := []string{"mcp remove zenify-grafana -s user", "mcp remove zenify-posthog -s user"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("user MCP removes = %v, want %v", got, want)
	}
	if !f.userMCP["zenify-plane"] {
		t.Error("user zenify-plane removed although no local one was registered")
	}
}

// A machine with no stamp and no prior install: the pre-install resync fails
// as on the real CLI, the local install succeeds, and up succeeds.
func TestUp_FreshMachineSucceeds(t *testing.T) {
	t.Setenv("ZENIFY_HOME", t.TempDir())
	home, ws := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(filepath.Join(ws, "svc", ".git", "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Org: "MyOrg", Repos: []manifest.Repo{{Name: "svc", Path: "svc"}}}
	plans := []reconcile.RepoPlan{{Name: "svc", State: reconcile.Wire, Path: "svc"}}
	f := newFakeClaude(ws)
	useFakeClaude(t, f)
	var errOut strings.Builder
	if err := runApply(io.Discard, &errOut, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err != nil {
		t.Fatalf("runApply on a fresh machine: %v", err)
	}
	if !f.installed {
		t.Error("local plugin not installed")
	}
	if strings.Contains(errOut.String(), "znf ensure: plugin") || strings.Contains(errOut.String(), "claude plugin") {
		t.Errorf("fresh machine printed plugin failures: %q", errOut.String())
	}
}

// FR-5.5: plugin enabled, stamp stale, `plugin update` failing → up fails,
// nothing removed, the stamp stays stale (so the next up retries).
func TestUp_StaleUpdateFailureKeepsUserInstalls(t *testing.T) {
	home, ws, m, plans := upFixture(t)
	writeStamp(t, ws, "v0.0.1")
	f := newFakeClaude(ws)
	f.installed, f.mktAdded, f.failUpdate = true, true, true
	useFakeClaude(t, f)
	if err := runApply(io.Discard, io.Discard, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err == nil {
		t.Fatal("runApply succeeded although plugin update failed")
	}
	if got := f.userRemoves(); len(got) > 0 {
		t.Errorf("user MCP removed: %v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf")); err != nil {
		t.Errorf("skills/znf removed: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "plugin-version")); string(b) != "v0.0.1" {
		t.Errorf("stamp = %q, want stale", b)
	}
}

// FR-5.5 (spec testing line): a failed broker MCP registration keeps the user install.
func TestUp_BrokerMCPFailureKeepsUserInstalls(t *testing.T) {
	home, ws, m, plans := upFixture(t)
	p := filepath.Join(t.TempDir(), "repos.yaml")
	if err := os.WriteFile(p, []byte(noBrokerYAML+"broker:\n  url: https://broker.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	useTrustedManifest(t, p)
	f := newFakeClaude(ws)
	f.failAddJSON = true
	useFakeClaude(t, f)
	var errOut strings.Builder
	if err := runApply(io.Discard, &errOut, plans, m, "embedded", ws, &fakeGH{}, &fakeGit{}); err == nil {
		t.Fatal("runApply succeeded although mcp add-json failed")
	}
	if !strings.Contains(errOut.String(), "broker MCP") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if got := f.userRemoves(); len(got) > 0 {
		t.Errorf("user MCP removed: %v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "znf")); err != nil {
		t.Errorf("skills/znf removed: %v", err)
	}
}

// FR-8.2: `up --reviewer codex` writes the workspace settings.local.json, never ~/.claude/settings.json.
func TestUp_ReviewerWritesWorkspaceLocal(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	// The missing manifest stops up right after the reviewer write.
	root.SetArgs([]string{"up", "--apply", "--workspace", ws, "--manifest", filepath.Join(ws, "missing.yaml"), "--reviewer", "codex"})
	_ = root.Execute()
	v, err := apply.ReviewerEnvValue(apply.WorkspaceLocalSettings(ws))
	if err != nil || v != "codex" {
		t.Fatalf("settings.local.json ZNF_REVIEWER = %q, %v", v, err)
	}
	if _, err := os.Stat(apply.UserSettings(home)); !os.IsNotExist(err) {
		t.Fatalf("~/.claude/settings.json written: %v", err)
	}
}
