package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
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

// registerDefaultChecks wires the foundation-layer checks. Called once at root
// construction. Uses os.Getenv and the workspace default settings path.
func registerDefaultChecks() {
	RegisterCheck(secretPresenceCheck(os.Getenv, defaultDoctorSettingsPath(os.Getenv)))
	RegisterCheck(toolPresenceCheck([]string{"git", "gh", "mongosh", "mysql"}))
	RegisterCheck(playwrightCheck())
}

func defaultDoctorSettingsPath(getenv func(string) string) string {
	return getenv("HOME") + "/WorkingSpace/zenify/.claude/settings.local.json"
}
