package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/mcpreg"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
)

// userKitMCPs are the MCP servers older kits registered at user scope.
var userKitMCPs = []string{"zenify-grafana", "zenify-posthog", "zenify-plane"}

// userKeep names user-level Codex installs `up` must keep because the
// workspace got no replacement: CodexHooks (with the agents) when codex is not
// on PATH, SkillsLink when <ws>/.agents/skills/znf is not the kit's link. The
// zero value (down) removes everything.
type userKeep struct{ CodexHooks, SkillsLink bool }

// skillsEdited reports whether the kit skills tree at dir holds a file its
// .manifest.json does not record, or one changed since the kit wrote it.
func skillsEdited(dir string) (bool, error) {
	m, err := managed.Load(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		return false, err
	}
	edited := false
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == filepath.Join(dir, ".manifest.json") {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // G304 -- walking the kit's own skills dir
		if err != nil {
			return err
		}
		if m.DecideRefresh(p, b) != managed.DecisionUpdate {
			edited = true
			return filepath.SkipAll
		}
		return nil
	})
	return edited, err
}

// migrateUserInstalls removes what older kits installed at user level, once
// the workspace install succeeded (FR-5). Only kit-owned entries go: personal
// skills, hooks, env keys and rules stay, and the .zenify/.env deny rule is
// never touched. mcps lists the user-scope MCP servers to remove: `up` passes
// only those it registered at local scope, so a skipped registration keeps the
// user one. removed names each item removed (or, with dryRun, each item
// that would be; the MCP servers are then listed without running claude). The
// first error stops the run; removed still lists what went before it.
func migrateUserInstalls(home, codexHome string, mcps []string, keep userKeep, run func(string, ...string) ([]byte, error), dryRun bool) (removed []string, err error) {
	settings := apply.UserSettings(home)

	skills := filepath.Join(home, ".claude", "skills", "znf")
	if _, serr := os.Stat(filepath.Join(skills, ".manifest.json")); serr == nil {
		edited, err := skillsEdited(skills)
		if err != nil {
			return removed, err
		}
		switch {
		case edited:
			backup := filepath.Join(zenifyHome(os.Getenv, func() (string, error) { return home, nil }),
				"backup", "skills-znf-"+time.Now().Format("20060102-150405"))
			if !dryRun {
				if err := os.MkdirAll(filepath.Dir(backup), 0o750); err != nil {
					return removed, err
				}
				if err := os.Rename(skills, backup); err != nil {
					return removed, fmt.Errorf("back up %s: %w", skills, err)
				}
			}
			removed = append(removed, skills+" (edited, moved to "+backup+")")
		case !dryRun:
			if err := os.RemoveAll(skills); err != nil {
				return removed, fmt.Errorf("remove %s: %w", skills, err)
			}
			fallthrough
		default:
			removed = append(removed, skills)
		}
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
		// Only the kit's own registration goes; a server the user configured
		// under the same name stays, even when a local one now shadows it.
		if owned, err := mcpreg.KitOwned(filepath.Join(home, ".claude.json"), "", name); err != nil {
			return removed, err
		} else if !owned {
			continue
		}
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
	if keep.SkillsLink {
		// the workspace has no link of its own: Codex keeps the user one
	} else if ok, err := apply.RemoveCodexSkillsLink(link, dryRun); err != nil {
		return removed, err
	} else if ok {
		removed = append(removed, link)
	}
	if !keep.CodexHooks {
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
