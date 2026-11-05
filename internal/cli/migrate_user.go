package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/mcpreg"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
)

// userKitMCPs are the MCP servers older kits registered at user scope.
var userKitMCPs = []string{"zenify-grafana", "zenify-posthog", "zenify-plane"}

// migrateUserInstalls removes what older kits installed at user level, once
// the workspace install succeeded (FR-5). Only kit-owned entries go: personal
// skills, hooks, env keys and rules stay, and the .zenify/.env deny rule is
// never touched. mcps lists the user-scope MCP servers to remove: `up` passes
// only those it registered at local scope, so a skipped registration keeps the
// user one. removed names each item removed (or, with dryRun, each item
// that would be; the MCP servers are then listed without running claude). The
// first error stops the run; removed still lists what went before it.
func migrateUserInstalls(home, codexHome string, mcps []string, run func(string, ...string) ([]byte, error), dryRun bool) (removed []string, err error) {
	settings := apply.UserSettings(home)

	skills := filepath.Join(home, ".claude", "skills", "znf")
	if _, serr := os.Stat(filepath.Join(skills, ".manifest.json")); serr == nil {
		if !dryRun {
			if err := os.RemoveAll(skills); err != nil {
				return removed, fmt.Errorf("remove %s: %w", skills, err)
			}
		}
		removed = append(removed, skills)
	}

	hooks, err := apply.RemoveGlobalHooks(settings, dryRun)
	if err != nil {
		return removed, err
	}
	if hooks.Removed > 0 {
		removed = append(removed, fmt.Sprintf("%d znf hooks in %s", hooks.Removed, settings))
	}
	if ok, err := removeGuard(settings, dryRun); err != nil {
		return removed, err
	} else if ok {
		removed = append(removed, "git-guard hook in "+settings)
	}

	for _, name := range mcps {
		if dryRun {
			removed = append(removed, "MCP "+name+" (-s user, if registered)")
			continue
		}
		ok, err := mcpreg.RemoveUserScoped(name, run)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, "MCP "+name+" (-s user)")
		}
	}

	link := filepath.Join(home, ".agents", "skills", "znf")
	if ok, err := apply.RemoveCodexSkillsLink(link, dryRun); err != nil {
		return removed, err
	} else if ok {
		removed = append(removed, link)
	}
	ch, err := apply.RemoveCodexHooks(codexHome, dryRun)
	if err != nil {
		return removed, err
	}
	if ch.Removed > 0 {
		removed = append(removed, fmt.Sprintf("%d kit hooks in %s", ch.Removed, filepath.Join(codexHome, "hooks.json")))
	}
	agents, err := plugin.RemoveCodexAgents(codexHome, dryRun)
	if err != nil {
		return removed, err
	}
	for _, n := range agents {
		removed = append(removed, filepath.Join(codexHome, "agents", n+".toml"))
	}

	for _, e := range []struct {
		key string
		fn  func(string, bool) (bool, error)
	}{{apply.SubagentModelEnv, apply.RemoveSubagentModelEnv}, {apply.ReviewerEnv, apply.RemoveReviewerEnv}} {
		ok, err := e.fn(settings, dryRun)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, "env."+e.key+" in "+settings)
		}
	}

	kt := apply.UserKitTarget(home)
	for _, l := range []struct {
		what string
		fn   func(apply.KitTarget, bool) (int, error)
	}{{"permissions.allow", apply.RemoveKitPermissions}, {"sandbox.excludedCommands", apply.RemoveKitSandbox}, {"permissions.ask", apply.RemoveKitAsk}} {
		n, err := l.fn(kt, dryRun)
		if errors.Is(err, fs.ErrNotExist) {
			continue // no record: the kit never added to this list
		}
		if err != nil {
			return removed, err
		}
		if n > 0 {
			removed = append(removed, fmt.Sprintf("%d kit rules in %s of %s", n, l.what, settings))
		}
	}
	return removed, nil
}
