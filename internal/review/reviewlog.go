package review

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FindingCounts: số finding kept theo severity.
type FindingCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

// Record: một review đã chạy, ghi ở POST (M4e learning-capture). KHÔNG lưu full
// findings — chỉ counts + categories (dimension mỗi kept finding).
type Record struct {
	TS         string        `json:"ts"`
	Repo       string        `json:"repo"`
	Base       string        `json:"base"`
	Head       string        `json:"head"`
	Tier       string        `json:"tier"`
	Outcome    string        `json:"outcome"`
	Findings   FindingCounts `json:"findings"`
	Kept       int           `json:"kept"`
	Refuted    int           `json:"refuted"`
	Shippable  bool          `json:"shippable"`
	Signals    []string      `json:"signals"`
	Categories []string      `json:"categories"`
}

// CategoryCount: một dimension và số lần xuất hiện (cho Summary).
type CategoryCount struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// Summary: tổng hợp nhiều record cho lệnh đọc.
type Summary struct {
	Total       int             `json:"total"`
	ByTier      map[string]int  `json:"by_tier"`
	Findings    FindingCounts   `json:"findings"`
	Kept        int             `json:"kept"`
	Refuted     int             `json:"refuted"`
	RefuteRate  float64         `json:"refute_rate"`
	ShippableN  int             `json:"shippable_n"`
	TopCategory []CategoryCount `json:"top_category"`
}

// sanitizeHead giữ chỉ [a-zA-Z0-9] cho an toàn tên file; rỗng → "unknown".
func sanitizeHead(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

// tsForFilename giữ chỉ [a-zA-Z0-9] khỏi ts, giống sanitizeHead — chặn escape
// dir qua ts độc hại (vd "../../..."). Không truncate (chỉ Head bị cắt 8 ký tự).
func tsForFilename(ts string) string {
	return sanitizeHead(ts)
}

// ensureDir tạo dir + self-ignoring .gitignore ("*") nếu chưa có (pattern SDD).
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	gi := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		_ = os.WriteFile(gi, []byte("*\n"), 0o600)
	}
	return nil
}

// WriteRecord ghi một record thành file <ts>-<head8>.json trong dir (tạo dir +
// self-ignore nếu chưa có). nil slice → [] trước encode; SetEscapeHTML(false);
// head sanitize để không escape dir; 0600.
func WriteRecord(dir string, r Record) (string, error) {
	if r.Signals == nil {
		r.Signals = []string{}
	}
	if r.Categories == nil {
		r.Categories = []string{}
	}
	if err := ensureDir(dir); err != nil {
		return "", err
	}
	head := sanitizeHead(r.Head)
	if len(head) > 8 {
		head = head[:8]
	}
	name := tsForFilename(r.TS) + "-" + head + ".json"
	path := filepath.Join(dir, name)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// LoadRecords đọc mọi *.json trong dir. dir thiếu → ([],nil). File hỏng → bỏ
// file đó, giữ phần còn lại (không drop cả tập).
func LoadRecords(dir string) ([]Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}
	recs := []Record{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // path từ ReadDir của dir nội bộ
		if err != nil {
			continue
		}
		var r Record
		if err := json.Unmarshal(b, &r); err != nil {
			continue
		}
		recs = append(recs, r)
	}
	return recs, nil
}

// Summarize cộng dồn record. RefuteRate = refuted/(kept+refuted), 0 nếu mẫu số 0.
// TopCategory giảm dần theo count, tie-break theo tên.
func Summarize(recs []Record) Summary {
	s := Summary{ByTier: map[string]int{}, TopCategory: []CategoryCount{}}
	catN := map[string]int{}
	for _, r := range recs {
		s.Total++
		s.ByTier[r.Tier]++
		s.Findings.Critical += r.Findings.Critical
		s.Findings.High += r.Findings.High
		s.Findings.Medium += r.Findings.Medium
		s.Findings.Low += r.Findings.Low
		s.Kept += r.Kept
		s.Refuted += r.Refuted
		if r.Shippable {
			s.ShippableN++
		}
		for _, c := range r.Categories {
			catN[c]++
		}
	}
	if denom := s.Kept + s.Refuted; denom > 0 {
		s.RefuteRate = float64(s.Refuted) / float64(denom)
	}
	for name, n := range catN {
		s.TopCategory = append(s.TopCategory, CategoryCount{Name: name, N: n})
	}
	sort.Slice(s.TopCategory, func(i, j int) bool {
		if s.TopCategory[i].N != s.TopCategory[j].N {
			return s.TopCategory[i].N > s.TopCategory[j].N
		}
		return s.TopCategory[i].Name < s.TopCategory[j].Name
	})
	return s
}
