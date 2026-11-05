package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// SC-25: default preview does NOT write. SC-26: non-goals stay untouched.
func TestDown_DryRun_NoWritesAndNonGoalsUntouched(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// global settings.json has 1 znf hook.
	gs := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(gs), 0o750); err != nil {
		t.Fatal(err)
	}
	gsBody := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"zenify hooks-run docs-sync"}]}],"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"zenify git-guard"}]}]}}`)
	if err := os.WriteFile(gs, gsBody, 0o600); err != nil {
		t.Fatal(err)
	}

	// manifest repos.yaml minimally 1 repo "svc" + repo dir with exclude + settings.
	repoDir := filepath.Join(ws, "svc")
	excl := filepath.Join(repoDir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excl), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excl, []byte(".worktrees/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(repoDir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// ownership manifest records settings as owned.
	zdir := filepath.Join(ws, ".zenify")
	if err := os.MkdirAll(zdir, 0o750); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{Entries: map[string]managed.Entry{}}
	_ = owned.Record(settings)
	if err := owned.Save(filepath.Join(zdir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	// non-goals: cloned code file + docs store.
	code := filepath.Join(repoDir, "main.go")
	if err := os.WriteFile(code, []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	repos := filepath.Join(ws, "manifest", "repos.yaml")
	if err := os.MkdirAll(filepath.Dir(repos), 0o750); err != nil {
		t.Fatal(err)
	}
	// minimal valid input for manifest.LoadWithOverlay — 1 repo path "svc".
	if err := os.WriteFile(repos, []byte("org: TestOrg\nrepos:\n  - name: svc\n    path: svc\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var prevOut bytes.Buffer
	cmd := newDownCmd()
	cmd.SetOut(&prevOut)
	cmd.SetArgs([]string{"--workspace", ws, "--manifest", repos}) // NO --apply → preview
	if err := cmd.Execute(); err != nil {
		t.Fatalf("down preview: %v", err)
	}

	for _, step := range []string{"codex-hooks", "codex-skills"} {
		if !strings.Contains(prevOut.String(), step) {
			t.Errorf("preview must list step %q: %s", step, prevOut.String())
		}
	}

	// SC-25: must not write — global settings.json, exclude, settings.local.json unchanged.
	if b, _ := os.ReadFile(gs); !bytes.Equal(b, gsBody) { //nolint:gosec // G304 -- test-local path under t.TempDir, not externally-tainted
		t.Error("preview must NOT modify global settings.json")
	}
	if b, _ := os.ReadFile(excl); string(b) != ".worktrees/\n" { //nolint:gosec // G304 -- test-local path under t.TempDir, not externally-tainted
		t.Error("preview must NOT modify exclude")
	}
	if _, err := os.Stat(settings); err != nil {
		t.Error("preview must NOT delete settings.local.json")
	}
	// SC-26: non-goals stay untouched.
	if _, err := os.Stat(code); err != nil {
		t.Error("cloned code file must stay untouched")
	}
	if _, err := os.Stat(filepath.Join(zdir, "manifest.json")); err != nil {
		t.Error(".zenify/manifest.json must stay untouched")
	}

	// --apply removes the git-guard hook that up/SessionStart installed.
	applyCmd := newDownCmd()
	applyCmd.SetOut(&bytes.Buffer{})
	applyCmd.SetArgs([]string{"--workspace", ws, "--manifest", repos, "--apply"})
	if err := applyCmd.Execute(); err != nil {
		t.Fatalf("down --apply: %v", err)
	}
	if ok, err := guardInstalled(guardSettingsPath(home)); err != nil || ok {
		t.Errorf("down --apply must remove git-guard, installed=%v err=%v", ok, err)
	}
}

// SC-13: down --apply removes this workspace's .zenify/.env deny rule from
// ~/.claude/settings.json and keeps the user's own deny rule and another
// workspace's kit rule.
func TestDown_Apply_RemovesKitDeny(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	userSettings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(userSettings), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userSettings, []byte(`{"permissions":{"deny":["Read(/secret)"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{ws, other} {
		if _, err := apply.EnsureKitDeny(home, w, false); err != nil {
			t.Fatal(err)
		}
	}
	repos := filepath.Join(ws, "manifest", "repos.yaml")
	if err := os.MkdirAll(filepath.Dir(repos), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repos, []byte("org: TestOrg\nrepos:\n  - name: svc\n    path: svc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newDownCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--workspace", ws, "--manifest", repos, "--apply"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("down --apply: %v", err)
	}
	b, err := os.ReadFile(userSettings) //nolint:gosec // G304 -- test temp path
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "Read(/"+ws+"/.zenify/.env)") || !strings.Contains(s, "Read(/secret)") || !strings.Contains(s, "Read(/"+other+"/.zenify/.env)") {
		t.Errorf("deny after down = %s", b)
	}
}

// downFakeClaude answers every claude call with success and records the args.
func downFakeClaude(t *testing.T, calls *[]string, fail func(args string) (string, bool)) {
	t.Helper()
	orig := pluginRunner
	pluginRunner = func(string) func(string, ...string) ([]byte, error) {
		return func(_ string, args ...string) ([]byte, error) {
			a := strings.Join(args, " ")
			*calls = append(*calls, a)
			if fail != nil {
				if out, bad := fail(a); bad {
					return []byte(out), errors.New("exit status 1")
				}
			}
			return nil, nil
		}
	}
	t.Cleanup(func() { pluginRunner = orig })
}

func downWorkspace(t *testing.T) (ws, home, repos string) {
	t.Helper()
	ws, home = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", filepath.Join(home, ".zenify"))
	repos = filepath.Join(ws, "manifest", "repos.yaml")
	writeF(t, repos, "org: TestOrg\nrepos:\n  - name: svc\n    path: svc\n")
	return ws, home, repos
}

func runDown(t *testing.T, ws, repos string, extra ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newDownCmd()
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{"--workspace", ws, "--manifest", repos}, extra...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("down: %v", err)
	}
	return out.String()
}

func TestDown_KeepsLocalSettingsFile(t *testing.T) {
	ws, home, repos := downWorkspace(t)
	writeF(t, filepath.Join(home, ".claude.json"), `{"projects":{"`+ws+`":{"mcpServers":{`+kitMCPEntries+`}}}}`)
	local := apply.WorkspaceLocalSettings(ws)
	writeF(t, local, `{"autoMemoryDirectory":"/m","hooks":{"Stop":[{"hooks":[{"type":"command","command":"zenify hooks-run docs-sync"}]}]}}`)
	writeF(t, pluginStampPath(ws), "v1")
	var calls []string
	downFakeClaude(t, &calls, nil)

	runDown(t, ws, repos, "--apply")

	b, err := os.ReadFile(local) //nolint:gosec // G304 -- test temp path
	if err != nil {
		t.Fatalf("settings.local.json must stay: %v", err)
	}
	if !strings.Contains(string(b), "autoMemoryDirectory") || strings.Contains(string(b), "zenify hooks-run") {
		t.Errorf("settings.local.json after down = %s", b)
	}
	if !slices.Contains(calls, "plugin uninstall znf@zenify-kit --scope local") {
		t.Errorf("plugin uninstall not run: %v", calls)
	}
	if !slices.Contains(calls, "plugin marketplace remove zenify-kit") {
		t.Errorf("marketplace remove not run: %v", calls)
	}
	for _, n := range []string{"zenify-grafana", "zenify-posthog", "zenify-plane"} {
		if !slices.Contains(calls, "mcp remove "+n+" -s local") {
			t.Errorf("mcp remove %s -s local not run: %v", n, calls)
		}
	}
	if _, err := os.Stat(pluginStampPath(ws)); !os.IsNotExist(err) {
		t.Errorf("plugin stamp must be removed, stat err=%v", err)
	}
}

// Real claude 2.1.318 answers, measured 2026-11-05: already-clean, not failures.
func TestDown_AlreadyCleanAnswersAreIdempotent(t *testing.T) {
	ws, _, repos := downWorkspace(t)
	writeF(t, pluginStampPath(ws), "v1")
	var calls []string
	downFakeClaude(t, &calls, func(a string) (string, bool) {
		switch {
		case strings.HasPrefix(a, "plugin uninstall"):
			return `✘ Failed to uninstall plugin "znf@zenify-kit": Plugin "znf@zenify-kit" is not installed in local scope. Use --scope to specify the correct scope.`, true
		case strings.HasPrefix(a, "plugin marketplace remove"):
			return `✘ Failed to remove marketplace: Marketplace 'zenify-kit' not found`, true
		case strings.HasPrefix(a, "mcp remove"):
			return `No MCP server named "x" in local scope`, true
		}
		return "", false
	})
	out := runDown(t, ws, repos, "--apply")
	if strings.Contains(out, "bỏ qua (exit status") { //znf:allow-lang
		t.Errorf("already-clean answers must not warn: %s", out)
	}
	if _, err := os.Stat(pluginStampPath(ws)); !os.IsNotExist(err) {
		t.Errorf("stamp must go when the plugin is already gone")
	}
}

func TestDown_OtherClaudeErrorsSurface(t *testing.T) {
	ws, _, repos := downWorkspace(t)
	writeF(t, pluginStampPath(ws), "v1")
	var calls []string
	downFakeClaude(t, &calls, func(a string) (string, bool) {
		switch {
		case strings.HasPrefix(a, "plugin uninstall"):
			return "boom", true
		case strings.HasPrefix(a, "plugin marketplace remove"):
			return "zsh: command not found: claude", true // must not match the marketplace matcher
		}
		return "", false
	})
	out := runDown(t, ws, repos, "--apply")
	if !strings.Contains(out, "boom") || !strings.Contains(out, "command not found") {
		t.Errorf("other errors must surface: %s", out)
	}
	if _, err := os.Stat(pluginStampPath(ws)); err != nil {
		t.Errorf("stamp must stay when uninstall failed: %v", err)
	}
}

func TestMarketplaceNotFound(t *testing.T) {
	if !marketplaceNotFound(`✘ Failed to remove marketplace: Marketplace 'zenify-kit' not found`) {
		t.Error("real not-found text must match")
	}
	if marketplaceNotFound("zsh: command not found: claude") {
		t.Error("a missing claude binary must not match")
	}
}

// down's --workspace defaults to ".", but up keyed the Codex trust entry by the
// absolute path: down must resolve it, or the entry is never found.
func TestDown_RelativeWorkspaceRemovesCodexTrust(t *testing.T) {
	ws, home, repos := downWorkspace(t)
	t.Setenv("CODEX_HOME", "")
	codexHome := filepath.Join(home, ".codex")
	if _, err := apply.EnsureCodexTrust(codexHome, ws, false); err != nil {
		t.Fatal(err)
	}
	var calls []string
	downFakeClaude(t, &calls, nil)
	t.Chdir(ws)

	runDown(t, ".", repos, "--apply")

	b, _ := os.ReadFile(filepath.Join(codexHome, "config.toml")) //nolint:gosec // G304 -- test temp path
	if strings.Contains(string(b), ws) {
		t.Errorf("codex trust for %s left after down: %s", ws, b)
	}
}

// A server the user configured under a kit name stays through down.
func TestDown_KeepsCustomLocalMCP(t *testing.T) {
	ws, home, repos := downWorkspace(t)
	writeF(t, filepath.Join(home, ".claude.json"), `{"projects":{"`+ws+`":{"mcpServers":{"zenify-posthog":{"type":"http","url":"https://mine.example/ph"}}}}}`)
	var calls []string
	downFakeClaude(t, &calls, nil)
	runDown(t, ws, repos, "--apply")
	if slices.Contains(calls, "mcp remove zenify-posthog -s local") {
		t.Errorf("custom zenify-posthog removed: %v", calls)
	}
}
