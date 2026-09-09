package release

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

const sep = "\x1f"    // unit separator between fields
const recSep = "\x1e" // record separator between commits (body can span multiple lines)

// %b MUST come last (multi-line body); %an is inserted between subject and body.
const logFormat = "--format=%h" + sep + "%s" + sep + "%an" + sep + "%b" + recSep

// The trailing anchor is whitespace/EOL so it does NOT match variants like origin/release84-hotfix or release84.1.
var relNumRe = regexp.MustCompile(`(?m)origin/release(\d+)(?:\s|$)`)

// ReleaseNums lists the release numbers from `git branch -r` (origin/release<N> only), ascending.
func ReleaseNums(r gitx.Runner, dir string) ([]int, error) {
	out, err := r.Run(dir, "branch", "-r")
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, m := range relNumRe.FindAllStringSubmatch(string(out), -1) {
		n, _ := strconv.Atoi(m[1])
		seen[n] = true
	}
	var nums []int
	for n := range seen {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return nums, nil
}

// PrevRelease returns the highest release number < n in nums.
func PrevRelease(nums []int, n int) (int, bool) {
	prev, ok := 0, false
	for _, x := range nums {
		if x < n && x > prev {
			prev, ok = x, true
		}
	}
	return prev, ok
}

// Fetch runs `git fetch origin <refs...>`.
func Fetch(r gitx.Runner, dir string, refs ...string) error {
	args := append([]string{"fetch", "origin"}, refs...)
	_, err := r.Run(dir, args...)
	return err
}

func parseCommits(out []byte) []Commit {
	var cs []Commit
	for _, rec := range strings.Split(string(out), recSep) {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, sep, 4)
		if len(parts) < 2 {
			continue
		}
		author := ""
		if len(parts) >= 3 {
			author = parts[2]
		}
		body := ""
		if len(parts) == 4 {
			body = strings.TrimRight(parts[3], "\n")
		}
		c := Commit{SHA: parts[0], Subject: parts[1], Author: author, Body: body, Type: ClassifyType(parts[1])}
		if b := ParseMergeBranch(parts[1]); b != "" {
			c.Merge, c.Branch = true, b
		}
		cs = append(cs, c)
	}
	return cs
}

// RangeCommits returns the commits in from..to (merges kept, to catch PR/hotfix).
func RangeCommits(r gitx.Runner, dir, from, to string) ([]Commit, error) {
	out, err := r.Run(dir, "log", logFormat, from+".."+to)
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

// RangeCommitsGrouped returns the commits in from..to with PR LABELS attached: it walks the
// mainline with --first-parent; for each merge-commit that is a PR (ParseMergeBranch matches),
// it sets PRBranch=branch on the merge-commit, THEN expands git log <merge>^1..<merge>^2 and
// sets PRBranch=branch on each expanded commit. A mainline commit that is not a PR (a direct
// push, or a non-PR merge like a back-merge) keeps PRBranch="". Used for group-by-PR
// aggregation; counts/regression still use the flat RangeCommits.
func RangeCommitsGrouped(r gitx.Runner, dir, from, to string) ([]Commit, error) {
	out, err := r.Run(dir, "log", "--first-parent", logFormat, from+".."+to)
	if err != nil {
		return nil, err
	}
	mainline := parseCommits(out)
	var cs []Commit
	for _, m := range mainline {
		if m.Merge && m.Branch != "" {
			m.PRBranch = m.Branch
			cs = append(cs, m)
			bout, err := r.Run(dir, "log", logFormat, m.SHA+"^1.."+m.SHA+"^2")
			if err != nil {
				continue
			}
			for _, child := range parseCommits(bout) {
				child.PRBranch = m.Branch
				cs = append(cs, child)
			}
			continue
		}
		cs = append(cs, m)
	}
	return cs, nil
}

// ChangedFiles returns the list of files changed in from..to.
func ChangedFiles(r gitx.Runner, dir, from, to string) ([]string, error) {
	out, err := r.Run(dir, "diff", "--name-only", from+".."+to)
	if err != nil {
		return nil, err
	}
	var fs []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" {
			fs = append(fs, l)
		}
	}
	return fs, nil
}

// NotInStaging returns the commits in from..to that are NOT reachable from staging (regression risk).
func NotInStaging(r gitx.Runner, dir, from, to, staging string) ([]Commit, error) {
	out, err := r.Run(dir, "log", logFormat, from+".."+to, "--not", staging)
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

// CutDate returns the date (YYYY-MM-DD) of the merge-base between release<n> and origin/staging.
func CutDate(r gitx.Runner, dir string, n int) (string, error) {
	rel := fmt.Sprintf("origin/release%d", n)
	base, err := r.Run(dir, "merge-base", rel, "origin/staging")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(base))
	out, err := r.Run(dir, "log", "-1", "--format=%ci", sha)
	if err != nil {
		return "", err
	}
	d := strings.TrimSpace(string(out))
	if len(d) >= 10 {
		d = d[:10]
	}
	return d, nil
}
