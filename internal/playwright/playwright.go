// Package playwright bootstraps and inspects the Playwright MCP server and its
// browser binaries for zenify (FR-014/FR-066). It shells out to `claude` and
// `npx` through an injected Runner so every path is unit-testable without
// mutating the machine; Status is strictly read-only and never launches a
// browser.
package playwright

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Target selects which runtime's MCP registry the bootstrap manages.
type Target string

const (
	TargetClaude Target = "claude"
	TargetCodex  Target = "codex"
)

type Options struct {
	// Target is the runtime to register with; empty means TargetClaude.
	Target Target
	Runner func(name string, args []string) error
	// Output runs a command and returns its stdout (used to read `claude mcp get`).
	Output func(name string, args []string) ([]byte, error)
	// Home is the user's home dir; the MCP output dir is derived from it.
	Home   string
	Getenv func(string) string
	GOOS   string
	Stdout io.Writer
}

// browsersDir returns where Playwright stores browser binaries: the
// PLAYWRIGHT_BROWSERS_PATH override if set, else the per-OS default cache dir.
func browsersDir(getenv func(string) string, goos string) string {
	if p := getenv("PLAYWRIGHT_BROWSERS_PATH"); p != "" {
		return p
	}
	switch goos {
	case "darwin":
		return filepath.Join(getenv("HOME"), "Library", "Caches", "ms-playwright")
	case "windows":
		// Build with a literal backslash rather than filepath.Join: this binary
		// may be cross-run/tested on a non-Windows host where filepath.Join uses
		// "/", but a Windows browser cache path must use "\".
		base := getenv("LOCALAPPDATA")
		if base == "" {
			base = getenv("HOME") + `\AppData\Local`
		}
		return base + `\ms-playwright`
	default:
		return filepath.Join(getenv("HOME"), ".cache", "ms-playwright")
	}
}

// MCPVersion pins @playwright/mcp. The MCP server bundles its own
// playwright-core, whose browsers.json names the one chromium revision it can
// launch (0.0.83 → 1247). A bare `npx playwright install chromium` resolves a
// different playwright and installs a different revision (1248 on 2026-10-12),
// so the browser is installed through the pinned MCP package itself, and a new
// chromium download reaches users only when this constant is bumped.
const MCPVersion = "0.0.83"

// chromiumRevision maps MCPVersion to the chromium revision its bundled
// playwright-core pins (node_modules/playwright-core/browsers.json). Bump it
// with MCPVersion; a version with no entry installs on every run.
var chromiumRevision = map[string]string{"0.0.83": "1247"}

// MCPPackage is the npx package spec the kit registers and installs browsers with.
const MCPPackage = "@playwright/mcp@" + MCPVersion

// isKitPackage reports whether a is any @playwright/mcp@<spec>: the slot the
// kit owns, so an older pin (or the pre-pin @latest) migrates instead of
// being kept as a custom registration.
func isKitPackage(a string) bool { return strings.HasPrefix(a, "@playwright/mcp@") }

// DesiredArgs are the npx args the kit registers the Playwright MCP with.
// --isolated keeps the profile in memory so concurrent sessions do not fight
// over one persistent profile (login comes from the state file each verify);
// --caps=storage enables browser_set_storage_state; the output dir keeps
// artifacts under the kit's own home dir.
func DesiredArgs(home string) []string {
	return []string{MCPPackage, "--isolated", "--caps=storage", "--output-dir", filepath.Join(home, ".zenify", "playwright")}
}

// RegState classifies the current Playwright MCP registration.
type RegState int

const (
	RegAbsent RegState = iota
	RegLegacyDefault
	RegCustom
	RegDesired
)

func (s RegState) String() string {
	return [...]string{"absent", "legacy-default", "custom", "desired"}[s]
}

// Registration parses `claude mcp get playwright`. A failing command is
// RegAbsent; anything unparseable or differing from the two kit-written shapes
// is RegCustom (never overwritten). The second result lists desired args that
// the current registration lacks.
func Registration(o Options) (RegState, []string) {
	if o.Target == TargetCodex {
		return registrationCodex(o)
	}
	out, err := o.Output("claude", []string{"mcp", "get", "playwright"})
	if err != nil {
		return RegAbsent, nil
	}
	var cmd string
	var args []string
	var rawArgs, scope string
	var haveCmd, haveArgs bool
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Command:"):
			cmd, haveCmd = strings.TrimSpace(strings.TrimPrefix(line, "Command:")), true
		case strings.HasPrefix(line, "Scope:"):
			scope = strings.TrimSpace(strings.TrimPrefix(line, "Scope:"))
		case strings.HasPrefix(line, "Args:"):
			rawArgs = strings.TrimSpace(strings.TrimPrefix(line, "Args:"))
			args, haveArgs = strings.Fields(rawArgs), true
		}
	}
	want := DesiredArgs(o.Home)
	var missing []string
	for _, w := range want {
		found := false
		for _, a := range args {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, w)
		}
	}
	if !haveCmd || !haveArgs || cmd != "npx" || !strings.HasPrefix(scope, "User config") {
		return RegCustom, missing
	}
	switch {
	// Compare the raw remainder: a home dir with spaces would split into extra tokens.
	case rawArgs == strings.Join(want, " "):
		return RegDesired, nil
	case len(args) == 1 && isKitPackage(args[0]):
		return RegLegacyDefault, missing
	case len(args) > 1 && isKitPackage(args[0]) && strings.TrimPrefix(rawArgs, args[0]+" ") == strings.Join(want[1:], " "):
		return RegLegacyDefault, missing // kit flags under an older pin
	}
	return RegCustom, missing
}

