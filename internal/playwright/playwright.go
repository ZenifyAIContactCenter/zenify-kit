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
)

type Options struct {
	Runner func(name string, args []string) error
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

// Status reports whether the Playwright MCP is registered and whether browser
// binaries are present — READ-ONLY, no browser is launched. Registration is
// probed with `claude mcp get playwright` (exit 0 = present). Browser presence
// is a directory existence + non-empty check.
func Status(o Options) (mcpRegistered bool, browsersPresent bool, detail string) {
	mcpRegistered = o.Runner("claude", []string{"mcp", "get", "playwright"}) == nil
	dir := browsersDir(o.Getenv, o.GOOS)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		browsersPresent = true
	}
	reg, br := "absent", "absent"
	if mcpRegistered {
		reg = "registered"
	}
	if browsersPresent {
		br = "present"
	}
	return mcpRegistered, browsersPresent, fmt.Sprintf("mcp=%s browsers=%s", reg, br)
}

// Bootstrap registers the Playwright MCP (idempotently) and ensures chromium is
// installed. Registration uses the grounded reference config: user scope, stdio
// `npx @playwright/mcp@latest`. Callers decide whether a returned error is fatal
// — in `zenify up` it is non-fatal (a warning).
func Bootstrap(o Options) error {
	note := func(format string, a ...any) {
		if o.Stdout != nil {
			_, _ = fmt.Fprintf(o.Stdout, format+"\n", a...)
		}
	}
	if o.Runner("claude", []string{"mcp", "get", "playwright"}) != nil {
		note("playwright: registering MCP server (user scope)")
		if err := o.Runner("claude", []string{"mcp", "add", "playwright", "-s", "user", "--", "npx", "@playwright/mcp@latest"}); err != nil {
			return fmt.Errorf("register playwright MCP: %w", err)
		}
	} else {
		note("playwright: MCP server already registered")
	}
	note("playwright: ensuring chromium is installed")
	if err := o.Runner("npx", []string{"playwright", "install", "chromium"}); err != nil {
		return fmt.Errorf("install chromium: %w", err)
	}
	return nil
}
