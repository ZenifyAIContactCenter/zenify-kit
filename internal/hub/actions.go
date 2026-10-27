package hub

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
)

// CopyURL is the text `y` copies: the PR URL.
func CopyURL(pr PR) string { return pr.URL }

// MergeCommand is the text `m` copies. The hub never merges by itself.
func MergeCommand(pr PR) string {
	return fmt.Sprintf("gh pr merge %d --repo %s --merge", pr.Number, pr.Repo)
}

// OSC52 wraps s in the terminal clipboard escape. Inside tmux the sequence is
// sent through tmux passthrough so it reaches the outer terminal (also over SSH).
func OSC52(s string) string {
	seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\a"
	if os.Getenv("TMUX") != "" {
		return "\x1bPtmux;\x1b" + seq + "\x1b\\"
	}
	return seq
}

// FixPrompt is the text `f` types into the task's agent.
func FixPrompt(pr PR) string { return "/znf:fix " + pr.FailedURL }

// Ack records that the user saw pr's alert: it creates or touches
// acks/<owner>_<repo>_<n>, which the poller compares with AlertAt.
func Ack(home string, pr PR) error {
	if err := touch(filepath.Join(AcksDir(home), ackName(pr)), time.Now()); err != nil {
		return fmt.Errorf("ack %s: %w", prKey(pr), err)
	}
	return nil
}

// touch creates path (and its directory) if missing and sets its mtime to now.
func touch(path string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304 -- path is built from the hub data dir, not external input
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}

// ItemKind is the kind of one sidebar row.
type ItemKind int

const (
	KindHeader      ItemKind = iota
	KindRepoGroup            // a repo in TASKS; Enter or Space expands or collapses it
	KindTask                 // a worktree (task × repo), under its repo group or the merged group
	KindTaskPR               // the open PR of the worktree row above it
	KindAgent                // under a task with several agents, or in the AGENTS section when unattached
	KindPR                   // a PR that matches no task
	KindMergedGroup          // the "merged (n)" row; Enter or Space expands or collapses it
)

// Item is one sidebar row. Worktree rows and the rows under them carry Task
// and the Repo they render under, with that repo's PR; a worktree row carries
// Agent when the task has exactly one agent.
type Item struct {
	Kind     ItemKind
	Title    string // header text, repo group name, or worktree label
	Count    int    // header count; group size
	Expanded bool   // a repo or merged group is open
	Task     *Task
	Repo     *TaskRepo
	Agent    *Agent
	PR       *PR
}

func hasPR(t Task) bool {
	for _, r := range t.Repos {
		if r.PR != nil {
			return true
		}
	}
	return false
}

// worktree is one task in one of its repos.
type worktree struct {
	t *Task
	r *TaskRepo
}

// active reports work in flight: an agent on the task or an open PR here.
func (w worktree) active() bool { return len(w.t.Agents) > 0 || w.r.PR != nil }

// repoGroup is the worktrees of one repo, sorted for display.
type repoGroup struct {
	name   string
	ws     []worktree
	active bool
	top    int // highest roll-up rank of its active worktrees
}

