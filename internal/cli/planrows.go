package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
)

// hooksRowState reports the HOOKS plan row's state via a dry-run
// EnsureGlobalHooks call — never writes (SC-10). A malformed/non-object
// settings.json is reported as a skip rather than an error, matching
// EnsureGlobalHooks' own fail-open contract.
func hooksRowState(home string) string {
	ch, err := apply.EnsureGlobalHooks(home, true) // dryRun
	if err != nil || ch.Skipped {
		return "skip: settings.json malformed"
	}
	pending := ch.Added + ch.Updated
	if pending == 0 {
		return "current"
	}
	return fmt.Sprintf("%d to wire", pending)
}

// docsRowState reports the DOCS-STORE plan row's state: the resolved
// knowledge-store path (resolveDocsStore is read-only — no clone, no write).
func docsRowState(workspace string) string {
	store := resolveDocsStore(workspace, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
	if store == "" {
		return "not resolved"
	}
	return "store: " + store
}

// planFooterRows renders the two synthetic HOOKS/DOCS-STORE rows appended to
// every plan render (headless dry-run text and the TUI plan render alike).
// Home-dir resolution failure (practically never) degrades to the same
// "malformed" skip state rather than erroring the whole row out.
func planFooterRows(workspace string) []string {
	hooks := "skip: settings.json malformed"
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		hooks = hooksRowState(home)
	}
	return []string{
		fmt.Sprintf("%-12s %s", "HOOKS", hooks),
		fmt.Sprintf("%-12s %s", "DOCS-STORE", docsRowState(workspace)),
	}
}

// printPlanFooterRows writes planFooterRows to w, one per line.
func printPlanFooterRows(w io.Writer, workspace string) {
	for _, r := range planFooterRows(workspace) {
		_, _ = fmt.Fprintln(w, r)
	}
}
