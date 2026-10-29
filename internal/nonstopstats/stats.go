// Package nonstopstats reads the nonstop estimates ledger (estimates.md) and
// computes the learned size scale and the day/week statistics from it. It is
// the single source of those numbers for the orchestrator skill and the pane.
package nonstopstats

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	minLearn   = 20 // measured tasks needed before the scale is learned
	maxLearn   = 60 // most recent measured tasks used to learn it
	minRuleRow = 3  // lines with an actual needed before a calibration rule fires
	ruleWindow = 30 // most recent estimates lines the calibration rules read
	fieldSep   = " · "
	dateLayout = "2006-01-02"
	// Calibration rules are output data consumed by the orchestrator and the pane.
	RuleUp   = "est lên 1 size"   //znf:allow-lang
	RuleDown = "est xuống 1 size" //znf:allow-lang
	RuleTier = "thường"           //znf:allow-lang
)

var sizeRank = map[string]int{"S": 1, "M": 2, "L": 3, "XL": 4}
var sizePoints = map[string]int{"S": 1, "M": 2, "L": 4, "XL": 8}
var rankSize = []string{"", "S", "M", "L", "XL"}

// Line is one valid row of estimates.md. Actual is "" when the file has "-"
// (cancelled or restarted task); Minutes (work) and ReviewMin are 0 when the
// column has none. Path and Tier are "-" for a v1 line or when unrecorded.
type Line struct {
	Date      string
	ID        string
	Repo      string
	Kind      string
	Est       string
	Actual    string
	Minutes   int
	ReviewMin int
	Files     int
	Path      string
	Tier      string
}

// DurEntry is one run of durations.log; Date is the local date of its time.
type DurEntry struct {
	Date string
	ID   string
	Kind string
	Ms   int
}

// Durations is the parsed durations.log, in file order.
type Durations []DurEntry

// Tiers counts the reviews of the week by tier.
type Tiers struct {
	T1 int `json:"T1"`
	T2 int `json:"T2"`
	T3 int `json:"T3"`
}

// Scale holds the exclusive upper time bounds, in minutes, of S, M and L.
type Scale struct {
	Source string `json:"source"`
	N      int    `json:"n"`
	S      int    `json:"S"`
	M      int    `json:"M"`
	L      int    `json:"L"`
}

type Today struct {
	Date          string   `json:"date"`
	Hit           int      `json:"hit"`
	Total         int      `json:"total"`
	Over          int      `json:"over"`
	Under         int      `json:"under"`
	Points        int      `json:"points"`
	ReviewMinutes int      `json:"reviewMinutes"`
	IDs           []string `json:"ids"`
}

type Week struct {
	HitRate      *float64 `json:"hitRate"`
	PointsPerDay *float64 `json:"pointsPerDay"`
	ReviewShare  *float64 `json:"reviewShare"`
}

type Result struct {
	Missing   bool     `json:"missing"`
	Scale     Scale    `json:"scale"`
	Today     Today    `json:"today"`
	Week      Week     `json:"week"`
	Rules     []string `json:"rules"`
	Malformed int      `json:"malformed"`
	Tiers     Tiers    `json:"tiers"`
	// DurationsMalformed is set by the caller that reads durations.log.
	DurationsMalformed int `json:"durationsMalformed"`
}

// Parse reads the ledger text. Blank and '#' lines are skipped; any other line
// that is not a valid 10-field (v1) or 13-field (v2) row is counted as malformed.
func Parse(text string) (lines []Line, malformed int) {
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		l, ok := parseLine(raw)
		if !ok {
			malformed++
			continue
		}
		lines = append(lines, l)
	}
	return lines, malformed
}

func parseLine(raw string) (Line, bool) {
	f := strings.Split(raw, fieldSep)
	if len(f) != 10 && len(f) != 13 {
		return Line{}, false
	}
	if _, err := time.Parse(dateLayout, f[0]); err != nil {
		return Line{}, false
	}
	l := Line{Date: f[0], ID: f[1], Repo: f[2], Kind: f[3], Est: f[4], Actual: f[5], Files: -1, Path: "-", Tier: "-"}
	if sizeRank[l.Est] == 0 {
		return Line{}, false
	}
	if l.Actual == "-" {
		l.Actual = ""
	} else if sizeRank[l.Actual] == 0 {
		return Line{}, false
	}
	l.Minutes = parseMinutes(f[6])
	filesCol := 7
	if len(f) == 13 {
		l.ReviewMin = parseMinutes(f[7])
		filesCol = 8
		switch f[10] {
		case "spike", "bounded", "architectural", "-":
			l.Path = f[10]
		default:
			return Line{}, false
		}
		switch f[11] {
		case "T1", "T2", "T3", "-":
			l.Tier = f[11]
		default:
			return Line{}, false
		}
	}
	if n, err := strconv.Atoi(f[filesCol]); err == nil && n >= 0 {
		l.Files = n
	}
	return l, true
}

