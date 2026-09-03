// Package review — phần doctrine (seam doctrine của znf:review, M4d).
// SanitizeVerified gỡ các dòng CHỈ-verdict khỏi block ## Verified của ship-pack,
// giữ dòng mang facts, để reviewer không bị anchor theo kết luận của tác giả.
// Cơ học, fail-open; KHÔNG dùng LLM.
package review

import "strings"

// verdictPhrases: dấu hiệu một dòng là CLAIM code-đúng (so khớp lowercase).
// KHÔNG có bare "correct" — quá rộng (đụng "correctness"); dùng cụm rõ nghĩa.
var verdictPhrases = []string{
	"✅", "verified", "looks good", "lgtm", "no issues", "no problem",
	"all correct", "works correctly", "passes review", "ready to ship",
	"shippable", "all good",
}

// gapPhrases: dòng nói VỀ chỗ THIẾU test — dòng giá trị nhất, KHÔNG bao giờ strip.
var gapPhrases = []string{
	"no test", "not covered", "untested", "no coverage",
	"chưa có test", "không có test", "không test",
}

// hasFactToken: dòng có bằng chứng cụ thể (số / path / lệnh) → giữ dù có verdict-phrase.
func hasFactToken(line string) bool {
	for _, r := range line {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	if strings.Contains(line, "/") {
		return true
	}
	for _, ext := range []string{".go", ".js", ".ts", ".py", ".md"} {
		if strings.Contains(line, ext) {
			return true
		}
	}
	trimmed := strings.TrimSpace(line)
	for _, cmd := range []string{"$ ", "pm ", "go ", "git ", "npm ", "yarn ", "bash "} {
		if strings.HasPrefix(trimmed, cmd) {
			return true
		}
	}
	return false
}

func containsAny(lower string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// SanitizeVerified gỡ các dòng chỉ-verdict. clean giữ nguyên thứ tự + dòng trắng;
// stripped là các dòng đã gỡ (đã trim). Fail-open: text rỗng → "", nil.
// Một dòng bị gỡ khi: có verdict-phrase, KHÔNG có fact-token, và KHÔNG phải dòng gap.
func SanitizeVerified(text string) (clean string, stripped []string) {
	if text == "" {
		return "", nil
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(line)
		if containsAny(lower, verdictPhrases) && !hasFactToken(line) && !containsAny(lower, gapPhrases) {
			stripped = append(stripped, strings.TrimSpace(line))
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), stripped
}
