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

// Spec names one kit-managed MCP server; URL is <broker.url>/mcp/<name>.
type Spec struct{ Name, URL string }

// Options carries the Spec and the injected command runner.
type Options struct {
	Spec Spec
	Run  func(name string, args ...string) ([]byte, error)
}

// Bootstrap registers the server with Claude when absent. A server whose
// `claude mcp get` output contains the URL is already right (RegDesired); one
// that exists without it is the user's own and is left alone (RegCustom).
// Claude only; Codex is a later phase.
func Bootstrap(o Options) (playwright.RegState, error) {
	out, err := o.Run("claude", "mcp", "get", o.Spec.Name)
	if err == nil {
		if strings.Contains(string(out), o.Spec.URL) {
			return playwright.RegDesired, nil
		}
		return playwright.RegCustom, nil
	}
	cfg, err := json.Marshal(map[string]string{"type": "http", "url": o.Spec.URL, "headersHelper": HeadersHelper})
	if err != nil {
		return playwright.RegAbsent, err
	}
	if _, err := o.Run("claude", "mcp", "add-json", o.Spec.Name, string(cfg), "-s", "user"); err != nil {
		return playwright.RegAbsent, fmt.Errorf("register %s MCP: %w", o.Spec.Name, err)
	}
	return playwright.RegAbsent, nil
}
