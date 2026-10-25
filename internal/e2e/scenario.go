package e2e

import (
	"fmt"
	"regexp"
	"strings"
)

// ACKind is the verification class of one acceptance criterion in a scenario file.
type ACKind string

const (
	ACKindE2E ACKind = "e2e"
	ACKindAsk ACKind = "hỏi"       //znf:allow-lang
	ACKindOut ACKind = "ngoài-e2e" //znf:allow-lang
)

// AC is one `## @ac:<n> [<kind>] <sentence>` section.
type AC struct {
	N     int
	Kind  ACKind
	Text  string
	Steps []string
	Sees  []string
}

// Scenario is a parsed `.znf/e2e/scenarios/ZEN-<n>.md`.
type Scenario struct {
	Key  string
	Role string
	ACs  []AC
	Raw  string
}

var (
	reScenTitle = regexp.MustCompile(`^#\s+(ZEN-\d+):`)
	reACHead    = regexp.MustCompile(`^##\s+@ac:(\d+)\s+\[([^\]]*)\]\s*(.*)$`)
	reStepItem  = regexp.MustCompile(`^\d+[.)]\s+(.*)$`)
	reSeeItem   = regexp.MustCompile(`^[-*]\s+(.*)$`)
)

// ParseScenario parses the frozen scenario format. Section keywords are Vietnamese by design.
func ParseScenario(src string) (Scenario, error) {
	sc := Scenario{Raw: src}
	lines := strings.Split(src, "\n")
	first := ""
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = strings.TrimSpace(l)
			break
		}
	}
	m := reScenTitle.FindStringSubmatch(first)
	if m == nil {
		return sc, fmt.Errorf("first line must be `# ZEN-<n>: <title>`")
	}
	sc.Key = m[1]

	var cur *AC
	mode := "" // "steps" | "sees" | "" (other)
	seen := map[int]bool{}
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(l, "Vai:") && cur == nil: //znf:allow-lang
			sc.Role = strings.TrimSpace(strings.TrimPrefix(l, "Vai:")) //znf:allow-lang
		case strings.HasPrefix(l, "## "):
			cur, mode = nil, ""
			h := reACHead.FindStringSubmatch(l)
			if h == nil {
				if strings.HasPrefix(l, "## @ac") {
					return sc, fmt.Errorf("malformed @ac heading: %q", l)
				}
				continue // e.g. the open-questions section
			}
			var n int
			fmt.Sscanf(h[1], "%d", &n)
			if seen[n] {
				return sc, fmt.Errorf("duplicate @ac:%d", n)
			}
			seen[n] = true
			kind := ACKind(h[2])
			if kind != ACKindE2E && kind != ACKindAsk && kind != ACKindOut {
				return sc, fmt.Errorf("unknown \"loại @ac\" %q in @ac:%d (want e2e, hỏi or ngoài-e2e)", h[2], n) //znf:allow-lang
			}
			sc.ACs = append(sc.ACs, AC{N: n, Kind: kind, Text: strings.TrimSpace(h[3])})
			cur = &sc.ACs[len(sc.ACs)-1]
		case cur == nil:
		case strings.HasPrefix(l, "Bước:"): //znf:allow-lang
			mode = "steps"
		case strings.HasPrefix(l, "Thấy:"): //znf:allow-lang
			mode = "sees"
		case strings.HasPrefix(l, "Dữ liệu sau:"): //znf:allow-lang
			mode = ""
		default:
			if mode == "steps" {
				if s := reStepItem.FindStringSubmatch(l); s != nil {
					cur.Steps = append(cur.Steps, strings.TrimSpace(s[1]))
				}
			} else if mode == "sees" {
				if s := reSeeItem.FindStringSubmatch(l); s != nil {
					cur.Sees = append(cur.Sees, strings.TrimSpace(s[1]))
				}
			}
		}
	}
	if len(sc.ACs) == 0 {
		return sc, fmt.Errorf("scenario has no @ac section")
	}
	return sc, nil
}
