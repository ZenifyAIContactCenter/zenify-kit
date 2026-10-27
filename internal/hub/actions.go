package hub

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	KindHeader ItemKind = iota
	KindTask
	KindRepo        // a task's worktree, with its PR when one is open
	KindAgent       // under a task with several agents, or in the AGENTS section when unattached
	KindPR          // a PR that matches no task
	KindMergedGroup // the "merged (n)" row; Enter expands or collapses it
)

// Item is one sidebar row. Task is set on every row inside a task. A task row
// carries Repo when its only repo has no PR (shown on the same line) and
// Agent when the task has exactly one agent.
type Item struct {
	Kind  ItemKind
	Title string // header text
	Count int    // header count; merged group size
	Task  *Task
	Repo  *TaskRepo
	Agent *Agent
	PR    *PR
}

// merged reports whether every repo of t is merged (and it has one).
func merged(t Task) bool {
	for _, r := range t.Repos {
		if !r.Merged {
			return false
		}
	}
	return len(t.Repos) > 0
}

func hasPR(t Task) bool {
	for _, r := range t.Repos {
		if r.PR != nil {
			return true
		}
	}
	return false
}

// Flatten lays s out as rows: TASKS, AGENTS (unattached) and MY PRS (other
// PRs); headers are always present. Tasks with an agent or a PR come first by
// roll-up state, then the rest by slug; tasks with neither an agent nor an
// open PR whose repos are all merged collapse into one merged-group row at
// the end, listed after it when expanded.
func Flatten(s State, expanded bool) []Item {
	var active, rest, done []*Task
	for i := range s.Tasks {
		t := &s.Tasks[i]
		switch {
		case len(t.Agents) > 0 || hasPR(*t): // work in flight: never hidden in the merged group
			active = append(active, t)
		case merged(*t):
			done = append(done, t)
		default:
			rest = append(rest, t)
		}
	}
	bySlug := func(ts []*Task) {
		sort.SliceStable(ts, func(i, j int) bool { return ts[i].Slug < ts[j].Slug })
	}
	bySlug(active)
	sort.SliceStable(active, func(i, j int) bool { return rank(active[i].State) > rank(active[j].State) })
	bySlug(rest)
	bySlug(done)

	items := []Item{{Kind: KindHeader, Title: "TASKS", Count: len(active) + len(rest)}}
	for _, t := range append(active, rest...) {
		items = appendTask(items, s, t)
	}
	if len(done) > 0 {
		items = append(items, Item{Kind: KindMergedGroup, Count: len(done)})
		if expanded {
			for _, t := range done {
				items = appendTask(items, s, t)
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

// appendTask adds t's row, its repo rows (none when its only repo has no PR)
// and, when it has several agents, its agent rows.
func appendTask(items []Item, s State, t *Task) []Item {
	var agents []*Agent
	for _, id := range t.Agents {
		if a, ok := s.Agents[id]; ok {
			agents = append(agents, &a)
		}
	}
	row := Item{Kind: KindTask, Task: t}
	if len(agents) == 1 {
		row.Agent = agents[0]
	}
	folded := len(t.Repos) == 1 && t.Repos[0].PR == nil
	if folded {
		row.Repo = &t.Repos[0]
	}
	items = append(items, row)
	if !folded {
		for j := range t.Repos {
			r := &t.Repos[j]
			items = append(items, Item{Kind: KindRepo, Task: t, Repo: r, PR: r.PR})
		}
	}
	if len(agents) > 1 {
		for _, a := range agents {
			items = append(items, Item{Kind: KindAgent, Task: t, Agent: a})
		}
	}
	return items
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
	out, err := r.Run("new-window", "-t", tmuxx.Session, "-P", "-F", "#{window_id}", "-c", root, "-n", "agent", "claude")
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
