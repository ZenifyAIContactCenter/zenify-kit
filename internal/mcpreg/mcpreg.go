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
// "not found" is also accepted; any other failure (a timeout, a crash) is not
// "absent".
func isNotFound(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "no mcp server") || strings.Contains(o, "not found")
}

// Spec names one kit-managed MCP server; URL is <broker.url>/mcp/<name>.
type Spec struct{ Name, URL string }

// Options carries the Spec and the injected command runner. Run returns the
// combined stdout+stderr of the command.
type Options struct {
	Spec Spec
	Run  func(name string, args ...string) ([]byte, error)
}

// Bootstrap registers the server with Claude when absent. A user-scope entry
// whose `URL:` line equals Spec.URL is already right (RegDesired). A user-scope
// http entry whose URL ends in the same /mcp/<name> path is the kit's own with a
// stale broker host: it is removed and re-added (RegLegacyDefault). `claude mcp
// get` does not print headersHelper (verified on 2.1.300), so the path is the
// only ownership mark; zenify-* names are kit-reserved. Anything else under that name is
// the user's own and is left alone (RegCustom). A `get` failure other than
// "not found" is an error and nothing is added. Claude only; Codex is a later
// phase.
func Bootstrap(o Options) (playwright.RegState, error) {
	out, err := o.Run("claude", "mcp", "get", o.Spec.Name)
	state := playwright.RegAbsent
	switch {
	case err == nil:
		url, scope := parseGet(string(out))
		switch {
		case url == o.Spec.URL:
			return playwright.RegDesired, nil
		case strings.HasPrefix(scope, "User config") && strings.Contains(string(out), "Type: http") && samePath(url, o.Spec.URL):
			if _, err := o.Run("claude", "mcp", "remove", o.Spec.Name, "-s", "user"); err != nil {
				return playwright.RegLegacyDefault, fmt.Errorf("remove stale %s MCP: %w", o.Spec.Name, err)
			}
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
	if _, err := o.Run("claude", "mcp", "add-json", o.Spec.Name, string(cfg), "-s", "user"); err != nil {
		return state, fmt.Errorf("register %s MCP: %w", o.Spec.Name, err)
	}
	return state, nil
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
