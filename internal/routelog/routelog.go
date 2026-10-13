// Package routelog records skill-routing decisions: one JSON file per
// record, written best-effort at the main checkout (.znf/route-log),
// read back by `zenify route-log`. The records hold plan metrics and manual
// advisor calls.
package routelog

import (
	"bufio"
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Record is one routing record plus what the caller learned afterwards.
type Record struct {
	TS             string            `json:"ts"`
	Repo           string            `json:"repo"`
	Branch         string            `json:"branch"`
	Site           string            `json:"site"` // plan-metrics; older records also carry architect|investigator|implementer|reviewer|manual
	Features       map[string]string `json:"features"`
	Gates          []string          `json:"gates"`
	Model          string            `json:"model"`   // the model actually sent (none = no dispatch)
	Trigger        string            `json:"trigger"` // manual|<phrase>|"" (manual site)
	Outcome        string            `json:"outcome"` // ""|dispatch_error
	FilesPred      int               `json:"files_pred"`
	Tasks          int               `json:"tasks"`
	Fanin          int               `json:"fanin"`
	ThinkingTokens int               `json:"thinking_tokens"`
	Runtime        string            `json:"runtime,omitempty"` // codex|claude|unknown, set by the CLI from env
}

// Summary is what `zenify route-log` prints.
type Summary struct {
	Total           int                       `json:"total"`
	BySite          map[string]int            `json:"by_site"`
	ModelBySite     map[string]map[string]int `json:"model_by_site"`
	GateFires       map[string]int            `json:"gate_fires"`
	ManualNoTrigger int                       `json:"manual_no_trigger"`
	ByRuntime       map[string]int            `json:"by_runtime"`
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

// randSuffix returns 6 lowercase hex chars; falls back to a base36 nanosecond timestamp
// if crypto/rand fails (should not happen in practice).
func randSuffix() string {
	b := make([]byte, 3)
	if _, err := crand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

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

// WriteRecord writes <ts>-<site>-<rand6>.json (append-only store; a record is never edited).
// The random suffix keeps two records with the same TS and Site (second granularity) from
// silently overwriting each other.
func WriteRecord(dir string, r Record) (string, error) {
	if r.Features == nil {
		r.Features = map[string]string{}
	}
	if r.Gates == nil {
		r.Gates = []string{}
	}
	if err := ensureDir(dir); err != nil {
		return "", err
	}
	path := filepath.Join(dir, sanitize(r.TS)+"-"+sanitize(r.Site)+"-"+randSuffix()+".json")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, buf.Bytes(), 0o600)
}

// LoadRecords reads every *.json; missing dir → empty; a corrupt file is dropped.
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
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // internal dir listed by ReadDir
		if err != nil {
			continue
		}
		var r Record
		if err := json.Unmarshal(b, &r); err != nil {
			continue
		}
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].TS < recs[j].TS })
	return recs, nil
}

// Summarize aggregates records for the report.
func Summarize(recs []Record) Summary {
	s := Summary{BySite: map[string]int{}, ModelBySite: map[string]map[string]int{}, GateFires: map[string]int{}, ByRuntime: map[string]int{}}
	for _, r := range recs {
		s.Total++
		s.BySite[r.Site]++
		rt := r.Runtime
		if rt == "" {
			rt = "unknown"
		}
		s.ByRuntime[rt]++
		if s.ModelBySite[r.Site] == nil {
			s.ModelBySite[r.Site] = map[string]int{}
		}
		s.ModelBySite[r.Site][r.Model]++
		for _, g := range r.Gates {
			s.GateFires[g]++
		}
		if r.Site == "manual" && r.Trigger == "" {
			s.ManualNoTrigger++
		}
	}
	return s
}

var (
	taskHeadingRe = regexp.MustCompile(`^### Task \d+`)
	fileBulletRe  = regexp.MustCompile("^- (?:Create|Modify|Test|Delete): `([^`]+)`")
)

// PlanMetrics counts `### Task N` headings and the distinct paths listed as
// `- Create|Modify|Test|Delete: \`path\` bullets under **Files:** blocks. A
// `path:10-20` line suffix is stripped so one file edited in two tasks counts once.
func PlanMetrics(plan []byte) (tasks, filesPred int) {
	seen := map[string]bool{}
	inFiles := false
	sc := bufio.NewScanner(bytes.NewReader(plan))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if taskHeadingRe.MatchString(line) {
			tasks++
			inFiles = false
			continue
		}
		if strings.HasPrefix(line, "**Files:**") {
			inFiles = true
			continue
		}
		if inFiles {
			if m := fileBulletRe.FindStringSubmatch(line); m != nil {
				p := m[1]
				if i := strings.LastIndex(p, ":"); i > 0 && strings.Trim(p[i+1:], "0123456789-") == "" {
					p = p[:i]
				}
				seen[p] = true
			} else if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "**") {
				inFiles = false
			}
		}
	}
	return tasks, len(seen)
}
