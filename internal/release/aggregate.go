package release

import (
	"regexp"
	"strings"
)

// Aggregate groups []Commit into []Change by a normalized key (last-segment branch ∪ scope).
// A merge commit and its standalone commits with the same slug are grouped together. A commit
// with no branch/scope → bucket "misc:<type>".
// notStaging: the set of (short) SHAs in NotInStaging → marks Change.NotOnStaging.
func Aggregate(commits []Commit, notStaging map[string]bool) []Change {
	idx := map[string]int{}
	var out []Change

	keyOf := func(c Commit) (key, slug string) {
		if c.PRBranch != "" {
			s := NormalizeKey(c.PRBranch)
			return s, s
		}
		if c.Merge && c.Branch != "" {
			s := NormalizeKey(c.Branch)
			return s, s
		}
		if sc := ParseScope(c.Subject); sc != "" {
			s := NormalizeKey(sc)
			return s, s
		}
		return "misc:" + c.Type, "misc:" + c.Type
	}

	for _, c := range commits {
		key, slug := keyOf(c)
		i, ok := idx[key]
		if !ok {
			i = len(out)
			idx[key] = i
			out = append(out, Change{Slug: slug})
		}
		out[i].Commits = append(out[i].Commits, c)
		if notStaging[c.SHA] {
			out[i].NotOnStaging = true
		}
		if c.Merge && IsHotfixBranch(c.Branch) {
			out[i].IsHotfix = true
		}
		if pr := ParsePRNum(c.Subject); pr != "" && out[i].PRNum == "" {
			out[i].PRNum = pr
		}
	}

	for i := range out {
		out[i].Type = changeType(out[i])
		out[i].Title = changeTitle(out[i])
		out[i].Authors = changeAuthors(out[i])
		out[i].Desc = changeDesc(out[i])
	}
	return out
}

// changeAuthors: the devs who wrote the code (distinct, first-seen order). Merge commits are
// skipped because their %an is whoever CLICKED merge, not the code author. Only when a change
// has ONLY merge commits (no regular commit in range) do we fall back to the merger's name.
func changeAuthors(ch Change) []string {
	if a := collectAuthors(ch.Commits, true); len(a) > 0 {
		return a
	}
	return collectAuthors(ch.Commits, false)
}

func collectAuthors(commits []Commit, skipMerge bool) []string {
	seen := map[string]bool{}
	var authors []string
	for _, c := range commits {
		if skipMerge && c.Merge {
			continue
		}
		a := strings.TrimSpace(c.Author)
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		authors = append(authors, a)
	}
	return authors
}

// convPrefixRe matches a conventional-commit prefix so it can be stripped to get the description: "feat(x): foo" → "foo".
var convPrefixRe = regexp.MustCompile(`^(?:feat|fix|perf|refactor|chore)(?:\([^)]*\))?!?:\s*`)

// changeDesc: the subject of the first non-merge commit, with the type(scope): prefix stripped.
// No non-merge commit (or every subject is empty after stripping the prefix) → "".
func changeDesc(ch Change) string {
	for _, c := range ch.Commits {
		if c.Merge {
			continue
		}
		s := convPrefixRe.ReplaceAllString(strings.TrimSpace(c.Subject), "")
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

var typeRank = map[string]int{"feat": 4, "fix": 3, "perf": 3, "refactor": 3, "chore": 1, "other": 0}

// branchTypeFloor infers a minimum type from the PR's branch prefix: segment feat|fix|perf|refactor|chore.
// hotfix is already handled by ch.IsHotfix at the top of changeType, so it is skipped here.
func branchTypeFloor(branch string) string {
	for _, seg := range strings.Split(branch, "/") {
		switch seg {
		case "feat", "fix", "perf", "refactor", "chore":
			return seg
		}
	}
	return ""
}

// changeType: hotfix if IsHotfix; else the "strongest" type among the commits (feat > fix/perf/refactor > chore > other),
// with a branch-prefix floor so a feat/* PR always shows as feat even if its commits are all chore.
func changeType(ch Change) string {
	if ch.IsHotfix {
		return "hotfix"
	}
	best, bestType := -1, "other"
	for _, c := range ch.Commits {
		t := c.Type
		if t == "perf" || t == "refactor" {
			t = "fix"
		}
		r := typeRank[c.Type]
		if r > best {
			best, bestType = r, t
		}
	}
	branch := ""
	for _, c := range ch.Commits {
		if c.Merge && c.Branch != "" {
			branch = c.Branch
			break
		}
		if c.PRBranch != "" {
			branch = c.PRBranch
			break
		}
	}
	if floor := branchTypeFloor(branch); floor != "" {
		t := floor
		if t == "perf" || t == "refactor" {
			t = "fix"
		}
		if typeRank[floor] > best {
			bestType = t
		}
	}
	return bestType
}

func changeTitle(ch Change) string {
	if strings.HasPrefix(ch.Slug, "misc:") {
		return "Khác (" + strings.TrimPrefix(ch.Slug, "misc:") + ")" //znf:allow-lang
	}
	return HumanizeTitle(ch.Slug)
}
