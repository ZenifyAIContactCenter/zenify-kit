package wt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// SweepItem is one worktree's sweep verdict. Stale marks a state.json entry
// whose directory is gone AND which git no longer lists — the port it records
// is held by nothing, so the entry itself is what sweep removes. BranchMerged
// is only computed for stale items (live items encode it in Remove/Reason).
type SweepItem struct {
	Slug         string
	Branch       string
	Port         string
	Path         string
	Reason       string
	Remove       bool
	Stale        bool
	BranchMerged bool
}

// sweepPlan decides, per worktree in the repo, whether it is safe to tear down.
// A worktree is removable only if it was created by wt (has wt.slug), is on a
// real branch (not detached), that branch has a merge trace in base, and its
// tree is clean. Everything else is kept, with the reason recorded. It also
// classifies stale state.json entries (dir gone, git no longer lists the path)
// via st — st == nil is treated as empty. Pure: reads git only through r, no
// side effects — this is the unit-tested core of sweep.
func sweepPlan(r gitx.Runner, repoRoot, base, worktreeDir string, st *StateFile) ([]SweepItem, error) {
	out, err := r.Run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	wtPrefix := filepath.Join(canon(repoRoot), worktreeDir) + string(filepath.Separator)
	listed := map[string]bool{} // canon paths git reports, for the stale check

	var items []SweepItem
	var curPath string
	flush := func() {
		defer func() { curPath = "" }()
		if curPath == "" {
			return
		}
		listed[canon(curPath)] = true
		if !strings.HasPrefix(canon(curPath), wtPrefix) {
			return
		}
		slug := cfgGet(r, curPath, "wt.slug")
		it := SweepItem{Slug: slug, Path: curPath}
		if slug == "" {
			it.Reason = "not created by wt"
			items = append(items, it)
			return
		}
		branch := ""
		if b, e := r.Run(curPath, "symbolic-ref", "--quiet", "--short", "HEAD"); e == nil {
			branch = strings.TrimSpace(string(b))
		}
		it.Branch = branch
		it.Port = cfgGet(r, curPath, "wt.port")
		if branch == "" {
			it.Reason = "detached HEAD"
			items = append(items, it)
			return
		}
		if !BranchMerged(r, repoRoot, branch, base) {
			it.Reason = fmt.Sprintf("no merge trace in %s", base)
			items = append(items, it)
			return
		}
		if cfgStatusDirty(r, curPath) {
			it.Reason = "merged but uncommitted changes"
			items = append(items, it)
			return
		}
		it.Remove = true
		it.Reason = "merged, clean"
		items = append(items, it)
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			curPath = strings.TrimPrefix(line, "worktree ")
		case line == "":
			flush()
		}
	}
	flush()

	// Stale state entries: recorded in state.json, directory gone, and git does
	// not list the path either. They hold a port and nothing else.
	if st != nil {
		for _, w := range st.Worktrees {
			if w.Path == "" || listed[canon(w.Path)] {
				continue
			}
			if _, e := os.Stat(w.Path); e == nil {
				continue
			}
			it := SweepItem{Slug: w.Slug, Branch: w.Branch, Path: w.Path, Reason: "stale: dir missing", Remove: true, Stale: true}
			if len(w.Ports) > 0 {
				it.Port = strconv.Itoa(w.Ports[0])
			}
			if w.Branch != "" {
				it.BranchMerged = BranchMerged(r, repoRoot, w.Branch, base)
			}
			items = append(items, it)
		}
	}
	return items, nil
}

// cfgStatusDirty reports whether the worktree at worktreePath has uncommitted
// changes (git status --porcelain non-empty).
func cfgStatusDirty(r gitx.Runner, worktreePath string) bool {
	out, _ := r.Run(worktreePath, "status", "--porcelain")
	return strings.TrimSpace(string(out)) != ""
}

// SweepOptions drives RunSweep. Stdout carries the per-task report (sweep is a
// report, not a value-returning command); side effects go through the real
// stopServer/closeWorkspace below.
type SweepOptions struct {
	RepoRoot string
	Host     string
	DryRun   bool
	Fetch    bool
	Quiet    bool // print nothing when no item is removable (wt new's auto-sweep)
	Pid      int
	Now      int64
	Runner   gitx.Runner
	Stdout   io.Writer
	Stderr   io.Writer
}

