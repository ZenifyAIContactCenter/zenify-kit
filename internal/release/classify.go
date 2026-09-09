package release

import (
	"path"
	"regexp"
	"strings"
)

var typeRe = regexp.MustCompile(`^(feat|fix|perf|refactor|chore)(\([^)]*\))?!?:`)

// ClassifyType returns the conventional-commit type of a subject: feat|fix|perf|refactor|chore, else "other".
func ClassifyType(subject string) string {
	if m := typeRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return "other"
}

var prMergeRe = regexp.MustCompile(`^Merge pull request #\d+ from [^/]+/(.+)$`)
var branchMergeRe = regexp.MustCompile(`^Merge branch '([^']+)'`)

// ParseMergeBranch extracts the source branch name from a merge-commit subject; "" if it isn't a merge.
func ParseMergeBranch(subject string) string {
	if m := prMergeRe.FindStringSubmatch(subject); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := branchMergeRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return ""
}

// IsHotfixBranch: the branch carries a sign of an urgent patch (hotfix / cherry-pick).
func IsHotfixBranch(branch string) bool {
	b := strings.ToLower(branch)
	return strings.Contains(b, "hotfix") || strings.Contains(b, "cherry-pick")
}

var migrationRe = regexp.MustCompile(`(^|/)(migrations?|migrate)(/|$)`)

// IsMigrationPath: the path lives under a migration directory.
func IsMigrationPath(p string) bool { return migrationRe.MatchString(p) }

// IsTestPath: the path is a test file.
func IsTestPath(p string) bool {
	base := path.Base(p)
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasSuffix(base, "_test.go") || strings.Contains(p, "/__tests__/")
}

// scopeRe extracts the scope in feat(<scope>): — group 1.
var scopeRe = regexp.MustCompile(`^(?:feat|fix|perf|refactor|chore)\(([^)]+)\)!?:`)

// ParseScope returns the scope of a conventional-commit ("" if there is no scope).
func ParseScope(subject string) string {
	if m := scopeRe.FindStringSubmatch(subject); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// NormalizeKey normalizes a branch/scope into one grouping key: last path-segment, lowercase, _→-.
func NormalizeKey(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.ReplaceAll(s, "_", "-")
}

// HumanizeTitle turns a kebab slug into a readable phrase: "linked-fields" → "Linked fields".
func HumanizeTitle(slug string) string {
	s := strings.ReplaceAll(slug, "-", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var prNumRe = regexp.MustCompile(`Merge pull request #(\d+)`)

// ParsePRNum extracts the PR number from a merge-commit subject ("" if none).
func ParsePRNum(subject string) string {
	if m := prNumRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return ""
}

// MatchesAny matches a path against a list of glob patterns (supports a "**/" prefix), returning the first matching pattern + true.
func MatchesAny(p string, patterns []string) (string, bool) {
	for _, pat := range patterns {
		if ok, _ := path.Match(pat, p); ok {
			return pat, true
		}
		if strings.HasPrefix(pat, "**/") {
			suf := pat[3:]
			if ok, _ := path.Match(suf, path.Base(p)); ok {
				return pat, true
			}
			if strings.Contains(p, "/") {
				if ok, _ := path.Match(suf, p[strings.Index(p, "/")+1:]); ok {
					return pat, true
				}
			}
		}
	}
	return "", false
}
