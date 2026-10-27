package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

// TaskRepo is one worktree of a task.
type TaskRepo struct {
	Repo   string `json:"repo"` // owner/repo from origin; "" when origin is unusable
	Name   string `json:"name"` // basename of the repo directory
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Merged bool   `json:"merged"`
	PR     *PR    `json:"pr,omitempty"`
}

// Task is a worktree slug shared across repos.
type Task struct {
	Slug   string     `json:"slug"`
	Repos  []TaskRepo `json:"repos"`
	Agents []string   `json:"agents"` // session ids
	State  AgentState `json:"state"`
}

// State is the snapshot the poller writes and the sidebar renders.
type State struct {
	Tasks      []Task           `json:"tasks"`
	Unattached []Agent          `json:"unattached"`
	OtherPRs   []PR             `json:"other_prs"`
	Agents     map[string]Agent `json:"agents"`
	LocalAt    int64            `json:"local_at"`
	GitHubAt   int64            `json:"github_at"`
	// Owners are the GitHub owners of the workspace repos' origins; only
	// their PRs are shown.
	Owners []string `json:"owners"`
	// Error joins LocalError and GitHubError for display.
	Error string `json:"error"`
	// LocalError and GitHubError are each round's last errors with the round
	// time, kept until that round runs clean.
	LocalError  string `json:"local_error"`
	GitHubError string `json:"github_error"`
}

func statePath(home string) string { return filepath.Join(Dir(home), "state.json") }

// normalizeOrigin reduces an origin URL to owner/repo, or errors when the
// result is not exactly two non-empty path segments.
func normalizeOrigin(url string) (string, error) {
	slug := gitx.NormalizeRemote(url, nil)
	owner, repo, ok := strings.Cut(slug, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", fmt.Errorf("origin %q is not a github owner/repo URL", strings.TrimSpace(url))
	}
	return slug, nil
}

// CollectLocal groups the worktrees of every wt-managed repo under root by slug
// and returns the distinct origin owners, sorted. A repo whose config or
// worktree list fails is skipped; a repo whose origin cannot be read or parsed
// keeps its worktrees with an empty Repo (no PR can attach). Those errors are
// joined and returned with the remaining tasks, sorted by slug.
func CollectLocal(root string, git gitx.Runner) ([]Task, []string, error) {
	var errs []error
	bySlug := map[string]*Task{}
	ownerSet := map[string]bool{}
	for _, repoRoot := range wt.WorkspaceRepos(root) {
		name := filepath.Base(repoRoot)
		cfg, err := wt.Load(repoRoot)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		rows, err := wt.List(git, repoRoot, cfg)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: list worktrees: %w", name, err))
			continue
		}
		repo := ""
		if out, err := git.Run(repoRoot, "remote", "get-url", "origin"); err != nil {
			errs = append(errs, fmt.Errorf("%s: read origin: %w", name, err))
		} else if repo, err = normalizeOrigin(string(out)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		} else {
			owner, _, _ := strings.Cut(repo, "/")
			ownerSet[owner] = true
		}
		for _, row := range rows {
			if row.Stale { // directory gone: the worktree was swept
				continue
			}
			slug := row.Slug
			if slug == "(unmanaged)" { // no wt.slug config: name the task by its directory
				slug = filepath.Base(row.Path)
			}
			t, ok := bySlug[slug]
			if !ok {
				t = &Task{Slug: slug}
				bySlug[slug] = t
			}
			t.Repos = append(t.Repos, TaskRepo{Repo: repo, Name: name, Branch: row.Branch, Path: row.Path, Merged: row.Merged == "merged"})
		}
	}
	var tasks []Task // nil when nothing was collected, so Tick can keep old tasks
	for _, t := range bySlug {
		tasks = append(tasks, *t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Slug < tasks[j].Slug })
	var owners []string
	for o := range ownerSet {
		owners = append(owners, o)
	}
	sort.Strings(owners)
	return tasks, owners, errors.Join(errs...)
}

