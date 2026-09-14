package wt

import (
	"fmt"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// RepoCount is one repo's sweepable tally for the SessionStart report.
type RepoCount struct {
	Abbrev    string
	Removable int // live worktrees: merged + clean
	Stale     int // state entries whose dir is gone
}

// CountSweepable tallies what `wt sweep` would remove in repoRoot, reading git
// only (no fetch, no lsof, no writes). Used by the SessionStart hook, so it must
// stay cheap and side-effect free.
func CountSweepable(r gitx.Runner, repoRoot string) (RepoCount, error) {
	cfg, err := Load(repoRoot)
	if err != nil {
		return RepoCount{}, err
	}
	st, err := ReadState(repoRoot)
	if err != nil {
		return RepoCount{}, err
	}
	items, err := sweepPlan(r, repoRoot, cfg.BaseRef, cfg.WorktreeDir, st)
	if err != nil {
		return RepoCount{}, err
	}
	c := RepoCount{Abbrev: cfg.Abbrev} // Load already defaults Abbrev when unset (config.go)
	for _, it := range items {
		switch {
		case it.Stale:
			c.Stale++
		case it.Remove:
			c.Removable++
		}
	}
	return c, nil
}

// FormatReport renders the one-line SessionStart notice. Empty when there is
// nothing to sweep, so the hook prints nothing. Repos with zero are omitted;
// "+<stale>" is omitted when zero.
func FormatReport(counts []RepoCount) string {
	total, stale := 0, 0
	var parts []string
	for _, c := range counts {
		if c.Removable+c.Stale == 0 {
			continue
		}
		total += c.Removable
		stale += c.Stale
		if c.Stale > 0 {
			parts = append(parts, fmt.Sprintf("%s %d+%d", c.Abbrev, c.Removable, c.Stale))
		} else {
			parts = append(parts, fmt.Sprintf("%s %d", c.Abbrev, c.Removable))
		}
	}
	if total+stale == 0 {
		return ""
	}
	return fmt.Sprintf("wt: %d merged worktree(s) + %d stale entr(ies) sweepable — %s → run `wt sweep --all`", total, stale, strings.Join(parts, ", "))
}