// Flatten lays s out as rows: TASKS, AGENTS (unattached) and MY PRS (other
// PRs); headers are always present. TASKS groups worktrees by repo: repos
// with an active worktree come first (by highest roll-up, then name) and are
// open unless folds says otherwise, the rest follow by name, collapsed
// unless folds opens them. Inside a repo, active worktrees come first (by
// roll-up, then slug), then the rest by slug. A worktree whose repo is merged
// while its task has neither an agent nor an open PR goes into one merged
// group at the end, listed as repo/slug when expanded.
func Flatten(s State, expanded bool, folds map[string]bool) []Item {
	byName := map[string]*repoGroup{}
	var groups []*repoGroup
	var done []worktree
	for i := range s.Tasks {
		t := &s.Tasks[i]
		for j := range t.Repos {
			w := worktree{t: t, r: &t.Repos[j]}
			if w.r.Merged && len(t.Agents) == 0 && !hasPR(*t) {
				done = append(done, w)
				continue
			}
			g := byName[w.r.Name]
			if g == nil {
				g = &repoGroup{name: w.r.Name}
				byName[w.r.Name] = g
				groups = append(groups, g)
			}
			g.ws = append(g.ws, w)
			if w.active() {
				g.top = max(g.top, rank(t.State))
				g.active = true
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.active != b.active {
			return a.active
		}
		if a.active && a.top != b.top {
			return a.top > b.top
		}
		return a.name < b.name
	})
	n := 0
	for _, g := range groups {
		n += len(g.ws)
		sort.Slice(g.ws, func(i, j int) bool {
			a, b := g.ws[i], g.ws[j]
			if a.active() != b.active() {
				return a.active()
			}
			if a.active() && rank(a.t.State) != rank(b.t.State) {
				return rank(a.t.State) > rank(b.t.State)
			}
			return a.t.Slug < b.t.Slug
		})
	}
	sort.Slice(done, func(i, j int) bool { return label(done[i]) < label(done[j]) })

	items := []Item{{Kind: KindHeader, Title: "TASKS", Count: n}}
	for _, g := range groups {
		open := g.active
		if v, ok := folds[g.name]; ok {
			open = v
		}
		items = append(items, Item{Kind: KindRepoGroup, Title: g.name, Count: len(g.ws), Expanded: open})
		if open {
			for _, w := range g.ws {
				items = appendWorktree(items, s, w, w.t.Slug)
			}
		}
	}
	if len(done) > 0 {
		items = append(items, Item{Kind: KindMergedGroup, Count: len(done), Expanded: expanded})
		if expanded {
			for _, w := range done {
				items = appendWorktree(items, s, w, label(w))
			}
		}
	}
	items = append(items, Item{Kind: KindHeader, Title: "AGENTS", Count: len(s.Unattached)})
	for i := range s.Unattached {
		items = append(items, Item{Kind: KindAgent, Agent: &s.Unattached[i]})
	}
	items = append(items, Item{Kind: KindHeader, Title: "MY PRS", Count: len(s.OtherPRs)})
	for i := range s.OtherPRs {
		items = append(items, Item{Kind: KindPR, PR: &s.OtherPRs[i]})
	}
	return items
}

// label is a worktree's name in the merged group.
func label(w worktree) string { return w.r.Name + "/" + w.t.Slug }

// appendWorktree adds w's row titled title, its PR row when the repo has an
// open PR and, when the task has several agents, its agent rows.
func appendWorktree(items []Item, s State, w worktree, title string) []Item {
	var agents []*Agent
	for _, id := range w.t.Agents {
		if a, ok := s.Agents[id]; ok {
			agents = append(agents, &a)
		}
	}
	row := Item{Kind: KindTask, Title: title, Task: w.t, Repo: w.r, PR: w.r.PR}
	if len(agents) == 1 {
		row.Agent = agents[0]
	}
	items = append(items, row)
	if w.r.PR != nil {
		items = append(items, Item{Kind: KindTaskPR, Task: w.t, Repo: w.r, PR: w.r.PR})
	}
	if len(agents) > 1 {
		for _, a := range agents {
			items = append(items, Item{Kind: KindAgent, Task: w.t, Repo: w.r, Agent: a})
		}
	}
	return items
}

// itemKey identifies a row across layouts, so the cursor can follow it.
func itemKey(it Item) string {
	switch it.Kind {
	case KindHeader, KindRepoGroup:
		return fmt.Sprintf("%d/%s", it.Kind, it.Title)
	case KindMergedGroup:
		return "merged"
	case KindPR:
		return fmt.Sprintf("pr/%s#%d", it.PR.Repo, it.PR.Number)
	}
	k := strconv.Itoa(int(it.Kind))
	if it.Repo != nil {
		k += "/" + it.Repo.Name + "/" + it.Task.Slug
	}
	if it.Agent != nil {
		k += "/" + it.Agent.SessionID
	}
	return k
}

// TargetAgent picks the agent a row jumps to: an agent row is itself; a row
// inside a task picks the task agent with the highest roll-up state, ties
// broken by the newest UpdatedAt.
func TargetAgent(s State, it Item) (Agent, bool) {
	if it.Kind == KindAgent && it.Agent != nil {
		return *it.Agent, true
	}
	if it.Task == nil {
		return Agent{}, false
	}
	var best Agent
	found := false
	for _, id := range it.Task.Agents {
		a, ok := s.Agents[id]
		if !ok {
			continue
		}
		if !found || rank(a.State) > rank(best.State) ||
			(rank(a.State) == rank(best.State) && a.UpdatedAt > best.UpdatedAt) {
			best, found = a, true
		}
	}
	return best, found
}

// NewAgentWindow opens a window running claude in root, with a hub sidebar
// (self is the zenify binary) split to its left; focus stays on claude. It
// then enables allow-passthrough for OSC 52 copy; a failure there (tmux < 3.3)
// is returned for the footer after the window is complete.
func NewAgentWindow(r tmuxx.Runner, root, self string) error {
	out, err := r.Run("new-window", "-t", tmuxx.Target+":", "-P", "-F", "#{window_id}", "-c", root, "-n", "agent", "claude")
	if err != nil {
		return err
	}
	win := strings.TrimSpace(string(out))
	if win == "" {
		return errors.New("tmux new-window printed no window id")
	}
	if _, err := r.Run("split-window", "-d", "-h", "-b", "-l", "30%", "-t", win, "-c", root, self, "hub", "sidebar"); err != nil {
		return err
	}
	if _, err := r.Run("set-option", "-w", "-t", win, "allow-passthrough", "on"); err != nil {
		return fmt.Errorf("window opened, but allow-passthrough failed (copy may not reach the terminal): %w", err)
	}
	return nil
}