// FilterOwners keeps the PRs whose repo owner is in owners (case-insensitive).
// With no known owner (no origin read yet) every PR is dropped, so personal
// PRs never enter the state or the alert baseline.
func FilterOwners(prs []PR, owners []string) []PR {
	if len(owners) == 0 {
		return nil
	}
	var out []PR
	for _, p := range prs {
		owner, _, _ := strings.Cut(p.Repo, "/")
		for _, o := range owners {
			if strings.EqualFold(owner, o) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// Assemble attaches each PR to the task repo with the same (Repo, Branch) and
// each agent to the task with its slug. Unmatched PRs go to OtherPRs; agents
// whose slug matches no task (including a swept one) go to Unattached. The
// input tasks are not modified.
func Assemble(tasks []Task, prs []PR, agents map[string]Agent) State {
	s := State{Agents: agents}
	index := map[string]int{} // slug → position in s.Tasks
	repoAt := map[string][2]int{}
	for i, t := range tasks {
		nt := Task{Slug: t.Slug, Repos: make([]TaskRepo, len(t.Repos))}
		for j, r := range t.Repos {
			r.PR = nil
			nt.Repos[j] = r
			if r.Repo != "" {
				repoAt[r.Repo+"\x00"+r.Branch] = [2]int{i, j}
			}
		}
		s.Tasks = append(s.Tasks, nt)
		index[t.Slug] = i
	}
	for _, p := range prs {
		if at, ok := repoAt[p.Repo+"\x00"+p.Branch]; ok {
			pr := p
			s.Tasks[at[0]].Repos[at[1]].PR = &pr
			continue
		}
		s.OtherPRs = append(s.OtherPRs, p)
	}
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	states := map[int][]AgentState{}
	for _, id := range ids {
		a := agents[id]
		i, ok := index[a.Slug]
		if !ok || a.Slug == "" {
			s.Unattached = append(s.Unattached, a)
			continue
		}
		s.Tasks[i].Agents = append(s.Tasks[i].Agents, a.SessionID)
		states[i] = append(states[i], a.State)
	}
	for i := range s.Tasks {
		s.Tasks[i].State = Rollup(states[i]...)
	}
	return s
}

// LoadAgents reads every agents/*.json. A missing directory yields an empty
// map. Files that fail to read or decode are skipped and reported in the
// joined error alongside the agents that loaded.
func LoadAgents(home string) (map[string]Agent, error) {
	out := map[string]Agent{}
	entries, err := os.ReadDir(AgentsDir(home))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}
	var errs []error
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(AgentsDir(home), e.Name()))
		if err != nil {
			errs = append(errs, fmt.Errorf("read agent file %s: %w", e.Name(), err))
			continue
		}
		var a Agent
		if err := json.Unmarshal(b, &a); err != nil {
			errs = append(errs, fmt.Errorf("decode agent file %s: %w", e.Name(), err))
			continue
		}
		out[a.SessionID] = a
	}
	return out, errors.Join(errs...)
}

// WriteState writes state.json atomically.
func WriteState(home string, s State) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := writeFileAtomic(Dir(home), statePath(home), ".state-*.tmp", b); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// ReadState loads state.json; a missing file yields an empty State.
func ReadState(home string) (State, error) {
	var s State
	b, err := os.ReadFile(statePath(home))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, fmt.Errorf("decode state: %w", err)
	}
	return s, nil
}

// Poller collects worktrees, agents and PRs into state.json. Its local and
// GitHub rounds may run concurrently; each merges into the latest state.json.
type Poller struct {
	Home, Root string
	Git        gitx.Runner
	GH         ghx.Runner
	Tmux       tmuxx.Runner
	Now        func() int64 // unix seconds

	// LocalDuration is how long the last local round's CollectLocal took, for
	// the caller to log (wt.List across many repos may be slow).
	LocalDuration time.Duration

	mu sync.Mutex // serializes the read-modify-write of state.json
}

