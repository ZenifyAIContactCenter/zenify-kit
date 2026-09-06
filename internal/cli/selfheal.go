// internal/cli/selfheal.go
package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// selfHeal re-materializes znf skills + re-wires hooks when the installed
// stamp is behind the running binary. Fully fail-open: returns nil always,
// swallowing errors so SessionStart is never blocked. home param kept for
// EnsureGlobalHooks (which is home-relative and test-fakeable).
func selfHeal(home string) error {
	dest, err := plugin.DefaultDest()
	if err != nil {
		return nil // fail-open
	}
	manifestPath, err := plugin.DefaultManifest()
	if err != nil {
		return nil
	}

	stamp := ""
	if m, err := managed.Load(manifestPath); err == nil {
		stamp = m.Version
	}
	if stamp == version.Current() {
		return nil // up to date, fast no-op
	}

	// Stale (or unknown): re-sync + re-wire, best-effort.
	if _, err := plugin.Sync(dest, manifestPath); err != nil {
		return nil // fail-open
	}
	if _, err := apply.EnsureGlobalHooks(home, false); err != nil {
		return nil // fail-open (malformed settings etc.)
	}
	return nil
}
