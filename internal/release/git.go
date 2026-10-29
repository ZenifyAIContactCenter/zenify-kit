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
// mainline with --first-parent; for each merge-commit that is a PR ("Merge pull request"), it
// sets PRBranch=branch on the merge-commit, THEN expands git log <merge>^1..<merge>^2 and sets
// PRBranch=branch on each expanded commit. A mainline commit that is not a PR (a direct push)
// keeps PRBranch=""; a non-PR merge (a back-merge) is dropped and its expanded commits stay
// plain, and an inner merge with nothing left after the outer exclusion is dropped. A release
// merge (a PR from the integration branch, e.g. staging→main) whose side walked with
// --first-parent holds "Merge pull request" commits is not a change of its own: its side is
// grouped the same way, so each inner PR keeps its own branch. Any other PR merge (a hotfix that
// merged a child PR) keeps its own identity. integration is the staging ref ("origin/staging"),
// compared by last path segment. Used for group-by-PR aggregation; counts/regression still use
// the flat RangeCommits.
func RangeCommitsGrouped(r gitx.Runner, dir, from, to, integration string) ([]Commit, error) {
	out, err := r.Run(dir, "log", "--first-parent", logFormat, from+".."+to)
	if err != nil {
		return nil, err
	}
	return groupMainline(r, dir, parseCommits(out), NormalizeKey(integration), nil), nil
}

// maxReleaseDepth bounds the recursion: only a merge from the integration branch expands, so
// real history nests one level; deeper merges group as an ordinary PR.
const maxReleaseDepth = 3

// groupMainline labels one first-parent walk. not holds ^<outer>^1 of each enclosing release
// merge: an inner side must not reach what the outer mainline already shipped (a main→staging
// sync would otherwise drag last week's hotfix back in).
func groupMainline(r gitx.Runner, dir string, mainline []Commit, integration string, not []string) []Commit {
	var cs []Commit
	for _, m := range mainline {
		if !m.Merge || m.Branch == "" {
			cs = append(cs, m)
			continue
		}
		side := append([]string{logFormat, m.SHA + "^1.." + m.SHA + "^2"}, not...)
		if len(not) < maxReleaseDepth && NormalizeKey(m.Branch) == integration {
			fout, err := r.Run(dir, append([]string{"log", "--first-parent"}, side...)...)
			if inner := parseCommits(fout); err == nil && hasPRMerge(inner) {
				cs = append(cs, groupMainline(r, dir, inner, integration, append(not[:len(not):len(not)], "^"+m.SHA+"^1"))...)
				continue
			}
		}
		bout, err := r.Run(dir, append([]string{"log"}, side...)...)
		children := parseCommits(bout)
		if err == nil && len(children) == 0 && len(not) > 0 {
			continue // nothing left after the outer exclusion (a main→staging sync): no empty row
		}
		if !prMergeRe.MatchString(m.Subject) {
			cs = append(cs, children...) // a back-merge is no PR: its commits stay plain, no row of its own
			continue
		}
		m.PRBranch = m.Branch
		cs = append(cs, m)
		for _, child := range children {
			child.PRBranch = m.Branch
			cs = append(cs, child)
		}
	}
	return cs
}

// hasPRMerge: cs holds a "Merge pull request" commit; a "Merge branch" back-merge does not count.
func hasPRMerge(cs []Commit) bool {
	for _, c := range cs {
		if prMergeRe.MatchString(c.Subject) {
			return true
		}
	}
	return false
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
