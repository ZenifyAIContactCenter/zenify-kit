package release

import (
	"path"
	"regexp"
	"strings"
)

var typeRe = regexp.MustCompile(`^(feat|fix|perf|refactor|chore)(\([^)]*\))?!?:`)

// ClassifyType trả về loại conventional-commit của subject: feat|fix|perf|refactor|chore, else "other".
func ClassifyType(subject string) string {
	if m := typeRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return "other"
}

var prMergeRe = regexp.MustCompile(`^Merge pull request #\d+ from [^/]+/(.+)$`)
var branchMergeRe = regexp.MustCompile(`^Merge branch '([^']+)'`)

// ParseMergeBranch rút tên branch nguồn từ subject của merge-commit; "" nếu không phải merge.
func ParseMergeBranch(subject string) string {
	if m := prMergeRe.FindStringSubmatch(subject); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := branchMergeRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return ""
}

// IsHotfixBranch: branch mang dấu hiệu vá gấp (hotfix / cherry-pick).
func IsHotfixBranch(branch string) bool {
	b := strings.ToLower(branch)
	return strings.Contains(b, "hotfix") || strings.Contains(b, "cherry-pick")
}

var migrationRe = regexp.MustCompile(`(^|/)(migrations?|migrate)(/|$)`)

// IsMigrationPath: path nằm trong thư mục migration.
func IsMigrationPath(p string) bool { return migrationRe.MatchString(p) }

// IsTestPath: path là file test.
func IsTestPath(p string) bool {
	base := path.Base(p)
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasSuffix(base, "_test.go") || strings.Contains(p, "/__tests__/")
}

// scopeRe rút scope trong feat(<scope>): — group 1.
var scopeRe = regexp.MustCompile(`^(?:feat|fix|perf|refactor|chore)\(([^)]+)\)!?:`)

// ParseScope trả scope của conventional-commit ("" nếu không có scope).
func ParseScope(subject string) string {
	if m := scopeRe.FindStringSubmatch(subject); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// NormalizeKey chuẩn-hoá branch/scope về một key gom nhóm: last path-segment, lowercase, _→-.
func NormalizeKey(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.ReplaceAll(s, "_", "-")
}

// HumanizeTitle biến slug kebab thành câu người đọc: "linked-fields" → "Linked fields".
func HumanizeTitle(slug string) string {
	s := strings.ReplaceAll(slug, "-", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var prNumRe = regexp.MustCompile(`Merge pull request #(\d+)`)

// ParsePRNum rút số PR từ subject merge-commit ("" nếu không có).
func ParsePRNum(subject string) string {
	if m := prNumRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return ""
}

// MatchesAny khớp path với danh sách glob patterns (hỗ trợ prefix "**/"), trả pattern khớp đầu tiên + true.
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
