// Package migrate plans and performs consolidating repos into one subdirectory (repos/).
// Pure: all I/O and git checks are injected. Fail-open.
package migrate

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

type Action string

const (
	Move   Action = "MOVE"
	Refuse Action = "REFUSE"
	Skip   Action = "SKIP" // already in toDir → skip (idempotent)
)

type Item struct {
	Name, From, To string
	Action         Action
	Reason         string
}

// BuildPlan classifies each repo: SKIP if already in toDir; REFUSE if the basename
// collides in multiple places (can't consolidate automatically); otherwise MOVE. No more
// dirty/worktree gate — v2 moves a dirty repo (os.Rename carries it along) and repairs
// worktrees after the move (see Apply).
func BuildPlan(root, toDir string, repos []workspace.Repo) []Item {
	target := filepath.Join(root, toDir)
	seen := map[string]int{}
	for _, rp := range repos {
		seen[rp.Name]++
	}
	var items []Item
	for _, rp := range repos {
		it := Item{Name: rp.Name, From: rp.Path}
		switch {
		case seen[rp.Name] > 1:
			it.Action = Refuse
			it.Reason = "repo name collides in multiple places — can't consolidate automatically, handle manually"
		case filepath.Dir(rp.Path) == target:
			it.Action = Skip
			it.Reason = "already in " + toDir + "/"
		default:
			it.To = filepath.Join(target, rp.Name)
			it.Action = Move
		}
		items = append(items, it)
	}
	return items
}

// ApplyIO bundles all injected I/O for Apply (tests use fakes, real use gitx+os in the CLI).
type ApplyIO struct {
	ListWT     func(repoDir string) ([]string, error)            // list worktrees BEFORE the move
	Move       func(from, to string) error                       // os.Rename
	MkdirAll   func(dir string) error                            // create the destination directory
	Repair     func(repoDir, wtPath string) error                // git worktree repair <new-path>
	Repoint    func(wtOld, wtNew, mainOld, mainNew string) error // re-point symlink node_modules
	UpdateYAML func(name, newPath string) error                  // update repos.yaml
	// Resolve returns the symlink-resolved path (filepath.EvalSymlinks + fallback Clean in the
	// CLI). `git worktree list` returns an ALREADY symlink-resolved path, while it.From (from
	// workspace.Discover) is a RAW path — so it.From must be resolved before prefix-matching in
	// newWorktreePath, otherwise an internal worktree under a workspace symlink (e.g. macOS
	// /var→/private/var) gets mistaken for one outside the repo.
	Resolve func(path string) string
}

// newWorktreePath maps the old worktree path → the path after the move. An internal
// worktree (under repoOld) is rebased onto repoNew; a worktree outside the repo (herdr)
// stays unchanged (not moved).
func newWorktreePath(wtOld, repoOld, repoNew string) string {
	rel, err := filepath.Rel(repoOld, wtOld)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return wtOld // outside repoOld → unchanged
	}
	return filepath.Join(repoNew, rel)
}

// Apply performs the move-and-repair, in TWO PASSES for repos.yaml (the manifest is
// updated last, after every move has settled). Each repo is atomic: list-worktrees → move
// → repair+repoint each worktree → move back on rollback if any step fails (that repo is
// Refused, YAML not updated). Fail-open across repos. Returns the notes.
func Apply(items []Item, io ApplyIO) []string {
	var notes []string
	var moved []Item

	// Pass 1: move + repair each repo (atomic).
	for _, it := range items {
		if it.Action != Move {
			continue
		}
		wts, err := io.ListWT(it.From)
		if err != nil {
			notes = append(notes, "skipping "+it.Name+": couldn't list worktrees: "+err.Error())
			continue
		}
		// Resolve BEFORE the move (while it.From still exists) — wts come from
		// `git worktree list` so they are ALREADY symlink-resolved; repoOld must be matched in
		// the same resolved form, otherwise newWorktreePath's prefix is off at the root and an
		// internal worktree gets mistaken for one outside the repo.
		repoOldResolved := io.Resolve(it.From)
		if err := io.MkdirAll(filepath.Dir(it.To)); err != nil {
			notes = append(notes, "couldn't create directory for "+it.Name+": "+err.Error())
			continue
		}
		if err := io.Move(it.From, it.To); err != nil {
			notes = append(notes, "couldn't move "+it.Name+": "+err.Error())
			continue
		}
		failed := ""
		okCount := 0
		for _, wt := range wts {
			wtNew := newWorktreePath(wt, repoOldResolved, it.To)
			if err := io.Repair(it.To, wtNew); err != nil {
				failed = "repair worktree " + wt + ": " + err.Error()
				break
			}
			if err := io.Repoint(wt, wtNew, it.From, it.To); err != nil {
				failed = "re-point symlink " + wt + ": " + err.Error()
				okCount++ // Repair succeeded for this wt → count it so the restore loop un-repairs it
				break
			}
			okCount++
		}
		if failed != "" {
			moveBackErr := io.Move(it.To, it.From)
			restoreErr := ""
			if moveBackErr == nil {
				// Worktrees BEFORE the failing one already finished repair+repoint into it.To —
				// moving back returns the repo to it.From, so they are now dangling. Un-repair each
				// one back to its old path (reversing the repoint direction) so we do NOT leave a
				// half-done state behind.
				for _, wt := range wts[:okCount] {
					wtNew := newWorktreePath(wt, repoOldResolved, it.To)
					if err := io.Repair(it.From, wt); err != nil {
						restoreErr += "; " + wt + ": " + err.Error()
						continue
					}
					if err := io.Repoint(wtNew, wt, it.To, it.From); err != nil {
						restoreErr += "; " + wt + ": " + err.Error()
					}
				}
			}
			switch {
			case moveBackErr != nil:
				notes = append(notes, "CRITICAL "+it.Name+": "+failed+" AND rollback failed: "+moveBackErr.Error()+" — check manually")
			case restoreErr != "":
				notes = append(notes, "refuse "+it.Name+": "+failed+" (rollback done, BUT some worktrees are NOT restored"+restoreErr+" — check manually: git worktree repair)")
			default:
				notes = append(notes, "refuse "+it.Name+": "+failed+" (rollback done, restored "+strconv.Itoa(okCount)+" previously repaired worktrees)")
			}
			continue
		}
		moved = append(moved, it)
		if len(wts) > 0 {
			notes = append(notes, "moved "+it.Name+" + repaired "+strconv.Itoa(len(wts))+" worktree(s)")
		}
	}

	// Pass 2: all moves/repairs have settled → update repos.yaml.
	for _, it := range moved {
		newPath := filepath.Base(filepath.Dir(it.To)) + "/" + it.Name
		if err := io.UpdateYAML(it.Name, newPath); err != nil {
			notes = append(notes, "moved "+it.Name+" but couldn't update repos.yaml: "+err.Error())
			continue
		}
		notes = append(notes, "updated repos.yaml: "+it.Name+" → "+newPath)
	}
	return notes
}
