package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
)

// secretPresenceCheck reports which credential KEYS are resolvable (env or
// <workspace>/.zenify/.env) — names and present/absent ONLY. Per FR-041 it
// never places a credential VALUE into its output. It covers MONGO_URL plus
// every global manifest key (FR-8.1); repo-scoped keys are not reported.
// ok = MONGO_URL present (the one key db-read hard-requires); the rest are
// informational. Production keys are skipped (filled by hand, never required);
// an empty `KEY=` line counts as absent.
func secretPresenceCheck(getenv func(string) string, envPath func() string, kitManifest func() *manifest.Manifest) Check {
	return Check{
		Name: "secrets",
		Run: func() (bool, string) {
			if brokerDeclared() {
				return true, "credentials via broker"
			}
			path := envPath()
			creds, _ := dbread.LoadCreds(getenv, path)
			var f *devenv.File
			if path != "" {
				f, _ = devenv.Load(path)
			}
			lookup := devenv.Lookup(getenv, f)
			keys := []string{"MONGO_URL"}
			if m := kitManifest(); m != nil {
				for _, k := range m.SecretKeys {
					if k.Name != "MONGO_URL" && k.EffectiveGroup() != manifest.GroupProduction {
						keys = append(keys, k.Name)
					}
				}
			}
			var parts []string
			for _, k := range keys {
				state := "absent"
				if lookup(k) != "" {
					state = "present"
				}
				parts = append(parts, k+"="+state) // key + state only; NEVER the value
			}
			detail := strings.Join(parts, " ")
			if creds["MONGO_URL"] == "" && path == "" {
				detail += " — no workspace: run `zenify up` (or cd into the workspace)"
			}
			ok := creds["MONGO_URL"] != ""
			return ok, detail
		},
	}
}

// brokerCheckTimeout bounds the whole broker check, both gh calls included.
var brokerCheckTimeout = 30 * time.Second

// brokerCheck verifies the three things a broker call needs, in order: a gh
// login with read:org, a broker reachable within 5s (network or firewall is the
// usual cause when it is not), and a fresh token exchange — never the cache, so org
// membership is really re-checked. It prints the login the broker issued the
// token to — never a token.
func brokerCheck(url string, gh ghx.Runner, probe *http.Client, client *brokerclient.Client) Check {
	return Check{
		Name: "broker",
		Run: func() (bool, string) {
			dctx, dcancel := context.WithTimeout(context.Background(), brokerCheckTimeout)
			defer dcancel()
			type authResult struct {
				auth ghx.Auth
				err  error
			}
			ch := make(chan authResult, 1)
			go func() { a, err := ghx.CheckAuth(gh); ch <- authResult{a, err} }()
			var auth ghx.Auth
			var err error
			select {
			case r := <-ch:
				auth, err = r.auth, r.err
			case <-dctx.Done():
				return false, "gh auth status quá thời gian" //znf:allow-lang
			}
			if err != nil || !auth.LoggedIn {
				return false, "gh chưa đăng nhập — chạy `gh auth login`" //znf:allow-lang
			}
			if !auth.HasScopes("read:org") {
				return false, "gh thiếu scope read:org — chạy `gh auth refresh -s read:org`" //znf:allow-lang
			}
			ctx, cancel := context.WithTimeout(dctx, 5*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(url, "/")+"/healthz", nil)
			if err != nil {
				return false, "broker url không hợp lệ" //znf:allow-lang
			}
			resp, err := probe.Do(req) //nolint:gosec // G704 -- url comes from the kit manifest, https-only (manifest.Load)
			if err != nil {
				return false, "không tới được broker — kiểm tra mạng và broker.url trong manifest rồi chạy lại" //znf:allow-lang
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false, fmt.Sprintf("broker /healthz trả %d", resp.StatusCode) //znf:allow-lang
			}
			_, login, role, err := client.Refresh(dctx)
			if err != nil {
				var ae *brokerclient.APIError
				switch {
				case errors.As(err, &ae) && ae.Code == api.CodeBrokerMisconfigured:
					return false, "broker cấu hình sai, báo người quản trị broker: " + ae.Message //znf:allow-lang
				case errors.As(err, &ae):
					return false, fmt.Sprintf("broker từ chối token: %d %s: %s", ae.Status, ae.Code, ae.Message) //znf:allow-lang
				}
				return false, "không lấy được token từ broker" //znf:allow-lang
			}
			return true, "broker ok, login=" + login + ", role=" + role
		},
	}
}

