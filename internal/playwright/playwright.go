// Package playwright bootstraps and inspects the Playwright MCP server and its
// browser binaries for zenify (FR-014/FR-066). It shells out to `claude` and
// `npx` through an injected Runner so every path is unit-testable without
// mutating the machine; Status is strictly read-only and never launches a
// browser.
package playwright

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
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

// DesiredArgs are the npx args the kit registers the Playwright MCP with.
// --caps=storage enables browser_set_storage_state; the output dir keeps
// artifacts under the kit's own home dir.
func DesiredArgs(home string) []string {
	return []string{"@playwright/mcp@latest", "--caps=storage", "--output-dir", filepath.Join(home, ".zenify", "playwright")}
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
	out, err := o.Output("claude", []string{"mcp", "get", "playwright"})
	if err != nil {
		return RegAbsent, nil
	}
	var cmd string
	var args []string
	var haveCmd, haveArgs bool
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Command:"):
			cmd, haveCmd = strings.TrimSpace(strings.TrimPrefix(line, "Command:")), true
		case strings.HasPrefix(line, "Args:"):
			args, haveArgs = strings.Fields(strings.TrimPrefix(line, "Args:")), true
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
	if !haveCmd || !haveArgs || cmd != "npx" {
		return RegCustom, missing
	}
	switch {
	case strings.Join(args, " ") == strings.Join(want, " "):
		return RegDesired, nil
	case len(args) == 1 && args[0] == want[0]:
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
		note("playwright: migrating MCP registration to storage caps")
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
	note("playwright: ensuring chromium is installed")
	if err := o.Runner("npx", []string{"playwright", "install", "chromium"}); err != nil {
		return fmt.Errorf("install chromium: %w", err)
	}
	return nil
}
