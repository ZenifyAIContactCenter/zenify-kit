package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
)

// secretPresenceCheck reports which DB credential KEYS are resolvable (env or
// the settings.local.json env block) — names and present/absent ONLY. Per
// FR-041 it never places a credential VALUE into its output. ok = MONGO_URL
// present (the one key db-read hard-requires); the rest are informational.
func secretPresenceCheck(getenv func(string) string, settingsPath string) Check {
	return Check{
		Name: "secrets",
		Run: func() (bool, string) {
			creds, _ := dbread.LoadCreds(getenv, settingsPath)
			keys := []string{"MONGO_URL", "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE"}
			var parts []string
			for _, k := range keys {
				state := "absent"
				if creds[k] != "" {
					state = "present"
				}
				parts = append(parts, k+"="+state) // key + state only; NEVER the value
			}
			ok := creds["MONGO_URL"] != ""
			return ok, strings.Join(parts, " ")
		},
	}
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
			o := playwright.Options{
				Runner: func(name string, args []string) error { return exec.Command(name, args...).Run() }, //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
				Getenv: os.Getenv,
				GOOS:   runtime.GOOS,
			}
			reg, _, detail := playwright.Status(o)
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

// registerDefaultChecks wires the foundation-layer checks. Called once at root
// construction. Uses os.Getenv and the workspace default settings path.
func registerDefaultChecks() {
	RegisterCheck(secretPresenceCheck(os.Getenv, defaultDoctorSettingsPath(os.Getenv)))
	RegisterCheck(toolPresenceCheck([]string{"git", "gh", "mongosh", "mysql"}))
	RegisterCheck(playwrightCheck())
	RegisterCheck(dockerCheck())
	RegisterCheck(pluginCheck())
}

func defaultDoctorSettingsPath(getenv func(string) string) string {
	return getenv("HOME") + "/WorkingSpace/zenify/.claude/settings.local.json"
}
