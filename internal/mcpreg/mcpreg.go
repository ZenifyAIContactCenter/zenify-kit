// Package mcpreg registers the kit's broker-backed HTTP MCP servers (Grafana,
// PostHog) with Claude Code. Claude connects to the broker as `type: http` and
// calls `zenify broker headers` for a fresh bearer header on each connection,
// so no credential is ever written to the registration.
package mcpreg

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
)

// HeadersHelper is the command Claude Code runs to obtain request headers.
const HeadersHelper = "zenify broker headers"

// isNotFound reports whether failed `claude mcp get` output says the name is
// unknown (Claude Code 2.1.300 prints `No MCP server named "<name>"` and exits 1).
// Any other failure (a timeout, a crash) is not "absent".
func isNotFound(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "no mcp server")
}

// Spec names one kit-managed MCP server; URL is <broker.url>/mcp/<name>.
type Spec struct{ Name, URL string }

// Options carries the Spec and the injected command runner. Run returns the
// combined stdout+stderr of the command.
type Options struct {
	Spec Spec
	Run  func(name string, args ...string) ([]byte, error)
}

// Bootstrap registers the server with Claude at local scope (the caller runs
// Run with the workspace root as cwd) when absent. A local-scope entry whose
// `URL:` line equals Spec.URL is already right (RegDesired). A local-scope http
// entry whose URL ends in the same /mcp/<name> path is the kit's own with a
// stale broker host: it is removed (-s local) and re-added (RegLegacyDefault). A
// user-scope entry with the same URL or path is a pre-workspace-scope install:
// the local entry is added and the user one is left for RemoveUserScoped
// (RegLegacyDefault). `claude mcp get` does not print headersHelper (verified on
// 2.1.300), so the path is the only ownership mark. zenify-* names are
// kit-reserved: a user's own server under that name and path is overwritten
// too. Anything else under that name is the user's own and is left alone
// (RegCustom). A `get` failure other than "not found" is an error and nothing is
// added. Claude only; Codex is a later phase.
func Bootstrap(o Options) (playwright.RegState, error) {
	out, err := o.Run("claude", "mcp", "get", o.Spec.Name)
	state := playwright.RegAbsent
	switch {
	case err == nil:
		url, scope := parseGet(string(out))
		local, user := strings.HasPrefix(scope, "Local config"), strings.HasPrefix(scope, "User config")
		switch {
		case url == o.Spec.URL && local:
			return playwright.RegDesired, nil
		case url == o.Spec.URL && user:
			state = playwright.RegLegacyDefault
		case local && strings.Contains(string(out), "Type: http") && samePath(url, o.Spec.URL):
			if _, err := o.Run("claude", "mcp", "remove", o.Spec.Name, "-s", "local"); err != nil {
				return playwright.RegLegacyDefault, fmt.Errorf("remove stale %s MCP: %w", o.Spec.Name, err)
			}
			state = playwright.RegLegacyDefault
		case user && strings.Contains(string(out), "Type: http") && samePath(url, o.Spec.URL):
			state = playwright.RegLegacyDefault
		default:
			return playwright.RegCustom, nil
		}
	case !isNotFound(string(out)):
		return playwright.RegAbsent, fmt.Errorf("claude mcp get %s: %w: %s", o.Spec.Name, err, strings.TrimSpace(string(out)))
	}
	cfg, err := json.Marshal(map[string]string{"type": "http", "url": o.Spec.URL, "headersHelper": HeadersHelper})
	if err != nil {
		return state, err
	}
	if _, err := o.Run("claude", "mcp", "add-json", o.Spec.Name, string(cfg), "-s", "local"); err != nil {
		return state, fmt.Errorf("register %s MCP: %w", o.Spec.Name, err)
	}
	return state, nil
}

// RemoveUserScoped removes the user-scope registration of name (the one older
// kits wrote). It reports false when there was none ("not found").
func RemoveUserScoped(name string, run func(string, ...string) ([]byte, error)) (bool, error) {
	out, err := run("claude", "mcp", "remove", name, "-s", "user")
	if err == nil {
		return true, nil
	}
	if o := strings.ToLower(string(out)); isNotFound(o) || strings.Contains(o, "not found") || strings.Contains(o, "mcp server found with name") {
		return false, nil
	}
	return false, fmt.Errorf("remove user-scope %s MCP: %w: %s", name, err, strings.TrimSpace(string(out)))
}