// RunSweep tears down every finished task in the repo: for each removable
// worktree it stops the dev server holding its port, closes the herdr workspace,
// then removes it via RunRm. Kept tasks print why. --dry-run reports without
// touching anything; --fetch refreshes origin first so a merged branch is not
// misread as unmerged against a stale base.
func RunSweep(o SweepOptions) error {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	r := o.Runner
	cfg, err := Load(o.RepoRoot)
	if err != nil {
		return err
	}
	if o.Fetch {
		if _, e := r.Run(o.RepoRoot, "fetch", "origin", "--quiet"); e != nil {
			_, _ = fmt.Fprintln(o.Stderr, "wt: fetch failed — merge state may be stale, so nothing may look removable")
		}
	}
	st, err := ReadState(o.RepoRoot)
	if err != nil {
		return err
	}
	items, err := sweepPlan(r, o.RepoRoot, cfg.BaseRef, cfg.WorktreeDir, st)
	if err != nil {
		return err
	}
	if o.Quiet {
		any := false
		for _, it := range items {
			if it.Remove {
				any = true
				break
			}
		}
		if !any {
			return nil
		}
	}
	removed, kept := 0, 0
	for _, it := range items {
		if !it.Remove {
			_, _ = fmt.Fprintf(o.Stdout, "wt: %s — %s, left alone\n", orDash(it.Slug), it.Reason)
			kept++
			continue
		}
		if o.DryRun {
			if it.Stale {
				_, _ = fmt.Fprintf(o.Stdout, "wt: %s — would drop stale entry (port %s, branch %s %s)\n", it.Slug, orDash(it.Port), orDash(it.Branch), mergedWord(it.BranchMerged))
			} else {
				_, _ = fmt.Fprintf(o.Stdout, "wt: %s — would stop port %s and remove (%s)\n", it.Slug, orDash(it.Port), it.Branch)
			}
			removed++
			continue
		}
		if it.Stale {
			if e := sweepStale(o, r, it); e != nil {
				_, _ = fmt.Fprintf(o.Stderr, "wt:   could not drop stale entry %s: %v\n", it.Slug, e)
				kept++
				continue
			}
			removed++
			continue
		}
		_, _ = fmt.Fprintf(o.Stdout, "wt: sweeping %s (%s)\n", it.Slug, it.Branch)
		stopServer(it.Path, o.Stdout)
		closeWorkspace(it.Path, o.Stdout)
		if e := RunRm(RmOptions{RepoRoot: o.RepoRoot, Slug: it.Slug, Host: o.Host, Pid: o.Pid, Now: o.Now, Runner: r, Stderr: o.Stderr}); e != nil {
			_, _ = fmt.Fprintf(o.Stderr, "wt:   could not remove %s: %v\n", it.Slug, e)
			kept++
			continue
		}
		removed++
	}
	if o.DryRun {
		_, _ = fmt.Fprintf(o.Stdout, "wt: %d would be removed, %d left alone\n", removed, kept)
	} else {
		_, _ = fmt.Fprintf(o.Stdout, "wt: swept %d, left %d\n", removed, kept)
	}
	return nil
}

func mergedWord(merged bool) string {
	if merged {
		return "merged"
	}
	return "unmerged"
}

