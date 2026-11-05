package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// pluginRunner builds the claude runner ensureWorkspace uses; tests swap it.
var pluginRunner = runClaudeIn

// runClaudeIn is runClaudeBounded with the child's working directory set to
// dir, so `--scope local` resolves to that workspace.
func runClaudeIn(dir string) func(string, ...string) ([]byte, error) {
	return func(n string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), claudeMCPTimeout)
		defer cancel()
		c := exec.CommandContext(ctx, n, args...) //nolint:gosec // G204 -- fixed trusted binary, internally-computed args
		c.Dir = dir
		c.WaitDelay = 2 * time.Second
		out, err := c.CombinedOutput()
		if err != nil && ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", ctx.Err(), err)
		}
		return out, err
	}
}

func samePath(a, b string) bool {
	norm := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return filepath.Clean(p)
	}
	return norm(a) == norm(b)
}

// installPluginLocal materializes the marketplace under zh and installs the
// znf plugin at local scope for workspace, skipping what is already enabled.
func installPluginLocal(workspace, zh string, run func(string, ...string) ([]byte, error)) error {
	if _, err := plugin.SyncMarketplace(zh); err != nil {
		return fmt.Errorf("plugin marketplace sync: %w", err)
	}
	if out, err := run("claude", "plugin", "list", "--json"); err == nil {
		var list []struct {
			ID          string `json:"id"`
			Scope       string `json:"scope"`
			ProjectPath string `json:"projectPath"`
			Enabled     bool   `json:"enabled"`
		}
		if json.Unmarshal(out, &list) == nil {
			for _, p := range list {
				if p.ID == plugin.PluginID && p.Scope == "local" && p.Enabled && samePath(p.ProjectPath, workspace) {
					return nil
				}
			}
		}
	}
	if out, err := run("claude", "plugin", "marketplace", "add", plugin.MarketplaceDir(zh), "--scope", "local"); err != nil &&
		!strings.Contains(strings.ToLower(string(out)+err.Error()), "already") {
		return fmt.Errorf("claude plugin marketplace add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := run("claude", "plugin", "install", plugin.PluginID, "--scope", "local"); err != nil {
		return fmt.Errorf("claude plugin install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resyncPluginLocal re-materializes the marketplace and updates the local
// plugin when the stamped version differs from the binary. Fail-open.
func resyncPluginLocal(workspace, zh string, run func(string, ...string) ([]byte, error), stderr io.Writer) {
	_ = workspace // the runner already carries the workspace as its cwd
	stamp := ""
	if m, err := managed.Load(plugin.PluginManifest(zh)); err == nil {
		stamp = m.Version
	}
	if stamp == version.Current() {
		return
	}
	if _, err := plugin.SyncMarketplace(zh); err != nil {
		fmt.Fprintln(stderr, "znf ensure: plugin sync:", err)
		return
	}
	for _, args := range [][]string{
		{"plugin", "marketplace", "update", plugin.MarketplaceName},
		{"plugin", "update", plugin.PluginID, "-s", "local"},
	} {
		if out, err := run("claude", args...); err != nil {
			fmt.Fprintf(stderr, "znf ensure: claude %s: %v: %s\n", strings.Join(args[:2], " "), err, strings.TrimSpace(string(out)))
		}
	}
}
