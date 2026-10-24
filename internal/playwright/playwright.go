// Package playwright installs and inspects the pinned @playwright/cli and its
// chromium for zenify, and removes the kit-owned Playwright MCP registration it
// replaces. It shells out to `npm`, `node`, `claude` and `codex` through an
// injected Runner so every path is unit-testable without mutating the machine;
// Status is strictly read-only and never launches a browser.
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

// CLIVersion pins @playwright/cli. Its bundled playwright-core names the one
// chromium revision it can launch (0.1.22 → 1247), so the browser is installed
// through the pinned CLI itself and a new download reaches users only when this
// constant is bumped.
const CLIVersion = "0.1.22"

// CLIPackage is the npm package spec installed under CLIDir.
const CLIPackage = "@playwright/cli@" + CLIVersion

// chromiumRevision maps CLIVersion to the chromium revision its bundled
// playwright-core pins. Bump it with CLIVersion; a version with no entry
// installs on every run.
var chromiumRevision = map[string]string{"0.1.22": "1247"}

// MCPPackage is the @playwright/mcp spec the kit used to register; it is kept
// to recognise (and remove) that registration.
const MCPPackage = "@playwright/mcp@0.0.83"

// CLIDir is where the CLI package is installed.
func CLIDir(home string) string {
	return filepath.Join(home, ".zenify", "playwright", "cli", "pkg")
}

// CLIScript is the CLI entry point run with `node`.
func CLIScript(home string) string {
	return filepath.Join(CLIDir(home), "node_modules", "@playwright", "cli", "playwright-cli.js")
}

// SessionsDir holds per-session browser state.
func SessionsDir(home string) string {
	return filepath.Join(home, ".zenify", "playwright", "sessions")
}

// CLIInstalled reports whether the pinned CLI version is installed.
func CLIInstalled(home string) bool {
	b, err := os.ReadFile(filepath.Join(CLIDir(home), "node_modules", "@playwright", "cli", "package.json")) //nolint:gosec // G304 -- path derived from home
	if err != nil {
		return false
	}
	var v struct {
		Version string `json:"version"`
	}
	return json.Unmarshal(b, &v) == nil && v.Version == CLIVersion
}

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

// Status reports whether the pinned CLI and its chromium are installed —
// READ-ONLY, nothing is run or launched.
func Status(o Options) (cliOK bool, browsersOK bool, detail string) {
	cliOK = CLIInstalled(o.Home)
	rev := chromiumRevision[CLIVersion]
	browsersOK = rev != "" && chromiumInstalled(browsersDir(o.Getenv, o.GOOS), rev)
	state := func(ok bool) string {
		if ok {
			return "ok"
		}
		return "missing"
	}
	detail = fmt.Sprintf("cli=%s browsers=%s", state(cliOK), state(browsersOK))
	if !cliOK || !browsersOK {
		detail += " (run `zenify up`)"
	}
	return cliOK, browsersOK, detail
}

// Install installs the pinned CLI under CLIDir and its chromium. Each step is
// skipped when already satisfied; callers decide whether an error is fatal.
func Install(o Options) error {
	note := func(format string, a ...any) {
		if o.Stdout != nil {
			_, _ = fmt.Fprintf(o.Stdout, format+"\n", a...)
		}
	}
	if !CLIInstalled(o.Home) {
		note("playwright: installing %s", CLIPackage)
		if err := os.MkdirAll(CLIDir(o.Home), 0o755); err != nil { //nolint:gosec // G301 -- user cache dir
			return fmt.Errorf("create %s: %w", CLIDir(o.Home), err)
		}
		if err := o.Runner("npm", []string{"install", "--prefix", CLIDir(o.Home), "--no-audit", "--no-fund", CLIPackage}); err != nil {
			return fmt.Errorf("install %s: %w", CLIPackage, err)
		}
	} else {
		note("playwright: %s already installed", CLIPackage)
	}
	rev := chromiumRevision[CLIVersion]
	if rev != "" && chromiumInstalled(browsersDir(o.Getenv, o.GOOS), rev) {
		note("playwright: chromium %s already installed for %s", rev, CLIPackage)
		return nil
	}
	note("playwright: ensuring chromium for %s (downloads a few hundred MB when its revision is missing)", CLIPackage)
	if err := o.Runner("node", []string{CLIScript(o.Home), "install-browser", "chromium"}); err != nil {
		return fmt.Errorf("install chromium: %w", err)
	}
	return nil
}

// Deregister removes the kit-owned Playwright MCP registration (RegDesired,
// RegLegacyDefault) for o.Target and returns the state found before removal.
// A custom registration is left untouched.
func Deregister(o Options) (RegState, error) {
	state, _ := Registration(o)
	if state != RegDesired && state != RegLegacyDefault {
		return state, nil
	}
	bin, args := "claude", []string{"mcp", "remove", "playwright", "-s", "user"}
	if o.Target == TargetCodex {
		bin, args = "codex", []string{"mcp", "remove", "playwright"}
	}
	if err := o.Runner(bin, args); err != nil {
		return state, fmt.Errorf("remove playwright MCP (%s): %w", bin, err)
	}
	return state, nil
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
