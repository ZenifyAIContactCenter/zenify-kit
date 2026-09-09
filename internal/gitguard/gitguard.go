package gitguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Decision struct {
	Deny    bool
	Message string
}

var baselineDeny = []string{"main", "master", "production", "staging", "develop"}

// Decide is the final decision for one command.
func Decide(command, callCwd string, getenv func(string) string, onCommit func(repoDir string) Decision) Decision {
	calls := ParseGitCalls(command)
	if len(calls) == 0 {
		return Decision{}
	}
	// Only cares about commit/merge/push.
	var relevant []GitCall
	for _, c := range calls {
		switch c.Sub {
		case "commit", "merge", "push":
			relevant = append(relevant, c)
		}
	}
	if len(relevant) == 0 {
		return Decision{}
	}

	// Anchor: -C wins over cd wins over cwd wins over CLAUDE_PROJECT_DIR wins over PWD.
	base := firstNonEmpty(callCwd, getenv("CLAUDE_PROJECT_DIR"), getenv("PWD"))
	if cd := LeadingCd(command); cd != "" {
		base = resolveDir(base, cd, getenv)
	}
	for _, c := range relevant {
		repodir := base
		if c.RepoFlagC != "" {
			repodir = resolveDir(base, c.RepoFlagC, getenv)
		}
		branch := gitBranch(repodir)
		if branch == "" || branch == "HEAD" {
			continue // detached / not-a-repo → allow this call
		}
		root := repoRoot(repodir)
		patterns := loadDeny(root)

		switch c.Sub {
		case "push":
			if hasAllFlag(c.Args) {
				return deny("push", "'git push --all/--mirror' could push deploy branches")
			}
			target := pushTarget(c.Args, branch)
			if matchDeny(target, patterns) {
				return deny("push", "cannot push to deploy branch '"+target+"'")
			}
		case "commit", "merge":
			if matchDeny(branch, patterns) {
				return deny(c.Sub, "cannot commit/merge on deploy branch '"+branch+"'")
			}
			if c.Sub == "commit" && onCommit != nil {
				if d := onCommit(root); d.Deny {
					return d
				}
			}
		}
	}
	return Decision{}
}

func deny(sub, why string) Decision {
	return Decision{Deny: true, Message: "🚫 [git-guard] BLOCKED — " + why + " (" + sub + ")."}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return "."
}

func resolveDir(base, dir string, getenv func(string) string) string {
	switch {
	case filepath.IsAbs(dir):
		return dir
	case strings.HasPrefix(dir, "~"):
		return getenv("HOME") + dir[1:]
	default:
		return filepath.Join(base, dir)
	}
}

func hasAllFlag(args []string) bool {
	for _, a := range args {
		if a == "--all" || a == "--mirror" {
			return true
		}
	}
	return false
}

// pushTarget: the first refspec that is neither a flag nor the remote; if
// there is none → current branch; "src:dst" → dst; "+dst" (force) → dst
// (leading '+' stripped); "HEAD" (literal, after stripping '+') → current
// branch (the branch is resolved by the caller, not the literal "HEAD").
func pushTarget(args []string, branch string) string {
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		pos = append(pos, a)
	}
	// pos[0] = remote, pos[1] = refspec (if any).
	if len(pos) < 2 {
		return branch
	}
	ref := pos[1]
	ref = strings.TrimPrefix(ref, "+")
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		ref = ref[i+1:]
	}
	if ref == "HEAD" {
		return branch
	}
	return ref
}

func matchDeny(target string, patterns []string) bool {
	if target == "" {
		return false
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if ok, _ := filepath.Match(p, target); ok {
			return true
		}
	}
	return false
}

func gitBranch(dir string) string {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func repoRoot(dir string) string {
	out, err := runGit(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return dir
	}
	common := strings.TrimSpace(out)
	if common == "" {
		return dir
	}
	return filepath.Dir(common)
}

// loadDeny unions .claude/deploy-branches from root up through every
// ancestor; no file at all → baseline. A level declaring the NONE token
// (standalone) means "this subtree has NO deploy branch": stop the union
// there, contributing 0 branches (explicit opt-out, fail-safe: an
// empty/missing file does NOT exempt).
func loadDeny(root string) []string {
	var out []string
	found := false
	d := root
	for {
		f := filepath.Join(d, ".claude", "deploy-branches")
		if lines, ok := readLines(f); ok {
			found = true
			if hasNone(lines) {
				// Stop the union here. Assumption: a directory tree that has
				// declared NONE does NOT contain a nested git repo inside it
				// without its own deploy-branches — if it did, that nested
				// repo would lose the baseline. True for the
				// sibling-under-repos/ layout (docs is a sibling, nests no repo).
				return out
			}
			out = append(out, lines...)
		}
		if d == "/" || d == filepath.Dir(d) {
			break
		}
		d = filepath.Dir(d)
	}
	if !found {
		return baselineDeny
	}
	return out
}

// hasNone is true if a line is exactly "NONE" (opt-out sentinel).
func hasNone(lines []string) bool {
	for _, l := range lines {
		if l == "NONE" {
			return true
		}
	}
	return false
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204 -- fixed "git" binary, args are internally-computed git subcommands, not attacker-controlled shell input
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// readLines reads the file, skipping '#' comment lines and blank lines;
// ok=false when the file doesn't exist (or can't be read).
func readLines(path string) ([]string, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is the repo's own .claude/deploy-branches config location, not attacker-controlled
	if err != nil {
		return nil, false
	}
	var out []string
	for _, ln := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out, true
}