// ParseDurations reads durations.log (`<ISO time> · <id> · work|review · <ms>`).
// Any other non-blank line is malformed.
func ParseDurations(text string) (Durations, int) {
	var d Durations
	malformed := 0
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		f := strings.Split(raw, fieldSep)
		if len(f) != 4 || (f[2] != "work" && f[2] != "review") {
			malformed++
			continue
		}
		at, err := time.Parse(time.RFC3339, f[0])
		n, err2 := strconv.Atoi(f[3])
		if err != nil || err2 != nil || n <= 0 {
			malformed++
			continue
		}
		d = append(d, DurEntry{Date: at.In(time.Local).Format(dateLayout), ID: f[1], Kind: f[2], Ms: n})
	}
	return d, malformed
}

// ApplyDurations makes the log win over the ledger columns. The runs of id x
// dated in (previous date of x, d] go to one line of (d, x): the last one with
// an actual (a cancel then a redo the same day), else the last one. An id reused
// on another day does not collect the old runs; summed ms round up to minutes.
func ApplyDurations(lines []Line, d Durations) {
	byID := map[string][]int{}
	for i, l := range lines {
		byID[l.ID] = append(byID[l.ID], i)
	}
	for id, idx := range byID {
		sort.SliceStable(idx, func(a, b int) bool { return lines[idx[a]].Date < lines[idx[b]].Date })
		var dates []string
		target := map[string]int{}
		for _, i := range idx {
			date := lines[i].Date
			t, seen := target[date]
			if !seen {
				dates = append(dates, date)
			}
			if !seen || lines[i].Actual != "" || lines[t].Actual == "" {
				target[date] = i
			}
		}
		prev := ""
		for _, date := range dates {
			i := target[date]
			var work, review int
			var hasWork, hasReview bool
			for _, e := range d {
				if e.ID != id || e.Date > date || e.Date <= prev {
					continue
				}
				if e.Kind == "work" {
					work, hasWork = work+e.Ms, true
				} else {
					review, hasReview = review+e.Ms, true
				}
			}
			if hasWork {
				lines[i].Minutes = (work + 59999) / 60000
			}
			if hasReview {
				lines[i].ReviewMin = (review + 59999) / 60000
			}
			prev = date
		}
	}
}

// parseMinutes reads "~16m", "16m" or "45s" (rounded up); 0 means no measurement.
func parseMinutes(s string) int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "~")
	if len(s) < 2 {
		return 0
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0
	}
	switch s[len(s)-1] {
	case 'm':
		return n
	case 's':
		return (n + 59) / 60
	}
	return 0
}

// LearnScale derives the S/M/L bounds from the 60 most recent measured lines.
func LearnScale(lines []Line) Scale {
	var mins []int
	for _, l := range lines {
		if l.Actual != "" && l.Minutes > 0 {
			mins = append(mins, l.Minutes)
		}
	}
	if len(mins) > maxLearn {
		mins = mins[len(mins)-maxLearn:]
	}
	n := len(mins)
	if n < minLearn {
		return Scale{Source: "default", N: n, S: 5, M: 20, L: 60}
	}
	sort.Ints(mins)
	s := Scale{Source: "learned", N: n, S: nearestRank(mins, 25), M: nearestRank(mins, 50), L: nearestRank(mins, 90)}
	if s.M < s.S+1 {
		s.M = s.S + 1
	}
	if s.L < s.M+1 {
		s.L = s.M + 1
	}
	return s
}