// retiredBrokerKeys are the hand-written .zenify/.env keys the credential
// broker replaces. When the trusted manifest declares a broker and no longer
// declares them, they are reported as unused, never as a failure; without a
// broker db-read still reads them, so they are ordinary custom keys.
var retiredBrokerKeys = map[string]bool{
	"MONGO_URL": true, "MYSQL_URL": true, "E2E_DOMAIN": true, "E2E_EMAIL": true, "E2E_PASSWORD": true,
	"POSTHOG_API_KEY": true, "MONGO_URL_PRD": true, "MYSQL_URL_PRD": true, "AUDIT_MONGO_URL_PRD": true, "REDIS_URL_PRD": true,
	"LUMI_MYSQL_URL_STG": true, "LUMI_MYSQL_URL_PRD": true, "QDRANT_URL_STG": true, "QDRANT_URL_PRD": true, "LUMI_APP_URL_STG": true,
}

// envFileCheck inspects <workspace>/.zenify/.env itself: a mode wider than 0600
// (FR-8.4) and custom keys — keys the manifest does not know — that carry no
// description line above them (FR-8.2). Names only, never values. The mode
// check is skipped when goos is "windows", which has no POSIX mode bits.
func envFileCheck(envPath func() string, kitManifest func() *manifest.Manifest, goos string) Check {
	return Check{
		Name: "env-file",
		Run: func() (bool, string) {
			path := envPath()
			if path == "" {
				return true, "no workspace"
			}
			fi, err := os.Stat(path)
			if err != nil {
				return true, "chưa có .zenify/.env" //znf:allow-lang
			}
			f, err := devenv.Load(path)
			if err != nil {
				return false, "cannot read " + path
			}
			var problems []string
			if perm := fi.Mode().Perm(); goos != "windows" && perm&0o077 != 0 {
				problems = append(problems, fmt.Sprintf("%s has mode %04o, want 0600 — run: chmod 600 %s", path, perm, path))
			}
			known := map[string]bool{}
			if m := kitManifest(); m != nil {
				for _, k := range m.AllKeys() {
					known[k.Name] = true
				}
			}
			for _, k := range apply.LegacyMySQLKeys {
				known[k] = true
			}
			var undescribed, legacy, retired []string
			viaBroker := brokerDeclared()
			for _, k := range f.Keys() {
				switch {
				case known[k]:
				case viaBroker && retiredBrokerKeys[k]:
					retired = append(retired, k)
				case f.Desc(k) == "":
					undescribed = append(undescribed, k)
				}
			}
			for _, k := range apply.LegacyMySQLKeys {
				if _, ok := f.Get(k); ok {
					legacy = append(legacy, k)
				}
			}
			if len(legacy) > 0 {
				problems = append(problems, "legacy MYSQL_* in .zenify/.env ("+strings.Join(legacy, ", ")+") — run zenify up --apply to merge into MYSQL_URL")
			}
			if len(undescribed) > 0 {
				problems = append(problems, "custom keys without a description: "+strings.Join(undescribed, ", ")+" — add one with: zenify env set <KEY> --desc <text>")
			}
			if len(problems) > 0 {
				return false, strings.Join(problems, "; ")
			}
			detail := "mode 0600, custom keys described"
			if goos == "windows" {
				detail = "custom keys described"
			}
			if len(retired) > 0 {
				detail += "; " + strings.Join(retired, ", ") + " không còn dùng, có thể xoá" //znf:allow-lang
			}
			return true, detail
		},
	}
}

