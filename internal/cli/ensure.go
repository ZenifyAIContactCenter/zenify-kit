package cli

import (
	"bytes"
	"fmt"
	"io"

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

	resyncPluginIfStale(stderr)

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

	// runConfig prints its whole plan to stdout; ensure only wants the count,
	// and its stderr fail-open note ("không đọc được manifest ... fail-open")
	// needs to surface as a "znf ensure:" line rather than being lost.
	var cfgErr bytes.Buffer
	n, err := runConfig(workspace, "", true, io.Discard, &cfgErr)
	if cfgErr.Len() > 0 {
		fmt.Fprint(stderr, "znf ensure: config: "+cfgErr.String())
	}
	if err != nil {
		fmt.Fprintln(stderr, "znf ensure: config:", err)
	} else if n > 0 {
		fmt.Fprintf(stdout, "znf config: đã ghi %d file\n", n) //znf:allow-lang
	}
}

// resyncPluginIfStale is the former selfHeal body: when the plugin manifest's
// version stamp differs from the binary, re-run plugin.Sync. Fail-open.
func resyncPluginIfStale(stderr io.Writer) {
	dest, err := plugin.DefaultDest()
	if err != nil {
		return
	}
	manifestPath, err := plugin.DefaultManifest()
	if err != nil {
		return
	}
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