// Local collects worktrees and agents and writes them into state.json,
// keeping the latest GitHub data. A source that fails outright keeps its
// previous data; partial results replace it. Errors are recorded in
// State.LocalError with the round time (empty after a clean round) and
// returned joined for the caller to log.
func (p *Poller) Local() error {
	now := p.Now()
	start := time.Now()
	tasks, owners, collectErr := CollectLocal(p.Root, p.Git)
	p.LocalDuration = time.Since(start)
	agents, agentsErr := LoadAgents(p.Home)

	p.mu.Lock()
	defer p.mu.Unlock()
	var errs []error
	old, err := ReadState(p.Home)
	if err != nil {
		errs = append(errs, err)
	}
	localAt := now
	if collectErr != nil {
		errs = append(errs, fmt.Errorf("collect worktrees: %w", collectErr))
		if tasks == nil { // nothing collected: the old tasks are as old as before
			tasks = old.Tasks
			localAt = old.LocalAt
		}
		if owners == nil {
			owners = old.Owners
		}
	}
	if agentsErr != nil {
		errs = append(errs, agentsErr)
		if agents == nil {
			agents = old.Agents
		}
	}
	prs, err := DropAcked(allPRs(old), AcksDir(p.Home))
	if err != nil {
		errs = append(errs, err)
	}

	s := Assemble(tasks, FilterOwners(prs, owners), agents)
	s.Owners, s.LocalAt, s.GitHubAt = owners, localAt, old.GitHubAt
	s.LocalError, s.GitHubError = stamp(now, errs), old.GitHubError
	return p.write(s, errs)
}

// GitHub fetches the user's PRs, fires alerts and writes them into
// state.json, keeping the latest local data. A failed fetch keeps the
// previous PRs. Errors are recorded in State.GitHubError with the round time
// (empty after a clean round) and returned joined for the caller to log.
func (p *Poller) GitHub() error {
	cur, fetchErr := FetchPRs(p.GH)

	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.Now() // after the fetch, so alerts are not older than the data
	var errs []error
	old, err := ReadState(p.Home)
	if err != nil {
		errs = append(errs, err)
	}
	prs := allPRs(old)
	githubAt := old.GitHubAt
	if fetchErr != nil {
		errs = append(errs, fetchErr)
	} else {
		var fired []PR
		prs, fired = DetectAlerts(prs, FilterOwners(cur, old.Owners), now)
		githubAt = now
		for _, f := range fired {
			msg := fmt.Sprintf("%s#%d: %s", path.Base(f.Repo), f.Number, f.Alert)
			// tmux expands #{...} and #(cmd) in the message; ## is a literal #.
			if err := tmuxx.DisplayMessage(p.Tmux, strings.ReplaceAll(msg, "#", "##")); err != nil {
				errs = append(errs, fmt.Errorf("tmux display-message: %w", err))
			}
		}
	}
	prs, err = DropAcked(prs, AcksDir(p.Home))
	if err != nil {
		errs = append(errs, err)
	}

	s := Assemble(old.Tasks, prs, old.Agents)
	s.Owners, s.LocalAt, s.GitHubAt = old.Owners, old.LocalAt, githubAt
	s.LocalError, s.GitHubError = old.LocalError, stamp(now, errs)
	return p.write(s, errs)
}

// write sets s.Error from both rounds' errors and writes state.json; it
// returns errs joined with any write error.
func (p *Poller) write(s State, errs []error) error {
	var shown []string
	for _, e := range []string{s.LocalError, s.GitHubError} {
		if e != "" {
			shown = append(shown, e)
		}
	}
	s.Error = strings.Join(shown, "; ")
	if err := WriteState(p.Home, s); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// stamp is errs joined and prefixed with the round time, or "" without errors.
func stamp(now int64, errs []error) string {
	joined := errors.Join(errs...)
	if joined == nil {
		return ""
	}
	return fmt.Sprintf("%s %v", time.Unix(now, 0).UTC().Format(time.RFC3339), joined)
}