// settingsEnvCheck names the manifest keys that still hold a value in the
// "env" block of the workspace, a repo or a repo worktree settings.local.json
// (FR-8.3) — they
// belong in .zenify/.env. Key names and file paths only, never values.
func settingsEnvCheck(envPath func() string, kitManifest func() *manifest.Manifest) Check {
	return Check{
		Name: "settings-env",
		Run: func() (bool, string) {
			path := envPath()
			m := kitManifest()
			if path == "" || m == nil {
				return true, "no workspace"
			}
			ws := filepath.Dir(filepath.Dir(path)) // envPath is <ws>/.zenify/.env
			var repoPaths []string
			for _, r := range m.Repos {
				repoPaths = append(repoPaths, r.Path)
			}
			paths, unreadable := settingsEnvPaths(ws, repoPaths)
			left, err := apply.LeftoverEnvKeys(paths)
			if err != nil {
				return false, err.Error()
			}
			keys := apply.MigrationKeys(manifest.KeyNames(m.AllKeys()))
			known := map[string]bool{}
			for _, k := range keys {
				known[k] = true
			}
			// Keys the migration skips by design need a manual fix; `up` would
			// not move them.
			preview, err := apply.PreviewSettingsEnv(paths, keys, path)
			if err != nil {
				return false, err.Error()
			}
			manual := map[string]bool{}
			for _, k := range append(preview.Conflicts, preview.Skipped...) {
				manual[k] = true
			}
			var movable, byHand []string
			for _, p := range paths {
				var mv, bh []string
				for _, k := range left[p] {
					switch {
					case !known[k]:
					case manual[k]:
						bh = append(bh, k)
					default:
						mv = append(mv, k)
					}
				}
				if len(mv) > 0 {
					movable = append(movable, strings.Join(mv, ", ")+" in "+p)
				}
				if len(bh) > 0 {
					byHand = append(byHand, strings.Join(bh, ", ")+" in "+p)
				}
			}
			var problems []string
			if len(movable) > 0 {
				problems = append(problems, strings.Join(movable, "; ")+" — run `zenify up --apply` to move them to .zenify/.env")
			}
			if len(byHand) > 0 {
				problems = append(problems, strings.Join(byHand, "; ")+" — fix by hand (values differ between files or .zenify/.env, or a value is not a one-line string): keep one with `zenify env set <KEY>`, then delete the key from the settings env")
			}
			if len(unreadable) > 0 {
				problems = append(problems, strings.Join(unreadable, "; ")+" — worktree settings file does not load (invalid JSON or \"env\" not an object), skipped: fix by hand")
			}
			if len(problems) > 0 {
				return false, strings.Join(problems, "; ")
			}
			return true, "no credential left in settings env"
		},
	}
}

// workspaceManifest loads the kit manifest for the workspace resolved from cwd;
// nil when there is no workspace or the manifest cannot be loaded.
func workspaceManifest() *manifest.Manifest {
	cwd, _ := os.Getwd()
	ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
	if !ok {
		return nil
	}
	m, _, err := loadKitManifest("", filepath.Join(ws, ".zenify-overlay.yaml"))
	if err != nil {
		return nil
	}
	return m
}

// toolPresenceCheck reports whether each external CLI db-read shells out to is
// on PATH. Read-only: LookPath does not execute the tool.
func toolPresenceCheck(tools []string) Check {
	sorted := append([]string(nil), tools...)
	sort.Strings(sorted)
	return Check{
		Name: "tools",
		Run: func() (bool, string) {
			ok := true
			var parts []string
			for _, t := range sorted {
				if _, err := exec.LookPath(t); err != nil {
					ok = false
					parts = append(parts, fmt.Sprintf("%s=missing", t))
				} else {
					parts = append(parts, fmt.Sprintf("%s=ok", t))
				}
			}
			return ok, strings.Join(parts, " ")
		},
	}
}

// playwrightCheck reports whether the Playwright MCP is registered and whether
// browser binaries are present — READ-ONLY, it never launches a browser (FR-012/
// FR-066). ok = MCP registered; browser absence is surfaced in the detail rather
// than failing, since the MCP server lazy-installs.
func playwrightCheck() Check {
	return Check{
		Name: "playwright",
		Run: func() (bool, string) {
			home, herr := os.UserHomeDir()
			if herr != nil || home == "" {
				return false, fmt.Sprintf("cannot resolve home: %v", herr)
			}
			o := playwright.Options{
				Runner: func(name string, args []string) error { return exec.Command(name, args...).Run() },              //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
				Output: func(name string, args []string) ([]byte, error) { return exec.Command(name, args...).Output() }, //nolint:gosec // G204 -- fixed trusted binary, internally-computed args
				Home:   home,
				Getenv: os.Getenv,
				GOOS:   runtime.GOOS,
			}
			reg, _, detail := playwright.Status(o)
			if codexOnPath() {
				o.Target = playwright.TargetCodex
				cs, _ := playwright.Registration(o)
				detail += fmt.Sprintf("; codex mcp=%s", cs) //znf:allow-lang
			}
			return reg, detail // ok = MCP registered; browser absence shown in detail
		},
	}
}

// pluginCheck reports whether the znf plugin has materialized: plugin.json exists
// + is valid, the number of skills/agents shipped, and the number of files managed.
func pluginCheck() Check {
	dest, _ := plugin.DefaultDest()
	return pluginCheckAt(dest)
}

