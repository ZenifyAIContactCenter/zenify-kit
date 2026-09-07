package release

import "strings"

// Aggregate gom []Commit thành []Change theo key chuẩn-hoá (last-segment branch ∪ scope).
// Merge-commit và commit lẻ cùng slug gom chung. Commit không branch/scope → bucket "misc:<type>".
// notStaging: set SHA (short) thuộc tập NotInStaging → đánh dấu Change.NotOnStaging.
func Aggregate(commits []Commit, notStaging map[string]bool) []Change {
	idx := map[string]int{}
	var out []Change

	keyOf := func(c Commit) (key, slug string) {
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
	}
	return out
}

// changeType: hotfix nếu IsHotfix; else loại "mạnh nhất" trong các commit (feat > fix/perf/refactor > chore > other).
func changeType(ch Change) string {
	if ch.IsHotfix {
		return "hotfix"
	}
	rank := map[string]int{"feat": 4, "fix": 3, "perf": 3, "refactor": 3, "chore": 1, "other": 0}
	best, bestType := -1, "other"
	for _, c := range ch.Commits {
		t := c.Type
		if t == "perf" || t == "refactor" {
			t = "fix"
		}
		r := rank[c.Type]
		if r > best {
			best, bestType = r, t
		}
	}
	return bestType
}

func changeTitle(ch Change) string {
	if strings.HasPrefix(ch.Slug, "misc:") {
		return "Khác (" + strings.TrimPrefix(ch.Slug, "misc:") + ")"
	}
	return HumanizeTitle(ch.Slug)
}
