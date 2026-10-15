// Package mcpreg registers the kit's broker-backed HTTP MCP servers (Grafana,
// PostHog) with Claude Code. Claude connects to the broker as `type: http` and
// calls `zenify broker headers` for a fresh bearer header on each connection,
// so no credential is ever written to the registration.
package mcpreg

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
)

// HeadersHelper is the command Claude Code runs to obtain request headers.
const HeadersHelper = "zenify broker headers"

// notFound is what `claude mcp get` prints for an unknown name.
const notFound = "No MCP server found"

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
// entry that names HeadersHelper is the kit's own with a stale URL: it is
// removed and re-added (RegLegacyDefault). Anything else under that name is
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
		case strings.HasPrefix(scope, "User config") && strings.Contains(string(out), HeadersHelper):
			if _, err := o.Run("claude", "mcp", "remove", o.Spec.Name, "-s", "user"); err != nil {
				return playwright.RegLegacyDefault, fmt.Errorf("remove stale %s MCP: %w", o.Spec.Name, err)
			}
			state = playwright.RegLegacyDefault
		default:
			return playwright.RegCustom, nil
		}
	case !strings.Contains(string(out), notFound):
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