func pluginCheckAt(dest string) Check {
	return Check{
		Name: "znf-plugin",
		Run: func() (bool, string) {
			manifestPath := filepath.Join(dest, ".manifest.json")
			pj := filepath.Join(dest, ".claude-plugin", "plugin.json")
			b, err := os.ReadFile(pj) //nolint:gosec // G304 -- internal dest
			if err != nil {
				return false, "znf chưa cài (chạy `zenify skills sync`)" //znf:allow-lang
			}
			var meta struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(b, &meta) != nil || meta.Name != "znf" {
				return false, "plugin.json không hợp lệ" //znf:allow-lang
			}
			m, err := managed.Load(manifestPath)
			if err != nil {
				return true, "ok — plugin.json hợp lệ (manifest không đọc được)" //znf:allow-lang
			}
			skills := countDirs(filepath.Join(dest, "skills"))
			agents := countFiles(filepath.Join(dest, "agents"))
			return true, fmt.Sprintf("ok — %d skill, %d agent, %d file managed", skills, agents, len(m.Entries))
		},
	}
}

// countDirs counts child directories that HAVE a SKILL.md (real skills); auxiliary
// dirs like _shared are skipped; error → 0.
func countDirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil {
			n++
		}
	}
	return n
}

// countFiles counts direct regular files; error → 0.
func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

