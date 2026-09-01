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

// Decide là quyết định cuối cho một command.
func Decide(command, callCwd string, getenv func(string) string, onCommit func(repoDir string) Decision) Decision {
	calls := ParseGitCalls(command)
	if len(calls) == 0 {
		return Decision{}
	}
	// Chỉ quan tâm commit/merge/push.
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

	// Anchor: -C thắng cd thắng cwd thắng CLAUDE_PROJECT_DIR thắng PWD.
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
			continue // detached / not-a-repo → allow lời gọi này
		}
		root := repoRoot(repodir)
		patterns := loadDeny(root)

		switch c.Sub {
		case "push":
			if hasAllFlag(c.Args) {
				return deny("push", "'git push --all' could push deploy branches")
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
		if a == "--all" {
			return true
		}
	}
	return false
}

// pushTarget: refspec đầu tiên không phải flag và không phải remote; nếu không
// có → nhánh hiện tại; "src:dst" → dst.
func pushTarget(args []string, branch string) string {
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		pos = append(pos, a)
	}
	// pos[0] = remote, pos[1] = refspec (nếu có).
	if len(pos) < 2 {
		return branch
	}
	ref := pos[1]
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[i+1:]
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

// loadDeny union .claude/deploy-branches từ root lên mọi ancestor; không file
// nào → baseline.
func loadDeny(root string) []string {
	var out []string
	found := false
	d := root
	for {
		f := filepath.Join(d, ".claude", "deploy-branches")
		if lines, ok := readLines(f); ok {
			found = true
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

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// readLines đọc file, bỏ dòng comment '#' và dòng rỗng; ok=false khi file
// không tồn tại (hoặc không đọc được).
func readLines(path string) ([]string, bool) {
	b, err := os.ReadFile(path)
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
