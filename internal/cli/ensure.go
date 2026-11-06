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
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

// ensureReport carries what the Codex steps of ensureWorkspace found, for the
// `zenify up` summary block (FR-7.2), and every local install step that failed.
type ensureReport struct {
	CodexInstalled bool
	CodexLinkPath  string
	CodexHooks     apply.HookChanges
	CodexAgents    int
	// LocalErrs has one "<step>: <err>" entry per local install step that failed.
	LocalErrs []string
}

// ensureOpts tunes ensureWorkspace. ForceLocal (set by `up`) writes the local
// hooks, git-guard, env and permissions even while ~/.claude/settings.json
// still holds the kit hooks; SessionStart and `zenify ensure` leave it false.
type ensureOpts struct{ ForceLocal bool }

// ensureWorkspace brings a workspace to the state the kit expects (W0 FR-01).
// Every kit write lands in the workspace (FR-1.6):
//
//  1. re-materialize the marketplace and update the local znf plugin when the
//     workspace's stamp is behind the running binary;
//  2. link <ws>/.agents/skills/znf to the plugin skills for Codex;
//  3. wire znf hooks, git-guard, the kit env keys (carrying a value the user
//     set at user level) and permission lists in <ws>/.claude/settings.local.json.
//     Unless opts.ForceLocal, each runtime is skipped while its own user copy
//     remains (a machine `up` has not migrated, or a migration that failed part
//     way), so no kit hook runs twice: znf hooks, env and permissions on the
//     kit hooks in ~/.claude/settings.json, git-guard on the user git-guard
//     (legacy script included). The Codex skills link is skipped likewise
//     while ~/.agents/skills/znf remains, so Codex never lists znf twice;
//  4. with codex on PATH: <ws>/.codex hooks and the workspace trust entry in
//     <codexHome>/config.toml (skipped while <codexHome>/hooks.json still has
//     kit hooks), and the agents;
//  5. pin the workspace default model in <workspace>/.claude/settings.json;
//  6. create the workspace marker .zenify/manifest.json when missing;
//  7. distribute the knowledge store's .config into the workspace
//     (`zenify config --apply`).
//
// Every step is fail-open: an error is one "znf ensure:" line on stderr and
// the next step still runs; a failed local install step is also recorded in
// rep.LocalErrs. stdout only gets a line when something changed, so a
// SessionStart on an up-to-date workspace prints nothing.
func ensureWorkspace(workspace, home string, stdout, stderr io.Writer, opts ensureOpts) (rep ensureReport) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(stderr, "znf ensure: recovered:", r)
			// The steps after the panic never ran: up keeps the user install.
			rep.LocalErrs = append(rep.LocalErrs, fmt.Sprintf("panic: %v", r))
		}
	}()
	localErr := func(step string, err error) {
		fmt.Fprintf(stderr, "znf ensure: %s: %v\n", step, err)
		rep.LocalErrs = append(rep.LocalErrs, step+": "+err.Error())
	}

	zh := zenifyHome(os.Getenv, func() (string, error) { return home, nil })
	// pluginRunner is runClaudeIn outside tests (TestMain swaps it).
	if err := resyncPluginLocal(workspace, zh, pluginRunner(workspace), stderr); err != nil {
		rep.LocalErrs = append(rep.LocalErrs, "plugin: "+err.Error()) // already on stderr
	}

	// Codex discovers skills under <ws>/.agents/skills, so this runs whether or
	// not codex is installed (FR-4.1).
	// Skipped (unless ForceLocal) while the user-level link remains, or Codex
	// lists every znf skill twice until `up` migrates.
	rep.CodexLinkPath = filepath.Join(workspace, ".agents", "skills", "znf")
	if fi, err := os.Stat(filepath.Join(home, ".agents", "skills", "znf")); err == nil && fi.IsDir() && !opts.ForceLocal { // only a live skills dir counts
		// user copy still serves Codex
	} else if st, err := apply.EnsureCodexSkillsLink(rep.CodexLinkPath, filepath.Join(plugin.PluginDir(zh), "skills"), false); err != nil {
		localErr("codex-skills", err)
	} else if st == apply.LinkRealDir {
		fmt.Fprintf(stderr, "znf ensure: codex-skills: %s là thư mục thật, bỏ qua\n", rep.CodexLinkPath) //znf:allow-lang
	}

	local := apply.WorkspaceLocalSettings(workspace)
	codexHome := codexHomeDir(home, os.Getenv)
	// Each runtime is gated on its own user copy, so a migration that removed
	// some user copies and failed on others never leaves one running twice.
	// A probe that errors loads nothing at user level, so it does not gate.
	wireLocal, wireGuard, wireCodex := true, true, true
	if !opts.ForceLocal {
		user := apply.UserSettings(home)
		if userHooks, err := apply.HasUserKitHooks(user); err != nil {
			fmt.Fprintln(stderr, "znf ensure: user-hooks:", err)
		} else {
			wireLocal = !userHooks
		}
		if userGuard, err := removeGuard(user, true); err == nil {
			wireGuard = !userGuard
		}
		// Without codex on PATH nothing Codex is written and user Codex hooks
		// `up` kept are inert, so they neither gate nor nudge.
		if codexOnPath() && !userCodexIsWorkspace(codexHome, workspace) {
			if ch, err := apply.RemoveCodexHooks(codexHome, true); err == nil {
				wireCodex = ch.Removed == 0
			}
		}
		if !wireLocal || !wireGuard || !wireCodex {
			fmt.Fprintln(stdout, "kit vẫn cài ở cấp user; chạy zenify up --apply để chuyển sang workspace") //znf:allow-lang
		}
	}

	if wireLocal {
		if ch, err := apply.EnsureGlobalHooks(local, false); err != nil {
			localErr("hooks", err)
		} else if n := ch.Added + ch.Updated; n > 0 {
			fmt.Fprintf(stdout, "wired %d znf hooks\n", n)
		}

	}
	// git-guard is a separate hook from the znf set above (it predates it and
	// `guard install` owns it), so it is wired here too — otherwise `up`
	// never installs it and deploy branches are unguarded on every machine.
	if wireGuard {
		if changed, err := installGuard(local); err != nil {
			localErr("git-guard", err)
		} else if changed {
			fmt.Fprintln(stdout, "wired git-guard hook (blocks commit/push on deploy branches)")
		}
	}

	if codexOnPath() {
		rep.CodexInstalled = true
		wsCodex := filepath.Join(workspace, ".codex")
		if wireCodex {
			if ch, err := apply.EnsureCodexHooks(wsCodex, false); err != nil {
				localErr("codex-hooks", err)
			} else if rep.CodexHooks = ch; ch.Added+ch.Updated > 0 {
				fmt.Fprintln(stdout, "Codex hook chưa chạy cho tới khi bạn trust một lần: mở codex, gõ /hooks, chọn trust.") //znf:allow-lang
			}
		}
		if ar, err := plugin.SyncCodexAgents(wsCodex, false); err != nil {
			localErr("codex-agents", err)
		} else {
			rep.CodexAgents = len(ar.Written) + len(ar.Unchanged)
			for _, n := range ar.Skipped {
				fmt.Fprintf(stderr, "znf ensure: codex-agents: %s.toml không phải của kit, bỏ qua\n", n) //znf:allow-lang
			}
			for _, n := range ar.Removed {
				fmt.Fprintf(stdout, "removed retired Codex agent %s.toml\n", n)
			}
		}
		if wireCodex { // trust runs the workspace hooks, so it waits for them
			if _, err := apply.EnsureCodexTrust(codexHome, workspace, false); err != nil {
				localErr("codex-trust", err)
			}
		}
	}

	if changed, err := apply.EnsureWorkspaceModel(workspace, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: model:", err)
	} else if changed {
		fmt.Fprintf(stdout, "pinned workspace model → %s\n", apply.DefaultModel)
	}

	if wireLocal {
		// A value the user chose at user level moves with them: the local file
		// wins over the user one, so seeding the default there would shadow it.
		for _, k := range []string{apply.SubagentModelEnv, apply.ReviewerEnv} {
			if _, err := apply.CarryEnv(local, apply.UserSettings(home), k, false); err != nil {
				localErr("env "+k, err)
			}
		}
		if changed, err := apply.EnsureSubagentModelEnv(local, false); err != nil {
			localErr("subagent model", err)
		} else if changed {
			fmt.Fprintf(stdout, "set %s=%s in .claude/settings.local.json (subagents that omit model now run there)\n",
				apply.SubagentModelEnv, apply.SubagentModelDefault)
		}
	}

	if changed, err := apply.RemoveStrongModelEnv(apply.UserSettings(home), false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: strong model:", err)
	} else if changed {
		fmt.Fprintf(stdout, "removed %s from ~/.claude/settings.json (select-route is gone)\n", apply.StrongModelEnv)
	}

	if wireLocal {
		if changed, err := apply.EnsureReviewerEnv(local, false); err != nil {
			localErr("reviewer", err)
		} else if changed {
			fmt.Fprintf(stdout, "set %s=%s in .claude/settings.local.json (zenify reviewer picks the review engine from it)\n",
				apply.ReviewerEnv, apply.ReviewerDefault)
		}

		// Rules the user removed at user level stay removed here; when that is
		// unknown, no kit rule is added rather than re-grant a removed one.
		if err := apply.SeedWorkspaceRecords(home, workspace); err != nil {
			localErr("kit records", err)
		} else {
			kt := apply.WorkspaceKitTarget(home, workspace)
			if n, err := apply.EnsureKitPermissions(kt, false); err != nil {
				localErr("permissions", err)
			} else if n > 0 {
				fmt.Fprintf(stdout, "allowed %d kit tool rules in .claude/settings.local.json\n", n)
			}
			if n, err := apply.EnsureKitSandbox(kt, false); err != nil {
				localErr("sandbox", err)
			} else if n > 0 {
				fmt.Fprintf(stdout, "excluded %d kit commands from the Bash sandbox in .claude/settings.local.json\n", n)
			}
			if n, err := apply.EnsureKitAsk(kt, false); err != nil {
				localErr("ask", err)
			} else if n > 0 {
				fmt.Fprintf(stdout, "added %d kit ask rules (prd data writes prompt) in .claude/settings.local.json\n", n)
			}
		}
	}

	// The workspace marker: wt.FindWorkspaceRoot looks for .zenify/manifest.json,
	// so a bare directory (the zenie worker's /work) is no workspace without it.
	if inside, err := ensureWorkspaceMarker(workspace, home); err != nil {
		fmt.Fprintln(stderr, "znf ensure: workspace marker:", err)
	} else if inside != "" {
		fmt.Fprintf(stderr, "znf ensure: workspace marker: %s is inside workspace %s, no marker created\n", workspace, inside)
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

// ensureWorkspaceMarker writes an empty .zenify/manifest.json when none exists
// and leaves an existing one untouched. It never creates one inside another
// workspace: wt.FindWorkspaceRoot picks the nearest marker, so a nested one
// would shadow the real workspace for every later command. It returns that
// enclosing workspace when it skips; $HOME (~/.zenify) is never one.
func ensureWorkspaceMarker(workspace, home string) (string, error) {
	marker := filepath.Join(workspace, ".zenify", "manifest.json")
	if _, err := os.Stat(marker); err == nil {
		return "", nil
	}
	if root, ok := wt.FindWorkspaceRoot(filepath.Dir(workspace)); ok && root != home {
		return root, nil
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil { //nolint:gosec // G301 -- same dir mode as the workspace's other kit dirs
		return "", err
	}
	m, err := managed.Load(marker)
	if err != nil {
		return "", err
	}
	return "", m.Save(marker)
}

// userCodexIsWorkspace reports whether CODEX_HOME is <workspace>/.codex: the
// Codex files there are the workspace install, not a user copy to gate on or
// migrate away.
func userCodexIsWorkspace(codexHome, workspace string) bool {
	return samePath(codexHome, filepath.Join(workspace, ".codex"))
}