// samePath reports whether got is an http(s) URL with the same path as want.
func samePath(got, want string) bool {
	g, err := neturl.Parse(got)
	if err != nil || g.Host == "" || (g.Scheme != "https" && g.Scheme != "http") {
		return false
	}
	w, err := neturl.Parse(want)
	return err == nil && g.Path == w.Path && g.RawQuery == ""
}

// parseGet reads the `URL:` and `Scope:` lines of `claude mcp get` output.
func parseGet(out string) (url, scope string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "URL:"):
			url = strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
		case strings.HasPrefix(line, "Scope:"):
			scope = strings.TrimSpace(strings.TrimPrefix(line, "Scope:"))
		}
	}
	return url, scope
}

// StdioSpec names one kit-managed stdio MCP server (a local command).
type StdioSpec struct {
	Name, Command string
	Args          []string
}

// BootstrapStdio registers a stdio server with Claude at local scope when
// absent. Ownership is decided only by the `Type:`, `Command:` and `Args:` lines
// of `claude mcp get`: the command also health-checks the server and may print
// `Status:`/`Issue:` lines while still exiting 0, so neither those lines nor a 0
// exit mean anything. A local entry with the same Command and Args is
// RegDesired; a local stdio entry with the same Command but other Args is the
// kit's own stale one and is replaced (RegLegacyDefault); a user-scope stdio
// entry with the same Command is a pre-workspace-scope install: the local entry
// is added and the user one is left for RemoveUserScoped (RegLegacyDefault);
// anything else under the name is the user's (RegCustom). The registration never
// carries an `env` key: the server reads its credential itself.
func BootstrapStdio(spec StdioSpec, run func(string, ...string) ([]byte, error)) (playwright.RegState, error) {
	out, err := run("claude", "mcp", "get", spec.Name)
	g := parseStdioGet(string(out))
	local, user := strings.HasPrefix(g.scope, "Local config"), strings.HasPrefix(g.scope, "User config")
	same := g.typ == "stdio" && g.command == spec.Command
	if same && local && g.args == strings.Join(spec.Args, " ") {
		return playwright.RegDesired, nil // even when get then failed (its health check timed out)
	}
	state := playwright.RegAbsent
	switch {
	case err == nil:
		switch {
		case same && local:
			if _, err := run("claude", "mcp", "remove", spec.Name, "-s", "local"); err != nil {
				return playwright.RegLegacyDefault, fmt.Errorf("remove stale %s MCP: %w", spec.Name, err)
			}
			state = playwright.RegLegacyDefault
		case same && user:
			state = playwright.RegLegacyDefault
		default:
			return playwright.RegCustom, nil
		}
	case !isNotFound(string(out)):
		return playwright.RegAbsent, fmt.Errorf("claude mcp get %s: %w: %s", spec.Name, err, strings.TrimSpace(string(out)))
	}
	args := spec.Args
	if args == nil {
		args = []string{}
	}
	cfg, err := json.Marshal(struct {
		Type    string   `json:"type"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{"stdio", spec.Command, args})
	if err != nil {
		return state, err
	}
	if _, err := run("claude", "mcp", "add-json", spec.Name, string(cfg), "-s", "local"); err != nil {
		return state, fmt.Errorf("register %s MCP: %w", spec.Name, err)
	}
	return state, nil
}

type stdioGet struct{ scope, typ, command, args string }

// parseStdioGet reads the `Scope:`, `Type:`, `Command:` and `Args:` lines;
// every other line (Status, Issue, Environment, ...) is ignored.
func parseStdioGet(out string) stdioGet {
	var g stdioGet
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Scope:"):
			g.scope = strings.TrimSpace(strings.TrimPrefix(line, "Scope:"))
		case strings.HasPrefix(line, "Type:"):
			g.typ = strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
		case strings.HasPrefix(line, "Command:"):
			g.command = strings.TrimSpace(strings.TrimPrefix(line, "Command:"))
		case strings.HasPrefix(line, "Args:"):
			g.args = strings.TrimSpace(strings.TrimPrefix(line, "Args:"))
		}
	}
	return g
}
