package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// ensureReport carries what the Codex steps of ensureWorkspace found, for the
// `zenify up` summary block (FR-7.2).
type ensureReport struct {
	CodexInstalled bool
	CodexLinkPath  string
	CodexHooks     apply.HookChanges
	CodexAgents    int
	CodexMCP       string // playwright registration state; filled by `up`
}

// ensureWorkspace brings a workspace to the state the kit expects (W0 FR-01):
//
//  1. re-materialize the znf plugin when the installed stamp is behind the
//     running binary (the former selfHeal);
//  2. wire/migrate znf hooks in <home>/.claude/settings.json;
//  3. pin the workspace default model in <workspace>/.claude/settings.json;
//  4. distribute the knowledge store's .config into the workspace
//     (`zenify config --apply`).
//
// Every step is fail-open: an error is one "znf ensure:" line on stderr and
// the next step still runs. stdout only gets a line when something changed,
// so a SessionStart on an up-to-date workspace prints nothing.
func ensureWorkspace(workspace, home string, stdout, stderr io.Writer) (rep ensureReport) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(stderr, "znf ensure: recovered:", r)
		}
	}()

	resyncPluginIfStale(home, stderr)

	// Codex discovers skills under ~/.agents/skills, so this runs whether or
	// not codex is installed (FR-2.1).
	rep.CodexLinkPath = filepath.Join(home, ".agents", "skills", "znf")
	if st, err := apply.EnsureCodexSkillsLink(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: codex-skills:", err)
	} else if st == apply.LinkRealDir {
		fmt.Fprintf(stderr, "znf ensure: codex-skills: %s là thư mục thật, bỏ qua\n", filepath.Join(home, ".agents", "skills", "znf")) //znf:allow-lang
	}

	if ch, err := apply.EnsureGlobalHooks(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: hooks:", err)
	} else if n := ch.Added + ch.Updated; n > 0 {
		fmt.Fprintf(stdout, "wired %d znf hooks\n", n)
	}

	// git-guard is a separate hook from the znf set above (it predates it and
	// `guard install` owns it), so it is wired here too — otherwise `up`
	// never installs it and deploy branches are unguarded on every machine.
	if changed, err := installGuard(home); err != nil {
		fmt.Fprintln(stderr, "znf ensure: git-guard:", err)
	} else if changed {
		fmt.Fprintln(stdout, "wired git-guard hook (blocks commit/push on deploy branches)")
	}

	if codexOnPath() {
		rep.CodexInstalled = true
		if ch, err := apply.EnsureCodexHooks(codexHomeDir(home, os.Getenv), false); err != nil {
			fmt.Fprintln(stderr, "znf ensure: codex-hooks:", err)
		} else if rep.CodexHooks = ch; ch.Added+ch.Updated > 0 {
			fmt.Fprintln(stdout, "Codex hook chưa chạy cho tới khi bạn trust một lần: mở codex, gõ /hooks, chọn trust.") //znf:allow-lang
		}
		if ar, err := plugin.SyncCodexAgents(codexHomeDir(home, os.Getenv), false); err != nil {
			fmt.Fprintln(stderr, "znf ensure: codex-agents:", err)
		} else {
			rep.CodexAgents = len(ar.Written) + len(ar.Unchanged)
			for _, n := range ar.Skipped {
				fmt.Fprintf(stderr, "znf ensure: codex-agents: %s.toml không phải của kit, bỏ qua\n", n) //znf:allow-lang
			}
			for _, n := range ar.Removed {
				fmt.Fprintf(stdout, "removed retired Codex agent %s.toml\n", n)
			}
		}
	}

	if changed, err := apply.EnsureWorkspaceModel(workspace, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: model:", err)
	} else if changed {
		fmt.Fprintf(stdout, "pinned workspace model → %s\n", apply.DefaultModel)
	}

	if changed, err := apply.EnsureSubagentModelEnv(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: subagent model:", err)
	} else if changed {
		fmt.Fprintf(stdout, "set %s=%s in ~/.claude/settings.json (subagents that omit model now run there)\n",
			apply.SubagentModelEnv, apply.SubagentModelDefault)
	}

	if changed, err := apply.RemoveStrongModelEnv(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: strong model:", err)
	} else if changed {
		fmt.Fprintf(stdout, "removed %s from ~/.claude/settings.json (select-route is gone)\n", apply.StrongModelEnv)
	}

	if changed, err := apply.EnsureReviewerEnv(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: reviewer:", err)
	} else if changed {
		fmt.Fprintf(stdout, "set %s=%s in ~/.claude/settings.json (zenify reviewer picks the review engine from it)\n",
			apply.ReviewerEnv, apply.ReviewerDefault)
	}

	if n, err := apply.EnsureKitPermissions(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: permissions:", err)
	} else if n > 0 {
		fmt.Fprintf(stdout, "allowed %d kit tool rules in ~/.claude/settings.json\n", n)
	}
	if n, err := apply.EnsureKitSandbox(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: sandbox:", err)
	} else if n > 0 {
		fmt.Fprintf(stdout, "excluded %d kit commands from the Bash sandbox in ~/.claude/settings.json\n", n)
	}
	if n, err := apply.EnsureKitAsk(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: ask:", err)
	} else if n > 0 {
		fmt.Fprintf(stdout, "added %d kit ask rules (prd data writes prompt) in ~/.claude/settings.json\n", n)
	}

	// runConfig prints its whole plan to stdout; ensure only wants the count
	// plus the CREATE/UPDATE lines (so the overwrite is visible in the
	// session's additional context — see final-review.md Important #2), and
	// its stderr fail-open note (the manifest-unreadable line) needs to
	// surface as a "znf ensure:" line rather than being lost.
	var cfgOut, cfgErr bytes.Buffer
	n, err := runConfig(workspace, "", true, &cfgOut, &cfgErr)
	if cfgErr.Len() > 0 {
		for _, line := range strings.Split(strings.TrimRight(cfgErr.String(), "\n"), "\n") {
			if line == "" {
				continue
			}
			fmt.Fprintln(stderr, "znf ensure: "+line)
		}
	}
	// distribute.Apply reports per-pair write failures as stdout notes
	// ("could not read source …" / "could not write …"), not as an error and
	// not in the written count — forward them to stderr regardless of n so a
	// run in which every write failed still leaves a signal.
	for _, line := range strings.Split(cfgOut.String(), "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "could not ") {
			fmt.Fprintln(stderr, "znf ensure: config: "+trimmed)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "znf ensure: config:", err)
	} else if n > 0 {
		fmt.Fprintf(stdout, "znf config: đã ghi %d file\n", n) //znf:allow-lang
		for _, line := range strings.Split(cfgOut.String(), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "CREATE") || strings.HasPrefix(trimmed, "UPDATE") {
				fmt.Fprintln(stdout, line)
			}
		}
	}
	return rep
}

// resyncPluginIfStale is the former selfHeal body: when the plugin manifest's
// version stamp differs from the binary, re-run plugin.Sync. Fail-open.
// dest/manifestPath are derived from home so this stays in step with the
// sibling ensureWorkspace steps, which all key off the same home parameter
// (plugin.DefaultDest/DefaultManifest independently call os.UserHomeDir).
func resyncPluginIfStale(home string, stderr io.Writer) {
	dest := filepath.Join(home, ".claude", "skills", "znf")
	manifestPath := filepath.Join(dest, ".manifest.json")
	stamp := ""
	if m, err := managed.Load(manifestPath); err == nil {
		stamp = m.Version
	}
	if stamp == version.Current() {
		return
	}
	if _, err := plugin.Sync(dest, manifestPath); err != nil {
		fmt.Fprintln(stderr, "znf ensure: plugin sync:", err)
	}
}
