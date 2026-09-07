package release

import (
	"regexp"
	"strings"
)

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
		out[i].Authors = changeAuthors(out[i])
		out[i].Desc = changeDesc(out[i])
	}
	return out
}

// changeAuthors: dev viết code (distinct, thứ tự gặp đầu). Bỏ merge-commit vì %an của nó là
// người BẤM merge, không phải tác giả code. Nếu change CHỈ có merge commit (không có commit
// thường trong khoảng) thì mới dùng tác giả merge làm best-available.
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

// convPrefixRe khớp prefix conventional-commit để cắt lấy phần mô tả: "feat(x): foo" → "foo".
var convPrefixRe = regexp.MustCompile(`^(?:feat|fix|perf|refactor|chore)(?:\([^)]*\))?!?:\s*`)

// changeDesc: subject của commit non-merge đầu tiên, đã cắt prefix type(scope):.
// Không có commit non-merge (hoặc mọi subject rỗng sau khi cắt prefix) → "".
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
