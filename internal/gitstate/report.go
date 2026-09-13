package gitstate

import (
	"fmt"
	"strings"
)

// Report renders states. Session: the full <git-state> block, exceptions only,
// summary line always present. Stop: only the standing alarm, "" otherwise.
// The Session wording is byte-compatible with the bash hook so CLAUDE.md and
// discipline text keep matching it.
func Report(states []RepoState, mode Mode) string {
	var dirty, stale []string
	var parked, alarm []string
	for _, s := range states {
		switch {
		case s.Dirty > 0:
			warn := ""
			if s.OnDeploy {
				warn = "  ⚠ EDITING ON A DEPLOY BRANCH"
			}
			if s.Branch == "HEAD" {
				warn = "  ⚠ DETACHED HEAD"
			}
			dirty = append(dirty, fmt.Sprintf("  %s [%s] %d file(s)%s", s.Name, s.Branch, s.Dirty, warn))
			if warn != "" {
				alarm = append(alarm, fmt.Sprintf("%s[%s]", s.Name, s.Branch))
			}
		case !s.OnDeploy && s.Branch != "HEAD":
			parked = append(parked, s.Name)
		}
		if s.Behind > 0 {
			lb := strings.TrimPrefix(s.BaseRef, "origin/")
			stale = append(stale, fmt.Sprintf("  %s: local %s is %d behind %s", s.Name, lb, s.Behind, s.BaseRef))
		}
	}
	if mode == Stop {
		if len(alarm) == 0 {
			return ""
		}
		return "⚠ Uncommitted changes sit on a DEPLOY branch: " + strings.Join(alarm, " ") +
			"\n  Branch now and move the work across — do not wait for git-guard to block the commit." +
			"\n  `wt new <slug>` (or `git stash` → `git checkout -b <user>/<type>/<desc>` → `git stash pop`)."
	}
	var b strings.Builder
	b.WriteString("<git-state>\n")
	fmt.Fprintf(&b, "%d repo(s) in scope.\n", len(states))
	if len(dirty) > 0 {
		b.WriteString("Uncommitted work in flight:\n" + strings.Join(dirty, "\n") + "\n")
	}
	if len(parked) > 0 {
		b.WriteString("Clean but parked on a task branch: " + strings.Join(parked, " ") + "\n")
	}
	if len(stale) > 0 {
		b.WriteString("Local base branch behind remote (since last fetch — re-fetch, and read origin/<base>, NOT the working tree, when grounding):\n" + strings.Join(stale, "\n") + "\n")
	}
	if len(dirty) > 0 || len(parked) > 0 {
		b.WriteString("Rule #8: a NEW task starting in one of those repos needs `wt new`, not `checkout -b`.\n")
	} else {
		b.WriteString("All clean, all on their base branch — `checkout -b` is correct for a new task.\n")
	}
	b.WriteString("</git-state>\n")
	return b.String()
}
