package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// pluginRunner builds the claude runner ensureWorkspace uses; tests swap it.
var pluginRunner = runClaudeIn

// runClaudeIn runs one command with claudeMCPTimeout, returning its combined
// output; WaitDelay stops a child that keeps the pipes open. dir is the child's
// working directory ("" = current), so `--scope local` resolves to that workspace.
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
					if exists, current := pluginStampState(workspace); exists && current {
						return nil
					}
					return updatePluginLocal(workspace, run)
				}
			}
		}
	}
	if out, err := run("claude", "plugin", "marketplace", "add", plugin.MarketplaceDir(zh), "--scope", "local"); err != nil &&
		!alreadyAdded(string(out)+err.Error()) {
		return fmt.Errorf("claude plugin marketplace add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := run("claude", "plugin", "install", plugin.PluginID, "--scope", "local"); err != nil {
		return fmt.Errorf("claude plugin install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return writePluginStamp(workspace)
}

func pluginStampPath(workspace string) string {
	return filepath.Join(workspace, ".zenify", "plugin-version")
}

// writePluginStamp records the kit version this workspace's plugin was last
// brought to (atomic: temp file + rename).
func writePluginStamp(workspace string) error {
	p := pluginStampPath(workspace)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // G301 -- same dir mode as the workspace's other kit dirs
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-plugin-version-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best effort; gone after rename
	if _, err := tmp.WriteString(version.Current()); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// pluginStampState reports whether <workspace>/.zenify/plugin-version exists
// and whether it names the running binary.
func pluginStampState(workspace string) (exists, current bool) {
	b, err := os.ReadFile(pluginStampPath(workspace)) //nolint:gosec // G304 -- internal path
	if err != nil {
		return false, false
	}
	return true, strings.TrimSpace(string(b)) == version.Current()
}

// updatePluginLocal updates the marketplace and the local znf plugin, then
// writes the stamp; the stamp is written only when both commands succeed, so a
// failure is retried next time.
func updatePluginLocal(workspace string, run func(string, ...string) ([]byte, error)) error {
	var failed []string
	for _, args := range [][]string{
		{"plugin", "marketplace", "update", plugin.MarketplaceName},
		{"plugin", "update", plugin.PluginID, "-s", "local"},
	} {
		if out, err := run("claude", args...); err != nil {
			failed = append(failed, fmt.Sprintf("claude %s: %v: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(string(out))))
		}
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	if err := writePluginStamp(workspace); err != nil {
		return fmt.Errorf("plugin stamp: %w", err)
	}
	return nil
}

// resyncPluginLocal re-materializes the marketplace and updates the local
// plugin when the workspace's stamp (<workspace>/.zenify/plugin-version) is
// stale. No stamp means the plugin was never installed locally here (`zenify
// up` installs it): it only re-materializes the marketplace, which the Codex
// skills link and hooks point into, and runs no claude command. Fail-open:
// a failure is one stderr line, and the returned error lets ensureWorkspace
// record the step in LocalErrs.
func resyncPluginLocal(workspace, zh string, run func(string, ...string) ([]byte, error), stderr io.Writer) error {
	exists, current := pluginStampState(workspace)
	if current {
		return nil
	}
	if _, err := plugin.SyncMarketplace(zh); err != nil {
		fmt.Fprintln(stderr, "znf ensure: plugin sync:", err)
		return fmt.Errorf("plugin sync: %w", err)
	}
	if !exists {
		return nil
	}
	if err := updatePluginLocal(workspace, run); err != nil {
		fmt.Fprintln(stderr, "znf ensure: plugin:", err)
		return err
	}
	return nil
}

func alreadyAdded(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "already exists") || strings.Contains(s, "already added")
}
