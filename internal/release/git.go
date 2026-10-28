package release

import (
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

// relTagRe matches a release tag vYYMMDD.n exactly: semver tags (v0.52.0), suffixed tags
// (v261028.0-rc) and legacy release<N> names are not release tags.
var relTagRe = regexp.MustCompile(`^v(\d{6})\.(\d+)$`)

// IsReleaseTag reports whether tag is a release tag vYYMMDD.n.
func IsReleaseTag(tag string) bool { return relTagRe.MatchString(tag) }

// tagKey returns (date, n) for a release tag; ok=false when tag is not vYYMMDD.n.
func tagKey(tag string) (date, n int, ok bool) {
	m := relTagRe.FindStringSubmatch(tag)
	if m == nil {
		return 0, 0, false
	}
	date, _ = strconv.Atoi(m[1])
	n, _ = strconv.Atoi(m[2])
	return date, n, true
}

// tagLess orders two release tags by date, then by number (v261028.10 after v261028.2).
func tagLess(a, b string) bool {
	da, na, _ := tagKey(a)
	db, nb, _ := tagKey(b)
	if da != db {
		return da < db
	}
	return na < nb
}

// ReleaseTags lists the repo's release tags vYYMMDD.n (weekly and hotfix), oldest first.
func ReleaseTags(dir string) ([]string, error) {
	return releaseTags(gitx.ExecRunner(), dir)
}

func releaseTags(r gitx.Runner, dir string) ([]string, error) {
	out, err := r.Run(dir, "tag", "--list")
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, l := range strings.Split(string(out), "\n") {
		if t := strings.TrimSpace(l); relTagRe.MatchString(t) {
			tags = append(tags, t)
		}
	}
	sort.Slice(tags, func(i, j int) bool { return tagLess(tags[i], tags[j]) })
	return tags, nil
}

// PrevTag returns the newest tag in tags that sorts before tag — the lower bound of tag's range,
// hotfix tags included.
func PrevTag(tags []string, tag string) (string, bool) {
	prev, ok := "", false
	for _, t := range tags {
		if tagLess(t, tag) && (!ok || tagLess(prev, t)) {
			prev, ok = t, true
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

// ReachableCommits returns every commit reachable from ref (merges kept, newest first). Used by
// spec-status: a merged feature's commit carries the Spec: trailer or a slug that links its spec,
// so scanning the whole base branch tells which specs have shipped.
func ReachableCommits(r gitx.Runner, dir, ref string) ([]Commit, error) {
	out, err := r.Run(dir, "log", logFormat, ref)
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
