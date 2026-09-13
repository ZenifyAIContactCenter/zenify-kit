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
	var parked, alarm, detached []string
	for _, s := range states {
		switch {
		case s.Dirty > 0 && s.Branch == "HEAD":
			// IsDeploy always returns false for "HEAD" (detached), so this is
			// never also OnDeploy — keep it out of the deploy alarm entirely.
			dirty = append(dirty, fmt.Sprintf("  %s [%s] %d file(s)  ⚠ DETACHED HEAD", s.Name, s.Branch, s.Dirty))
			detached = append(detached, fmt.Sprintf("%s[%s]", s.Name, s.Branch))
		case s.Dirty > 0:
			warn := ""
			if s.OnDeploy {
				warn = "  ⚠ EDITING ON A DEPLOY BRANCH"
				alarm = append(alarm, fmt.Sprintf("%s[%s]", s.Name, s.Branch))
			}
			dirty = append(dirty, fmt.Sprintf("  %s [%s] %d file(s)%s", s.Name, s.Branch, s.Dirty, warn))
		case s.Branch == "HEAD":
			parked = append(parked, s.Name+" [HEAD] ⚠ DETACHED HEAD")
		case !s.OnDeploy:
			parked = append(parked, s.Name)
		}
		if s.Behind > 0 {
			lb := strings.TrimPrefix(s.BaseRef, "origin/")
			stale = append(stale, fmt.Sprintf("  %s: local %s is %d behind %s", s.Name, lb, s.Behind, s.BaseRef))
		}
	}
	if mode == Stop {
		if len(alarm) == 0 && len(detached) == 0 {
			return ""
		}
		var parts []string
		if len(alarm) > 0 {
			parts = append(parts, "⚠ Uncommitted changes sit on a DEPLOY branch: "+strings.Join(alarm, " ")+
				"\n  Branch now and move the work across — do not wait for git-guard to block the commit."+
				"\n  `wt new <slug>` (or `git stash` → `git checkout -b <user>/<type>/<desc>` → `git stash pop`).")
		}
		if len(detached) > 0 {
			parts = append(parts, "⚠ Uncommitted changes on a DETACHED HEAD: "+strings.Join(detached, " ")+
				"\n  Check out a branch before committing — a detached-HEAD commit is easy to lose."+
				"\n  `git checkout -b <user>/<type>/<desc>`.")
		}
		return strings.Join(parts, "\n")
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