// nearestRank returns the value at index ceil(p*n/100)-1 of a sorted slice.
func nearestRank(sorted []int, p int) int {
	idx := (p*len(sorted)+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

// effectiveActual is the higher of the time size and the files size; a line
// with no minutes keeps its recorded actual. "" means excluded from statistics.
func effectiveActual(l Line, s Scale) string {
	if l.Actual == "" {
		return ""
	}
	if l.Minutes == 0 {
		return l.Actual
	}
	r := 4
	switch {
	case l.Minutes < s.S:
		r = 1
	case l.Minutes < s.M:
		r = 2
	case l.Minutes < s.L:
		r = 3
	}
	if l.Files >= 0 {
		fr := 4
		switch {
		case l.Files <= 3:
			fr = 1
		case l.Files <= 10:
			fr = 2
		case l.Files <= 30:
			fr = 3
		}
		if fr > r {
			r = fr
		}
	}
	return rankSize[r]
}

// Compute builds the full result; today is YYYY-MM-DD.
func Compute(lines []Line, malformed int, today string) Result {
	scale := LearnScale(lines)
	res := Result{Scale: scale, Malformed: malformed, Rules: []string{}, Today: Today{Date: today, IDs: []string{}}}

	weekStart := ""
	if t, err := time.Parse(dateLayout, today); err == nil {
		weekStart = t.AddDate(0, 0, -6).Format(dateLayout)
	}
	type group struct{ n, up, down int }
	type tierGroup struct {
		n    int
		tier string
		same bool
	}
	groups := map[string]*group{}
	var weekHit, weekTotal, weekPoints, weekWork, weekReview int
	tierGroups := map[string]*tierGroup{}
	weekDays := map[string]bool{}

	for i, l := range lines {
		act := effectiveActual(l, scale)
		inRuleWindow := i >= len(lines)-ruleWindow
		inWeek := weekStart != "" && l.Date >= weekStart && l.Date <= today
		if l.Date == today {
			res.Today.IDs = append(res.Today.IDs, l.ID)
		}
		if inWeek && act != "" {
			weekDays[l.Date] = true
		}
		if act == "" {
			continue // cancelled: excluded from every statistic
		}
		if l.Date == today {
			res.Today.ReviewMinutes += l.ReviewMin
		}
		if inRuleWindow && l.Date <= today && l.Tier != "-" {
			tg := tierGroups[l.Repo+" "+l.Kind]
			if tg == nil {
				tg = &tierGroup{tier: l.Tier, same: true}
				tierGroups[l.Repo+" "+l.Kind] = tg
			}
			tg.n++
			tg.same = tg.same && l.Tier == tg.tier
		}
		if inWeek {
			weekWork += l.Minutes
			weekReview += l.ReviewMin
			switch l.Tier {
			case "T1":
				res.Tiers.T1++
			case "T2":
				res.Tiers.T2++
			case "T3":
				res.Tiers.T3++
			}
		}
		if inRuleWindow {
			g := groups[l.Repo+" "+l.Kind]
			if g == nil {
				g = &group{}
				groups[l.Repo+" "+l.Kind] = g
			}
			g.n++
			switch {
			case sizeRank[l.Est] < sizeRank[act]:
				g.up++
			case sizeRank[l.Est] > sizeRank[act]:
				g.down++
			}
		}
		if l.Date == today {
			res.Today.Total++
			res.Today.Points += sizePoints[act]
			switch {
			case l.Est == act:
				res.Today.Hit++
			case sizeRank[l.Est] > sizeRank[act]:
				res.Today.Over++
			default:
				res.Today.Under++
			}
		}
		if inWeek {
			weekTotal++
			weekPoints += sizePoints[act]
			if l.Est == act {
				weekHit++
			}
		}
	}

	if weekTotal > 0 {
		v := float64(weekHit) / float64(weekTotal)
		res.Week.HitRate = &v
	}
	if len(weekDays) > 0 {
		v := float64(weekPoints) / float64(len(weekDays))
		v = math.Round(v*100) / 100
		res.Week.PointsPerDay = &v
	}

	if weekWork+weekReview > 0 {
		v := float64(weekReview) / float64(weekWork+weekReview)
		res.Week.ReviewShare = &v
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		if g.n < minRuleRow {
			continue
		}
		if g.up == g.n {
			res.Rules = append(res.Rules, k+": "+RuleUp)
		} else if g.down == g.n {
			res.Rules = append(res.Rules, k+": "+RuleDown)
		}
	}
	tkeys := make([]string, 0, len(tierGroups))
	for k := range tierGroups {
		tkeys = append(tkeys, k)
	}
	sort.Strings(tkeys)
	for _, k := range tkeys {
		if g := tierGroups[k]; g.n >= minRuleRow && g.same {
			res.Rules = append(res.Rules, k+": "+RuleTier+" "+g.tier)
		}
	}
	return res
}