// dockerCheck reports whether Docker is available for `zenify visual`: docker on
// PATH + daemon running (`docker info`). Docker is a prerequisite of visual-regression;
// it is NOT auto-installed (system daemon, needs admin/reboot/license per-OS) — this
// only reports.
func dockerCheck() Check {
	return dockerCheckWith(exec.LookPath, func() error {
		return exec.Command("docker", "info").Run() //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	})
}

func dockerCheckWith(lookPath func(string) (string, error), info func() error) Check {
	return Check{
		Name: "docker",
		Run: func() (bool, string) {
			if _, err := lookPath("docker"); err != nil {
				return false, "docker=missing — cài để dùng `zenify visual` (mac: brew install --cask docker · win: winget install Docker.DockerDesktop · linux: apt install docker.io)" //znf:allow-lang
			}
			if err := info(); err != nil {
				return false, "docker=installed daemon=down — mở Docker Desktop / start dockerd" //znf:allow-lang
			}
			return true, "docker=ok daemon=up"
		},
	}
}

// gitGuardCheck reports whether the git-guard PreToolUse hook is wired in
// ~/.claude/settings.json. Fix installs it (the same idempotent edit as
// `zenify guard install`).
func gitGuardCheck(home func() (string, error)) Check {
	return Check{
		Name: "git-guard",
		Run: func() (bool, string) {
			h, err := home()
			if err != nil {
				return false, "cannot resolve HOME"
			}
			ok, err := guardInstalled(h)
			if err != nil {
				return false, fmt.Sprintf("settings.json unreadable: %v", err)
			}
			if !ok {
				return false, "hook=missing — commit/push to deploy branches is NOT blocked; run `zenify doctor --fix` or `zenify guard install`"
			}
			return true, "hook=installed"
		},
		Fix: func() (bool, string) {
			h, err := home()
			if err != nil {
				return false, "cannot resolve HOME"
			}
			if _, err := installGuard(h); err != nil {
				return false, err.Error()
			}
			return true, "wired PreToolUse → zenify git-guard"
		},
	}
}

// kitPermissionsCheck reports whether the kit's Playwright/e2e allow rules and
// sandbox exclusions are in ~/.claude/settings.json. Fix adds the missing ones
// (same union as `up`).
func kitPermissionsCheck(home func() (string, error)) Check {
	return Check{
		Name: "kit-permissions",
		Run: func() (bool, string) {
			h, err := home()
			if err != nil {
				return false, "cannot resolve HOME"
			}
			n, err := apply.EnsureKitPermissions(h, true)
			if err != nil {
				return false, fmt.Sprintf("settings.json unreadable: %v", err)
			}
			s, err := apply.EnsureKitSandbox(h, true)
			if err != nil {
				return false, fmt.Sprintf("settings.json unreadable: %v", err)
			}
			if n > 0 || s > 0 {
				return false, fmt.Sprintf("missing %d kit allow rules, %d sandbox exclusions; run `zenify doctor --fix` or `zenify up`", n, s)
			}
			return true, "allow rules and sandbox exclusions present"
		},
		Fix: func() (bool, string) {
			h, err := home()
			if err != nil {
				return false, "cannot resolve HOME"
			}
			n, err := apply.EnsureKitPermissions(h, false)
			if err != nil {
				return false, err.Error()
			}
			s, err := apply.EnsureKitSandbox(h, false)
			if err != nil {
				return false, err.Error()
			}
			return true, fmt.Sprintf("added %d kit allow rules, %d sandbox exclusions", n, s)
		},
	}
}

// gitIdentityCheck reports whether git can author a commit. It asks git
// itself (`git var GIT_AUTHOR_IDENT` / `GIT_COMMITTER_IDENT`, which fail
// exactly when `git commit` would), so GIT_AUTHOR_* / GIT_COMMITTER_* env
// vars and repo-local config count, not just global user.name/user.email.
// Without an identity every `zenify docs sync` commit fails with exit 128
// and the knowledge store silently stops syncing.
func gitIdentityCheck() Check {
	return gitIdentityCheckWith(func(v string) bool {
		return exec.Command("git", "var", v).Run() == nil //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	})
}

func gitIdentityCheckWith(identOK func(gitVar string) bool) Check {
	return Check{
		Name: "git-identity",
		Run: func() (bool, string) {
			var missing []string
			for _, v := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
				if !identOK(v) {
					missing = append(missing, v)
				}
			}
			if len(missing) > 0 {
				return false, strings.Join(missing, " ") + "=unknown — `zenify docs sync` cannot commit; run: git config --global user.name <name> && git config --global user.email <email>"
			}
			return true, "author=ok committer=ok"
		},
	}
}

// registerDefaultChecks wires the foundation-layer checks. Called once at root
// construction. Uses os.Getenv and the workspace default settings path.
func registerDefaultChecks() {
	RegisterCheck(secretPresenceCheck(os.Getenv, workspaceEnvPath, workspaceManifest))
	RegisterCheck(envFileCheck(workspaceEnvPath, workspaceManifest, runtime.GOOS))
	RegisterCheck(settingsEnvCheck(workspaceEnvPath, workspaceManifest))
	RegisterCheck(brokerCredsCheck())
	RegisterCheck(dbToolsCheck())
	RegisterCheck(playwrightCheck())
	RegisterCheck(kitPermissionsCheck(os.UserHomeDir))
	RegisterCheck(dockerCheck())
	RegisterCheck(gitGuardCheck(os.UserHomeDir))
	RegisterCheck(gitIdentityCheck())
	RegisterCheck(pluginCheck())
	registerCodexChecks()
	registerContextChecks()
}

// brokerCheckOnce registers the broker check on the first doctor run, so the
// embedded manifest is parsed only by commands that need it, not by every
// zenify invocation.
var brokerCheckOnce sync.Once

// registerBrokerCheck adds the `broker` check only when the trusted manifest
// (brokerManifest: embedded, never cwd) declares a broker.
func registerBrokerCheck() {
	m, err := brokerManifest("", "")
	if err != nil {
		return
	}
	c, ok := newBrokerClient(m)
	if !ok {
		return
	}
	RegisterCheck(brokerCheck(c.URL, currentGH{}, brokerclient.NewHTTPClient(5*time.Second), c))
}

// dbToolsCheck is the tools check: with a broker, db-read runs on the broker,
// so mongosh and mysql are not required.
func dbToolsCheck() Check {
	local := toolPresenceCheck([]string{"git", "gh", "mongosh", "mysql"})
	viaBroker := toolPresenceCheck([]string{"git", "gh"})
	return Check{Name: local.Name, Run: func() (bool, string) {
		if brokerDeclared() {
			return viaBroker.Run()
		}
		return local.Run()
	}}
}

// brokerCredsCheckTimeout bounds the one /creds call of the broker-creds check.
const brokerCredsCheckTimeout = 5 * time.Second

// brokerCredsCheck asks the trusted-manifest broker for staging read
// credentials and names the engines it vends (sorted keys) — never a value.
// Without a broker there is nothing to verify, so it skips.
func brokerCredsCheck() Check {
	return Check{
		Name: "broker-creds",
		Run: func() (bool, string) {
			bc, ok, err := brokerFromFlags("", "")
			if err != nil || !ok {
				return true, "skip: không có broker" //znf:allow-lang
			}
			ctx, cancel := context.WithTimeout(context.Background(), brokerCredsCheckTimeout)
			defer cancel()
			var resp api.CredsResponse
			if err := bc.Post(ctx, "/creds", api.CredsRequest{Env: "stg", Mode: api.CredsModeRead}, &resp); err != nil {
				var ae *brokerclient.APIError
				if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
					return true, "skip: broker chưa có /creds — cần nâng cấp broker" //znf:allow-lang
				}
				return false, "broker không cấp được credential staging: " + brokerCallErr(err).Error() //znf:allow-lang
			}
			names := make([]string, 0, len(resp.Values))
			for k := range resp.Values {
				names = append(names, k)
			}
			sort.Strings(names)
			return true, "broker cấp credential staging: " + strings.Join(names, ", ") //znf:allow-lang
		},
	}
}
