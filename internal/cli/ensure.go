package cli

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

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
func ensureWorkspace(workspace, home string, stdout, stderr io.Writer) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(stderr, "znf ensure: recovered:", r)
		}
	}()

	resyncPluginIfStale(home, stderr)

	if ch, err := apply.EnsureGlobalHooks(home, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: hooks:", err)
	} else if n := ch.Added + ch.Updated; n > 0 {
		fmt.Fprintf(stdout, "wired %d znf hooks\n", n)
	}

	if changed, err := apply.EnsureWorkspaceModel(workspace, false); err != nil {
		fmt.Fprintln(stderr, "znf ensure: model:", err)
	} else if changed {
		fmt.Fprintf(stdout, "pinned workspace model → %s\n", apply.DefaultModel)
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