// sweepStale drops a state entry whose directory is gone. A merged branch goes
// through RunRm with Force (the gone-dir path refuses without it, rm.go) so
// prune + branch -D + state removal + session-pointer release all happen in one
// place; an unmerged branch is never deleted — only the entry and git's stale
// registration go, and the kept branch is named on stdout.
func sweepStale(o SweepOptions, r gitx.Runner, it SweepItem) error {
	if it.BranchMerged {
		_, _ = fmt.Fprintf(o.Stdout, "wt: dropping stale entry %s (port %s, branch %s merged)\n", it.Slug, orDash(it.Port), it.Branch)
		return RunRm(RmOptions{RepoRoot: o.RepoRoot, Slug: it.Slug, Host: o.Host, Force: true, Pid: o.Pid, Now: o.Now, Runner: r, Stderr: o.Stderr})
	}
	if _, e := RemoveWorktree(o.RepoRoot, it.Slug, o.Pid, o.Host, o.Now); e != nil {
		return e
	}
	if _, e := r.Run(o.RepoRoot, "worktree", "prune"); e != nil {
		_, _ = fmt.Fprintf(o.Stderr, "wt:   worktree prune failed: %v\n", e)
	}
	_, _ = fmt.Fprintf(o.Stdout, "wt:   %s — state dropped, branch %s kept (unmerged)\n", it.Slug, orDash(it.Branch))
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// stopServer stops whatever dev server is serving a port FROM INSIDE path. It is
// a drop-in port of bash stop_server: anchor on the listening port (the one thing
// wt knows), then verify the owning process's cwd is under path before killing —
// a port can be held by something unrelated, and killing that is worse than
// leaving a server up. NOT unit-tested (external process interaction); judge for
// fidelity to the bash algorithm.
func stopServer(path string, out io.Writer) {
	real := path
	if rp, e := filepath.EvalSymlinks(path); e == nil {
		real = rp
	}
	pidsOut, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, pid := range strings.Fields(string(pidsOut)) {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		cwd := lsofCwd(pid)
		// cwd must be the worktree itself or genuinely inside it. A bare prefix
		// match (as bash does) would treat sibling ".../foobar" as inside ".../foo"
		// and kill the wrong dev server — the separator boundary captures the real
		// intent ("cwd under this worktree") without that false match.
		if cwd == "" || (cwd != real && !strings.HasPrefix(cwd, real+string(filepath.Separator))) {
			continue
		}
		n, e := strconv.Atoi(pid)
		if e != nil {
			continue
		}
		proc, e := os.FindProcess(n)
		if e != nil {
			continue
		}
		_ = proc.Signal(syscall.SIGTERM)
		// Poll up to ~2s for it to exit (signal 0 = liveness probe), then SIGKILL.
		gone := false
		for i := 0; i < 40; i++ {
			if proc.Signal(syscall.Signal(0)) != nil {
				gone = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !gone {
			_ = proc.Kill()
		}
		_, _ = fmt.Fprintf(out, "wt:   stopped pid %s under %s\n", pid, path)
	}
}

// lsofCwd returns the cwd of pid via `lsof -p <pid> -a -d cwd -Fn` (the `-a` is
// load-bearing: lsof ORs selectors otherwise). "" on any failure.
func lsofCwd(pid string) string {
	out, err := exec.Command("lsof", "-p", pid, "-a", "-d", "cwd", "-Fn").Output() //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

// closeWorkspace closes the herdr workspace bound to path, best-effort. A missing
// herdr never blocks a sweep, and the workspace this sweep runs inside is left
// open (closing it would kill the shell mid-sweep). Drop-in port of bash
// close_workspace; NOT unit-tested.
func closeWorkspace(path string, out io.Writer) {
	if _, err := exec.LookPath("herdr"); err != nil {
		return
	}
	listed, err := exec.Command("herdr", "workspace", "list").Output()
	if err != nil {
		return
	}
	ws := herdrWorkspaceFor(string(listed), path)
	if ws == "" {
		return
	}
	if cur := os.Getenv("HERDR_WORKSPACE_ID"); cur != "" && cur == ws {
		_, _ = fmt.Fprintf(out, "wt:   workspace %s is the one you are in — left open, close it yourself\n", ws)
		return
	}
	if e := exec.Command("herdr", "workspace", "close", ws).Run(); e == nil { //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
		_, _ = fmt.Fprintf(out, "wt:   closed herdr workspace %s\n", ws)
	}
}

// herdrWorkspaceFor parses `herdr workspace list` JSON and returns the
// workspace_id whose worktree.checkout_path matches path, or "" if none. Mirrors
// bash close_workspace's reader (wt:551-556): `.result` is either an array or an
// object with a `workspaces` array; each element carries `worktree.checkout_path`
// and `workspace_id`. Fail-safe: any parse failure or ambiguity returns "" so a
// wrong workspace is never closed. Matches on an exact path or a symlink-resolved
// equal (macOS /private/...) — never a prefix, which could match a sibling.
func herdrWorkspaceFor(listJSON, path string) string {
	type herdrWS struct {
		WorkspaceID string `json:"workspace_id"`
		Worktree    struct {
			CheckoutPath string `json:"checkout_path"`
		} `json:"worktree"`
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(listJSON), &envelope); err != nil || len(envelope.Result) == 0 {
		return ""
	}
	var list []herdrWS
	if err := json.Unmarshal(envelope.Result, &list); err != nil {
		var obj struct {
			Workspaces []herdrWS `json:"workspaces"`
		}
		if err := json.Unmarshal(envelope.Result, &obj); err != nil {
			return ""
		}
		list = obj.Workspaces
	}
	want := canon(path)
	for _, w := range list {
		cp := w.Worktree.CheckoutPath
		if cp == "" {
			continue
		}
		if cp == path || canon(cp) == want {
			return w.WorkspaceID
		}
	}
	return ""
}
