// Package review cung cấp finding-verifier cơ học cho engine znf:review (seam VERIFY).
// Nó KHÔNG đánh giá finding có phải lỗi thật hay không (đó là việc của reviewer LLM);
// nó chỉ kiểm chứng CITATION: dòng+trích dẫn của finding có khớp file thật không.
package review

import (
	"strconv"
	"strings"
)

// window là số dòng lệch cho phép hai bên `line` khi tìm evidence (số dòng trôi sau edit).
const window = 3

// Finding theo _shared/finding-schema.md. Mọi field omitempty để round-trip không phình.
type Finding struct {
	Dimension string `json:"dimension,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Title     string `json:"title,omitempty"`
	File      string `json:"file,omitempty"`
	Line      string `json:"line,omitempty"`
	Issue     string `json:"issue,omitempty"`
	Fix       string `json:"fix,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Refuted   bool   `json:"refuted,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Result: findings CHỈ chứa finding kept; refuted bị loại và đếm riêng.
type Result struct {
	Findings []Finding `json:"findings"`
	Kept     int       `json:"kept"`
	Refuted  int       `json:"refuted"`
}

// Verify kiểm chứng từng finding. readFile được inject để test (cli truyền os.ReadFile).
func Verify(findings []Finding, readFile func(string) ([]byte, error)) Result {
	res := Result{Findings: []Finding{}}
	for _, f := range findings {
		if keep, out := verifyOne(f, readFile); keep {
			res.Findings = append(res.Findings, out)
		} else {
			res.Refuted++
		}
	}
	res.Kept = len(res.Findings)
	return res
}

func verifyOne(f Finding, readFile func(string) ([]byte, error)) (bool, Finding) {
	// Không định vị được → không thể verify cơ học, giữ nguyên.
	if f.File == "" || f.Line == "" {
		return true, f
	}
	n, ok := parseLine(f.Line)
	if !ok {
		f.Reason = "refuted: line không parse được: " + f.Line
		return false, f
	}
	if f.Evidence == "" {
		f.Reason = "unverified: no evidence"
		return true, f
	}
	data, err := readFile(f.File)
	if err != nil {
		f.Reason = "refuted: file không đọc được: " + f.File
		return false, f
	}
	lines := strings.Split(string(data), "\n")
	target := normalize(f.Evidence)
	lo := n - window
	if lo < 1 {
		lo = 1
	}
	hi := n + window
	if hi > len(lines) {
		hi = len(lines)
	}
	for i := lo; i <= hi; i++ {
		if strings.Contains(normalize(lines[i-1]), target) {
			return true, f
		}
	}
	f.Reason = "refuted: evidence không thấy quanh dòng " + f.Line
	return false, f
}

// normalize gom mọi run khoảng trắng thành một space và trim, để so khớp không lệ thuộc indent.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// parseLine lấy run chữ số đầu tiên của "N" hoặc "N-M". false nếu không có chữ số.
func parseLine(s string) (int, bool) {
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			n, _ := strconv.Atoi(s[start:i])
			return n, true
		}
	}
	if start >= 0 {
		n, _ := strconv.Atoi(s[start:])
		return n, true
	}
	return 0, false
}