// parseCodexMCPGet reads transport.command and transport.args from the JSON
// printed by `codex mcp get playwright --json`.
func parseCodexMCPGet(b []byte) (command string, args []string, err error) {
	var v struct {
		Transport struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return "", nil, err
	}
	return v.Transport.Command, v.Transport.Args, nil
}

// registrationCodex classifies the Codex registration. A failing
// `codex mcp get` (exit != 0, e.g. "No MCP server named") is RegAbsent.
func registrationCodex(o Options) (RegState, []string) {
	out, err := o.Output("codex", []string{"mcp", "get", "playwright", "--json"})
	if err != nil {
		return RegAbsent, nil
	}
	cmd, args, perr := parseCodexMCPGet(out)
	want := DesiredArgs(o.Home)
	var missing []string
	for _, w := range want {
		found := false
		for _, a := range args {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, w)
		}
	}
	if perr != nil || cmd != "npx" {
		return RegCustom, missing
	}
	if strings.Join(args, " ") == strings.Join(want, " ") {
		return RegDesired, nil
	}
	if len(args) >= 1 && isKitPackage(args[0]) && (len(args) == 1 || strings.Join(args[1:], " ") == strings.Join(want[1:], " ")) {
		return RegLegacyDefault, missing
	}
	return RegCustom, missing
}

// Status reports whether the Playwright MCP is registered as the kit wants it
// and whether browser binaries are present — READ-ONLY, no browser is
// launched. mcpOK is true only for RegDesired. Browser presence is a directory
// existence + non-empty check.
func Status(o Options) (mcpOK bool, browsersPresent bool, detail string) {
	state, missing := Registration(o)
	mcpOK = state == RegDesired
	dir := browsersDir(o.Getenv, o.GOOS)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		browsersPresent = true
	}
	br := "absent"
	if browsersPresent {
		br = "present"
	}
	detail = fmt.Sprintf("mcp=%s browsers=%s", state, br)
	if len(missing) > 0 {
		detail += " missing=" + strings.Join(missing, ",")
	}
	return mcpOK, browsersPresent, detail
}

// Bootstrap registers the Playwright MCP and ensures chromium is installed.
// Absent -> add; kit-written legacy default -> remove + add (claude mcp add
// fails on an existing name); custom -> left untouched with a warning.
// Callers decide whether a returned error is fatal — in `zenify up` it is
// non-fatal (a warning).
func Bootstrap(o Options) error {
	note := func(format string, a ...any) {
		if o.Stdout != nil {
			_, _ = fmt.Fprintf(o.Stdout, format+"\n", a...)
		}
	}
	if o.Target == TargetCodex {
		return bootstrapCodex(o, note)
	}
	add := func() error {
		cmd := append([]string{"mcp", "add", "playwright", "-s", "user", "--", "npx"}, DesiredArgs(o.Home)...)
		if err := o.Runner("claude", cmd); err != nil {
			return fmt.Errorf("register playwright MCP: %w", err)
		}
		return nil
	}
	state, missing := Registration(o)
	switch state {
	case RegAbsent:
		note("playwright: registering MCP server (user scope)")
		if err := add(); err != nil {
			return err
		}
	case RegLegacyDefault:
		note("playwright: migrating MCP registration to %s", MCPPackage)
		if err := o.Runner("claude", []string{"mcp", "remove", "playwright", "-s", "user"}); err != nil {
			return fmt.Errorf("remove legacy playwright MCP: %w", err)
		}
		if err := add(); err != nil {
			return err
		}
	case RegCustom:
		note("playwright: custom MCP registration kept; missing args: %s", strings.Join(missing, " "))
	default:
		note("playwright: MCP server already registered")
	}
	if rev := chromiumRevision[MCPVersion]; rev != "" && chromiumInstalled(browsersDir(o.Getenv, o.GOOS), rev) {
		// Skip npx: playwright prints its "without first installing your
		// project's dependencies" box on every npx run, installed or not.
		note("playwright: chromium %s already installed for %s", rev, MCPPackage)
		return nil
	}
	note("playwright: ensuring chromium for %s (downloads a few hundred MB when its revision is missing)", MCPPackage)
	if err := o.Runner("npx", []string{"--yes", "--package", MCPPackage, "playwright", "install", "chromium"}); err != nil {
		return fmt.Errorf("install chromium: %w", err)
	}
	return nil
}

// chromiumInstalled reports whether both browsers `playwright install
// chromium` lays down for rev finished installing under dir.
func chromiumInstalled(dir, rev string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}
	for _, name := range []string{"chromium-" + rev, "chromium_headless_shell-" + rev} {
		if _, err := os.Stat(filepath.Join(dir, name, "INSTALLATION_COMPLETE")); err != nil {
			return false
		}
	}
	return true
}

// bootstrapCodex registers the Playwright MCP with Codex (no scope flag, no
// browser install: chromium is already provisioned by the claude target).
func bootstrapCodex(o Options, note func(string, ...any)) error {
	add := func() error {
		cmd := append([]string{"mcp", "add", "playwright", "--", "npx"}, DesiredArgs(o.Home)...)
		if err := o.Runner("codex", cmd); err != nil {
			return fmt.Errorf("register playwright MCP (codex): %w", err)
		}
		return nil
	}
	state, missing := Registration(o)
	switch state {
	case RegAbsent:
		note("playwright: registering MCP server (codex)")
		return add()
	case RegLegacyDefault:
		note("playwright: migrating codex MCP registration to %s", MCPPackage)
		if err := o.Runner("codex", []string{"mcp", "remove", "playwright"}); err != nil {
			return fmt.Errorf("remove legacy playwright MCP (codex): %w", err)
		}
		return add()
	case RegCustom:
		note("playwright: custom codex MCP registration kept; missing args: %s", strings.Join(missing, " "))
	default:
		note("playwright: codex MCP server already registered")
	}
	return nil
}
