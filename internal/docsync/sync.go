// Package docsync syncs the docs repo (agent-managed): commits changes the
// agent writes, then rebases onto remote and pushes. Modeled on Obsidian
// git-sync. Pure: injects gitx.Runner.
package docsync

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Sync syncs the docs repo at dir. FAIL-OPEN: always returns notes, never an error.
//
// Deliberate order (status-first, commit-first):
//  1. status --porcelain — LOCAL, no network. Clean → return immediately. Most
//     turns (the Stop hook runs every turn) are clean, so this costs no
//     network round-trip at all.
//  2. Dirty → add -A + commit (what the agent just wrote is now a commit, no
//     longer dirty).
//  3. pull --rebase — rebase our commit onto remote. On conflict, this is a
//     REAL rebase so it can be aborted: `rebase --abort` restores our commit
//     intact (just not pushed), and we NEVER commit over a conflict marker
//     and push garbage to the shared repo.
//  4. push.
//
// Clean but STILL has an unpushed commit (the clean-but-ahead path): an
// earlier transient pull/rebase failure left an already-made commit stuck
// locally. The tree is clean on the next turn, so if "clean → return
// immediately" held, that commit would stay stuck forever. So even when
// clean we still count ahead via a LOCAL rev-list (no network); only when
// ahead>0 do we touch the network, to push it through via the proper
// rebase+push path.
func Sync(r gitx.Runner, dir string) []string {
	st, err := r.Run(dir, "status", "--porcelain")
	if err != nil {
		return note(fmt.Sprintf("docs sync: status error: %s (fail-open)", gitErr(err)))
	}
	if strings.TrimSpace(string(st)) == "" {
		// Nothing to commit — but there may still be an unpushed local commit.
		if aheadCommits(r, dir) == 0 {
			return note("docs sync: clean") // no network, no commit
		}
		return pushPending(r, dir) // commit stuck from before → push it through
	}
	if _, err := r.Run(dir, "add", "-A"); err != nil {
		return note(fmt.Sprintf("docs sync: add error: %s (fail-open)", gitErr(err)))
	}
	msg := "chore(docs): sync " + time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if _, err := r.Run(dir, "commit", "-m", msg); err != nil {
		return note(fmt.Sprintf("docs sync: commit error: %s (fail-open)", gitErr(err)))
	}
	return pushPending(r, dir)
}

// aheadCommits counts local commits not yet on upstream, PURE LOCAL (no
// network — uses the existing remote-tracking ref). Can't be determined
// (upstream not set, output doesn't parse) → 0, to keep the "clean = no
// network touch" fast path.
func aheadCommits(r gitx.Runner, dir string) int {
	out, err := r.Run(dir, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// pushPending rebases the local commit onto remote then pushes. On conflict →
// abort, keeping the local commit intact (unpushed), fail-open. Shared by
// both the dirty path (just committed) and the clean-but-ahead path (commit
// stuck from before).
func pushPending(r gitx.Runner, dir string) []string {
	if _, err := r.Run(dir, "pull", "--rebase"); err != nil {
		// Most likely a rebase conflict. Do NOT commit over the marker: abort
		// to return to our commit (safe, unpushed), report it and skip this sync.
		_, _ = r.Run(dir, "rebase", "--abort")
		return note(fmt.Sprintf("docs sync: pull/rebase error: %s — aborted, local commit kept intact (not pushed). Skipping this sync (fail-open)", gitErr(err)))
	}
	if _, err := r.Run(dir, "push"); err != nil {
		return note(fmt.Sprintf("docs sync: push error: %s (fail-open, local commit kept intact)", gitErr(err)))
	}
	return note("docs sync: pushed")
}

func note(s string) []string { return []string{s} }

// gitErr renders a git failure WITH git's own reason. exec.ExitError's
// Error() is only "exit status N", which hides the actual cause (e.g.
// "Author identity unknown" on a machine with no user.email). The note is
// injected into the session on every turn, so only the first stderr line and
// git's final fatal:/error: line are kept, capped at maxReason runes.
func gitErr(err error) string {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return err.Error()
	}
	var lines []string
	for _, ln := range strings.Split(string(ee.Stderr), "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			lines = append(lines, ln)
		}
	}
	if len(lines) == 0 {
		return err.Error()
	}
	reason := lines[0]
	if last := lines[len(lines)-1]; len(lines) > 1 &&
		(strings.HasPrefix(last, "fatal:") || strings.HasPrefix(last, "error:")) {
		reason += " … " + last
	}
	if r := []rune(reason); len(r) > maxReason {
		reason = string(r[:maxReason]) + "…"
	}
	return fmt.Sprintf("%v: %s", err, reason)
}

const maxReason = 240
