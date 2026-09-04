// Package analyze inspects a spec + plan pair mechanically: requirement
// coverage (FR -> plan task via _Requirements:), leftover clarification
// markers, and Brief structural shape. Pure and deterministic; all I/O
// lives in the cli wrapper. Every scan key is a kit convention (FR-, SC-,
// _Requirements:, [NEEDS CLARIFICATION, ## Brief), never a project-language
// label — the package stays project-agnostic.
package analyze

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Severity string

const (
	Critical Severity = "CRITICAL"
	High     Severity = "HIGH"
)

// Marker is one leftover [NEEDS CLARIFICATION ...] occurrence.
type Marker struct {
	File string `json:"file"` // "spec" or "plan"
	Line int    `json:"line"` // 1-indexed
	Text string `json:"text"`
}

// Finding is one mechanical defect.
type Finding struct {
	Severity Severity `json:"severity"`
	Kind     string   `json:"kind"` // orphan-fr | orphan-task | dangling-ref | marker
	ID       string   `json:"id,omitempty"`
	Location string   `json:"location,omitempty"`
	Message  string   `json:"message"`
}

// Result is the full mechanical analysis. MEDIUM findings are NOT produced
// here — they come from the judgment layer (the skill).
type Result struct {
	SpecFRs        []string            `json:"spec_frs"`
	PlanRefs       []string            `json:"plan_refs"`
	Coverage       map[string][]string `json:"coverage"`
	Findings       []Finding           `json:"findings"`
	BriefFound     bool                `json:"brief_found"`
	BriefFields    int                 `json:"brief_fields"`
	Markers        []Marker            `json:"markers"`
	SeverityCounts map[string]int      `json:"severity_counts"`
}

var (
	frRe          = regexp.MustCompile(`\bFR-\d+(?:\.\d+)*`)
	scRe          = regexp.MustCompile(`\bSC-\d+(?:\.\d+)*`)
	taskRe        = regexp.MustCompile(`^###\s+Task\b`)
	numberedRe    = regexp.MustCompile(`^\s*\d+\.\s`)
	briefRe       = regexp.MustCompile(`(?i)^##\s+brief\b`)
	nextSectionRe = regexp.MustCompile(`^#{1,2}\s`)
	reqLinePrefix = "_Requirements:"
	markerToken   = "[NEEDS CLARIFICATION"
)

// topLevel strips a sub-part: FR-1.2 -> FR-1, SC-3 -> SC-3.
func topLevel(id string) string {
	if i := strings.Index(id, "."); i >= 0 {
		return id[:i]
	}
	return id
}

func Analyze(specText, planText string) Result {
	r := Result{
		Coverage:       map[string][]string{},
		SeverityCounts: map[string]int{},
	}

	// --- spec FR set (top-level) ---
	frSet := map[string]bool{}
	for _, m := range frRe.FindAllString(specText, -1) {
		frSet[topLevel(m)] = true
	}
	for id := range frSet {
		r.SpecFRs = append(r.SpecFRs, id)
	}
	sort.Strings(r.SpecFRs)

	// --- plan: task blocks + _Requirements: refs ---
	refSet := map[string]bool{} // top-level FR/SC cited in plan
	planLines := strings.Split(planText, "\n")
	type block struct {
		title   string
		hasReqs bool
		refs    []string
	}
	var blocks []block
	cur := -1
	for _, ln := range planLines {
		if taskRe.MatchString(ln) {
			blocks = append(blocks, block{title: strings.TrimSpace(ln)})
			cur = len(blocks) - 1
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(ln), reqLinePrefix) {
			ids := append(frRe.FindAllString(ln, -1), scRe.FindAllString(ln, -1)...)
			for _, id := range ids {
				tl := topLevel(id)
				refSet[tl] = true
				if cur >= 0 {
					blocks[cur].refs = append(blocks[cur].refs, tl)
				}
			}
			if cur >= 0 {
				blocks[cur].hasReqs = true
			}
		}
	}
	for id := range refSet {
		r.PlanRefs = append(r.PlanRefs, id)
	}
	sort.Strings(r.PlanRefs)

	// --- coverage: FR -> tasks citing it ---
	for _, b := range blocks {
		for _, ref := range b.refs {
			if strings.HasPrefix(ref, "FR-") {
				r.Coverage[ref] = append(r.Coverage[ref], b.title)
			}
		}
	}

	// --- orphan FR (CRITICAL): spec FR not cited anywhere in plan ---
	for _, id := range r.SpecFRs {
		if !refSet[id] {
			r.add(Finding{Severity: Critical, Kind: "orphan-fr", ID: id,
				Message: "FR has no implementing task (orphan)"})
		}
	}
	// --- orphan task (HIGH): task block with no _Requirements: line ---
	for _, b := range blocks {
		if !b.hasReqs {
			r.add(Finding{Severity: High, Kind: "orphan-task", Location: b.title,
				Message: "task declares no _Requirements:"})
		}
	}
	// --- dangling ref (HIGH): plan cites an FR the spec never declares ---
	for _, id := range r.PlanRefs {
		if strings.HasPrefix(id, "FR-") && !frSet[id] {
			r.add(Finding{Severity: High, Kind: "dangling-ref", ID: id,
				Message: "plan cites an FR not declared in the spec"})
		}
	}

	// --- markers ---
	scanMarkers := func(text, file string) {
		for i, ln := range strings.Split(text, "\n") {
			if strings.Contains(ln, markerToken) {
				m := Marker{File: file, Line: i + 1, Text: strings.TrimSpace(ln)}
				r.Markers = append(r.Markers, m)
				r.add(Finding{Severity: High, Kind: "marker", Location: file + ":" + strconv.Itoa(i+1),
					Message: "unresolved clarification marker"})
			}
		}
	}
	scanMarkers(specText, "spec")
	scanMarkers(planText, "plan")

	// --- structural Brief ---
	specLines := strings.Split(specText, "\n")
	for i, ln := range specLines {
		if briefRe.MatchString(ln) {
			r.BriefFound = true
			for _, bl := range specLines[i+1:] {
				if nextSectionRe.MatchString(bl) {
					break
				}
				if numberedRe.MatchString(bl) {
					r.BriefFields++
				}
			}
			break
		}
	}

	return r
}

func (r *Result) add(f Finding) {
	r.Findings = append(r.Findings, f)
	r.SeverityCounts[string(f.Severity)]++
}
