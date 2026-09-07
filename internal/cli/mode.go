package cli

type upMode int

const (
	modeWizard upMode = iota
	modeApply
	modeDryRun
)

// decideMode routes `zenify up` WITHOUT changing the meaning of --dry-run/--apply
// (preserves the FR-050 headless contract from commit 429062a). It only ADDS the
// TTY-wizard branch for a bare interactive invocation; every other context keeps
// the current print-plan behaviour (exit 0, never hangs).
func decideMode(isTTY, apply, dryRunSet, dryRun, jsonOut, nonInteractive bool) upMode {
	// Explicit flags win, everywhere.
	if apply {
		return modeApply
	}
	if dryRunSet && dryRun {
		return modeDryRun
	}
	if jsonOut || nonInteractive {
		return modeDryRun // headless default: dry-run preview, requires --apply to mutate
	}
	if isTTY {
		return modeWizard // the one new branch
	}
	return modeDryRun // non-TTY, no flags: preserve print-plan, don't hang (SC-9)
}
