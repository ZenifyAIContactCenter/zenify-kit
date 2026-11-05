package trackstats

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	minLearn   = 20 // measured items needed before the scale is learned
	maxLearn   = 60 // most recent measured items used to learn it
	minRuleRow = 3  // items with an estimate before a calibration rule fires
	ruleWindow = 30 // most recent items per kind the estimate and rules read
	staleDays  = 7  // a start older than this with no end is stale
	dateLayout = "2006-01-02"

	// RuleUp and RuleDown are suffixes of the calibration rules ("<kind>: " + suffix).
	RuleUp   = "est up 1 size"
	RuleDown = "est down 1 size"
)

// Default scale bounds in minutes of wall time, used until minLearn real items exist.
const defaultS, defaultM, defaultL = 15, 45, 120

var kinds = []string{"cook", "fix", "hotfix"}
var sizeRank = map[string]int{"S": 1, "M": 2, "L": 3, "XL": 4}

// Scale holds the exclusive upper wall-time bounds, in minutes, of S, M and L.
type Scale struct {
	Source string `json:"source"`
	N      int    `json:"n"`
	S      int    `json:"S"`
	M      int    `json:"M"`
	L      int    `json:"L"`
}

// Item is one tracked id.
type Item struct {
	ID        string              `json:"id"`
	Kind      string              `json:"kind"`
	Repo      string              `json:"repo"`
	Est       string              `json:"est"`
	Session   string              `json:"session"`
	Cwd       string              `json:"cwd"`
	Tab       string              `json:"tab"`
	Phase     string              `json:"phase"`
	Status    string              `json:"status"`
	StartedAt string              `json:"startedAt"`
	EndedAt   string              `json:"endedAt"`
	PRs       []string            `json:"prs"`
	AgentMin  int                 `json:"agentMin"`
	WallMin   int                 `json:"wallMin"`
	Wait      WaitMin             `json:"wait"`
	Phases    map[string]PhaseMin `json:"phases"`
	Tokens    map[string]Tokens   `json:"tokens"`
}

type Today struct {
	Date    string         `json:"date"`
	Plans   map[string]int `json:"plans"`
	Started map[string]int `json:"started"`
	// Ended is the sum of Done, Cancelled and RolledBack. Done holds the done and pushed items.
	Ended      map[string]int `json:"ended"`
	Done       map[string]int `json:"done"`
	Cancelled  map[string]int `json:"cancelled"`
	RolledBack map[string]int `json:"rolledBack"`
	// WallMin and AgentMin total the items counted in Done.
	WallMin   int `json:"wallMin"`
	AgentMin  int `json:"agentMin"`
	Tasks     int `json:"tasks"`
	Rounds    int `json:"rounds"`
	PRsOpened int `json:"prsOpened"`
	Merged    int `json:"merged"`
	// PRsWaiting counts tracked PRs (any day) not known as merged or closed (FR-11.7 (5)).
	PRsWaiting  int    `json:"prsWaiting"`
	MergedError string `json:"mergedError"`
	Items       []Item `json:"items"`
	// DailyPlan is today's latest `plan` row with its progress; nil when none was set today.
	DailyPlan *DailyPlan `json:"dailyPlan"`
}

type KindEstimate struct {
	N    int `json:"n"`
	Hit  int `json:"hit"`
	Low  int `json:"low"`
	High int `json:"high"`
}

type HotfixEstimate struct {
	N         int  `json:"n"`
	MedianMin *int `json:"medianMin"`
	P90Min    *int `json:"p90Min"`
}

type Estimate struct {
	ByKind     map[string]KindEstimate       `json:"byKind"`
	Hotfix     HotfixEstimate                `json:"hotfix"`
	PhaseShare map[string]map[string]float64 `json:"phaseShare"`
}

type Findings struct {
	C float64 `json:"C"`
	H float64 `json:"H"`
	M float64 `json:"M"`
	L float64 `json:"L"`
}

type TokensAvg struct {
	In  float64 `json:"in"`
	Out float64 `json:"out"`
}

type KindQuality struct {
	N               int                  `json:"n"`
	ShipBlockedRate *float64             `json:"shipBlockedRate"`
	TopBlockedStep  *string              `json:"topBlockedStep"`
	FindingsAvg     *Findings            `json:"findingsAvg"`
	FixLoopAvg      *float64             `json:"fixLoopAvg"`
	CIRedFirstRate  *float64             `json:"ciRedFirstRate"`
	WaitShare       *float64             `json:"waitShare"`
	TokensAvg       map[string]TokensAvg `json:"tokensAvg"`
}

// FixQuality covers the fix rounds (`fix` log rows) of the fix/hotfix items:
// N is the items with at least one round, RoundAvg the mean of their highest
// round, the rates are over the rows.
type FixQuality struct {
	N              int      `json:"n"`
	RoundAvg       *float64 `json:"roundAvg"`
	WideRate       *float64 `json:"wideRate"`
	RegressionRate *float64 `json:"regressionRate"`
}

type Quality struct {
	ByKind map[string]KindQuality `json:"byKind"`
	Fix    FixQuality             `json:"fix"`
}

// Result is the JSON of `zenify track stats --json`. Enabled is set by the CLI.
type Result struct {
	Enabled   bool     `json:"enabled"`
	Malformed int      `json:"malformed"`
	Scale     Scale    `json:"scale"`
	Rules     []string `json:"rules"`
	Today     Today    `json:"today"`
	Running   []Item   `json:"running"`
	Estimate  Estimate `json:"estimate"`
	Quality   Quality  `json:"quality"`
	Backlog   Backlog  `json:"backlog"`
	Kitchen   Kitchen  `json:"kitchen"`
	Onboarded bool     `json:"onboarded"`
}

// Input is everything Compute reads. Sessions maps a session id to the text of
// its sessions/<id>.log; Cache maps a merged PR url to its mergedAt.
type Input struct {
	Log       string
	Sessions  map[string]string
	PlansRoot string
	Today     string
	Now       time.Time
	Merged    MergedFunc
	Cache     map[string]string
	// GitHub lists open issues; nil skips the source. On a partial failure it
	// returns the items it got together with the error.
	GitHub func() ([]Issue, error)
	// Workspace is the workspace root, used to label a live tab by its cwd.
	Workspace string
	// Live holds the live/<session>.json files with the mtime of their .beat.
	Live []LiveFile
	// Beats maps a session id to the mtime of its live/<session>.beat, for every
	// beat file on disk (a live session may never have written a .json).
	Beats map[string]time.Time
	// PlanTitle returns the title of the plan at path, "" and nil when the file is gone.
	PlanTitle func(path string) (string, error)
	// TabTitle returns the Claude Code tab name of a session, "" and nil when it has none.
	TabTitle func(session string) (string, error)
	// PRTitle returns the title of the pull request at url; nil skips gh this run.
	PRTitle func(url string) (string, error)
	// PRTitles caches PR titles by url; Compute adds the titles it fetches.
	// Pass a non-nil map to keep titles across runs.
	PRTitles map[string]string
	// SDD lists the deferred and parked ledger findings; nil skips the source.
	SDD []SDDItem
}

// LearnScale derives the S/M/L bounds from the 60 most recent real wall
// minutes (oldest first); below minLearn samples it returns the default scale.
func LearnScale(mins []int) Scale {
	mins = append([]int(nil), mins...)
	if len(mins) > maxLearn {
		mins = mins[len(mins)-maxLearn:]
	}
	n := len(mins)
	if n < minLearn {
		return Scale{Source: "default", N: n, S: defaultS, M: defaultM, L: defaultL}
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

func sizeOf(min int, s Scale) string {
	switch {
	case min < s.S:
		return "S"
	case min < s.M:
		return "M"
	case min < s.L:
		return "L"
	}
	return "XL"
}

func isEnded(it Item) bool {
	return it.Status == "done" || it.Status == "pushed" || it.Status == "rollback"
}

// work is the per-id state gathered from log.md.
type work struct {
	item   Item
	events []Event
	end    *Event
	start  time.Time
	from   string // the `start --from` ref, "" when none
}

// Compute builds the stats and returns the new merged cache. Bad data is
// counted in Malformed; a Merged error is reported in Today.MergedError.
func Compute(in Input) (Result, map[string]string) {
	res := Result{Rules: []string{}, Running: []Item{}, Backlog: Backlog{Items: []BacklogItem{}}, Kitchen: Kitchen{Carried: []CarriedItem{}, Cooked: []CookedItem{}, RollingBoil: []BoilItem{}}}
	res.Today = Today{Date: in.Today, Plans: map[string]int{}, Started: map[string]int{}, Ended: map[string]int{}, Done: map[string]int{}, Cancelled: map[string]int{}, RolledBack: map[string]int{}, Items: []Item{}}
	res.Estimate = Estimate{ByKind: map[string]KindEstimate{}, PhaseShare: map[string]map[string]float64{}}
	res.Quality = Quality{ByKind: map[string]KindQuality{}}

	logEv, bad := ParseLog(in.Log)
	res.Malformed += bad

	var sessEv []Event
	sessOf := map[string]string{}
	cwdOf := map[string]string{}
	keys := make([]string, 0, len(in.Sessions))
	for k := range in.Sessions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ev, cwd, m := ParseSession(k, in.Sessions[k])
		res.Malformed += m
		sessEv = append(sessEv, ev...)
		for _, e := range ev {
			if e.Type == "bind" {
				if _, ok := sessOf[e.ID]; !ok {
					sessOf[e.ID], cwdOf[e.ID] = k, cwd
				}
			}
		}
	}

	day := func(t time.Time) string { return t.In(in.Now.Location()).Format(dateLayout) }

	works := map[string]*work{}
	var order []string
	from := map[string]bool{}
	for i := range logEv {
		e := logEv[i]
		switch e.Type {
		case "onboard":
			res.Onboarded = true
			continue
		case "backlog", "drop", "history", "plan":
			continue // backlog and drop are read by computeBacklog; history feeds no stat
		}
		w := works[e.ID]
		if e.Type == "start" {
			if w != nil {
				res.Malformed++ // a repeated start keeps the first
				continue
			}
			w = &work{start: e.At, item: Item{
				ID: e.ID, Kind: e.F[0], Repo: e.F[1], Est: e.F[2], Session: "-", Phase: "-",
				StartedAt: e.At.UTC().Format(time.RFC3339), PRs: []string{},
			}}
			works[e.ID] = w
			order = append(order, e.ID)
			if len(e.F) == 4 && e.F[3] != "-" {
				from[e.F[3]] = true
				w.from = e.F[3]
			}
			continue
		}
		if w == nil {
			res.Malformed++ // a row for an id that never started
			continue
		}
		switch e.Type {
		case "est":
			w.item.Est = e.F[0]
		case "end":
			if !validOutcome(e.F[0]) {
				res.Malformed++ // unknown outcome: the item keeps the status of its other rows
				continue
			}
			if w.end == nil {
				w.end = &logEv[i]
			}
		}
		w.events = append(w.events, e)
	}

	evByID := map[string][]Event{}
	for _, e := range sessEv {
		evByID[e.ID] = append(evByID[e.ID], e)
	}
	var items []Item
	endedPRs := map[string]bool{}
	firstEnd := map[string]time.Time{} // a PR counts as opened on the day of its first end
	var prOrder []string
	for _, id := range order {
		w := works[id]
		it := w.item
		if s, ok := sessOf[id]; ok {
			it.Session, it.Cwd = s, cwdOf[id]
		}
		tm := ComputeTiming(id, it.Kind, in.Now, evByID[id])
		res.Malformed += tm.Malformed
		it.AgentMin, it.Wait, it.Phases, it.Tokens = tm.AgentMin, tm.Wait, tm.Phases, tm.Tokens
		if tm.Phase != "" {
			it.Phase = tm.Phase
		}
		if w.end != nil {
			it.WallMin = wallMin(w.start, w.end.At)
		} else {
			it.WallMin = wallMin(w.start, in.Now)
		}
		switch {
		case w.end == nil && in.Now.Sub(w.start) > staleDays*24*time.Hour:
			it.Status = "stale"
		case w.end == nil:
			it.Status = "running"
		default:
			it.EndedAt = w.end.At.UTC().Format(time.RFC3339)
			out := w.end.F[0]
			switch {
			case out == "cancelled":
				it.Status = "cancelled"
			case out == "rollback":
				it.Status = "rollback"
			case strings.HasPrefix(out, "branch:"):
				it.Status = "pushed"
			default:
				it.Status = "done"
				for _, p := range strings.Split(out, ",") {
					if strings.HasPrefix(p, "http") {
						it.PRs = append(it.PRs, p)
						if !endedPRs[p] {
							endedPRs[p] = true
							prOrder = append(prOrder, p)
						}
					}
				}
			}
			if day(w.end.At) == in.Today {
				res.Today.Ended[it.Kind]++
				switch it.Status {
				case "cancelled":
					res.Today.Cancelled[it.Kind]++
				case "rollback":
					res.Today.RolledBack[it.Kind]++
				default:
					res.Today.Done[it.Kind]++
					res.Today.WallMin += it.WallMin
					res.Today.AgentMin += it.AgentMin
				}
				res.Today.Tasks += atoiDash(w.end.F[1])
				res.Today.Rounds += atoiDash(w.end.F[2])
			}
			for _, p := range it.PRs {
				if at, seen := firstEnd[p]; !seen || w.end.At.Before(at) {
					firstEnd[p] = w.end.At
				}
			}
		}
		if day(w.start) == in.Today {
			res.Today.Started[it.Kind]++
		}
		if day(w.start) == in.Today || (w.end != nil && day(w.end.At) == in.Today) {
			res.Today.Items = append(res.Today.Items, it)
		}
		if it.Status == "running" || it.Status == "stale" {
			res.Running = append(res.Running, it)
		}
		items = append(items, it)
	}

	for _, at := range firstEnd {
		if day(at) == in.Today {
			res.Today.PRsOpened++
		}
	}
	res.Today.Plans = countPlans(in, &res)
	newCache, ghDown := mergedToday(in, prOrder, planPRIDs(logEv, in.Today), &res)

	// Scale: wall minutes of the done and pushed items with a real estimate, oldest end first.
	ended := byEnd(items)
	var mins []int
	for _, it := range ended {
		if (it.Status == "done" || it.Status == "pushed") && sizeRank[it.Est] > 0 && sizeRank[it.Est] < 4 && it.WallMin > 0 {
			mins = append(mins, it.WallMin)
		}
	}
	res.Scale = LearnScale(mins)

	byKindEnded := map[string][]Item{}
	for _, it := range ended {
		if it.WallMin == 0 {
			continue // zero wall time: not an estimate sample
		}
		byKindEnded[it.Kind] = append(byKindEnded[it.Kind], it)
	}
	for _, k := range kinds {
		win := tail(byKindEnded[k], ruleWindow)
		computeEstimate(k, win, res.Scale, &res)
	}
	computeHotfix(in, byKindEnded["hotfix"], &res)

	byStart := map[string][]Item{}
	for _, it := range items {
		if it.Status != "cancelled" {
			byStart[it.Kind] = append(byStart[it.Kind], it)
		}
	}
	computeQuality(byStart, works, &res)
	var issues []Issue
	res.Backlog, issues = computeBacklog(in, logEv, from, items)
	res.Kitchen = computeKitchen(in, logEv, works, order, items, issues, ghDown, newCache, &res)
	return res, newCache
}

// wallMin is the whole minutes from start to end, rounded up; never negative.
func wallMin(start, end time.Time) int {
	d := end.Sub(start)
	if d <= 0 {
		return 0
	}
	return int((d + time.Minute - 1) / time.Minute)
}

func tail(s []Item, n int) []Item {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func validOutcome(out string) bool {
	if out == "cancelled" || out == "rollback" || strings.HasPrefix(out, "branch:") {
		return true
	}
	for _, p := range strings.Split(out, ",") {
		if !strings.HasPrefix(p, "http") {
			return false
		}
	}
	return true
}

func atoiDash(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// byEnd returns the ended, non-cancelled items oldest end first.
func byEnd(items []Item) []Item {
	var out []Item
	for _, it := range items {
		if isEnded(it) {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].EndedAt < out[j].EndedAt })
	return out
}

func countPlans(in Input, res *Result) map[string]int {
	plans := map[string]int{}
	if in.PlansRoot == "" {
		return plans
	}
	matches, err := filepath.Glob(filepath.Join(in.PlansRoot, "*", in.Today+"-*.md"))
	if err != nil {
		res.Malformed++
		return plans
	}
	for _, m := range matches {
		plans[filepath.Base(filepath.Dir(m))]++
	}
	return plans
}

// mergedToday asks Merged about every ended PR not yet known as merged,
// counts the ones merged today and returns the new cache. extra holds the plan PR URLs: they are
// asked about too, but only urls feed PRsWaiting and Merged.
func mergedToday(in Input, urls, extra []string, res *Result) (cache map[string]string, ghDown bool) {
	cache = map[string]string{}
	for k, v := range in.Cache {
		cache[k] = v
	}
	if in.Merged != nil {
		for _, u := range append(append([]string(nil), urls...), extra...) {
			if _, known := cache[u]; known {
				continue
			}
			at, err := in.Merged(u)
			if err != nil {
				// One PR's error (no access, deleted repo) must not hide the PRs after it;
				// only a missing gh fails every PR, so that one stops the loop.
				if res.Today.MergedError == "" {
					res.Today.MergedError = fmt.Sprintf("%s: %v", u, err)
				}
				// A missing gh fails every PR and a timeout means gh or GitHub is hanging:
				// both stop the loop, so N slow PRs never outlast the caller's deadline.
				if errors.Is(err, exec.ErrNotFound) || errors.Is(err, ErrGhTimeout) {
					ghDown = true
					break
				}
				continue
			}
			if at != "" {
				cache[u] = at
			}
		}
	}
	for _, u := range urls {
		at, ok := cache[u]
		if !ok {
			res.Today.PRsWaiting++
		}
		if !ok || at == ClosedUnmerged {
			continue
		}
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			res.Malformed++
			continue
		}
		if t.In(in.Now.Location()).Format(dateLayout) == in.Today {
			res.Today.Merged++
		}
	}
	return cache, ghDown
}

func computeEstimate(kind string, win []Item, scale Scale, res *Result) {
	if len(win) == 0 {
		return
	}
	// phase share over all the window items
	sum := map[string]int{}
	total := 0
	for _, it := range win {
		for p, v := range it.Phases {
			sum[p] += v.AgentMin
			total += v.AgentMin
		}
	}
	if total > 0 {
		share := map[string]float64{}
		for p, v := range sum {
			share[p] = float64(v) / float64(total)
		}
		res.Estimate.PhaseShare[kind] = share
	}
	var ke KindEstimate
	var up, down int
	for _, it := range win {
		if sizeRank[it.Est] == 0 {
			continue
		}
		ke.N++
		actual := sizeRank[sizeOf(it.WallMin, scale)]
		switch est := sizeRank[it.Est]; {
		case est == actual:
			ke.Hit++
		case est < actual:
			ke.Low++
			up++
		default:
			ke.High++
			down++
		}
	}
	if ke.N == 0 {
		return
	}
	res.Estimate.ByKind[kind] = ke
	if ke.N >= minRuleRow {
		if up == ke.N {
			res.Rules = append(res.Rules, kind+": "+RuleUp)
		} else if down == ke.N {
			res.Rules = append(res.Rules, kind+": "+RuleDown)
		}
	}
}

func computeHotfix(in Input, hotfixes []Item, res *Result) {
	t, err := time.Parse(dateLayout, in.Today)
	if err != nil {
		res.Malformed++
		return
	}
	from := t.AddDate(0, 0, -6).Format(dateLayout)
	var mins []int
	for _, it := range hotfixes {
		et, err := time.Parse(time.RFC3339, it.EndedAt)
		if err != nil {
			continue
		}
		if d := et.In(in.Now.Location()).Format(dateLayout); d >= from && d <= in.Today {
			mins = append(mins, it.WallMin)
		}
	}
	h := HotfixEstimate{N: len(mins)}
	if len(mins) > 0 {
		sort.Ints(mins)
		med, p90 := nearestRank(mins, 50), nearestRank(mins, 90)
		h.MedianMin, h.P90Min = &med, &p90
	}
	res.Estimate.Hotfix = h
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}

func computeQuality(byStart map[string][]Item, works map[string]*work, res *Result) {
	var fixRows, wide, test, fixIDs, roundSum int
	for _, k := range kinds {
		win := tail(byStart[k], ruleWindow)
		if len(win) == 0 {
			continue
		}
		q := KindQuality{N: len(win), TokensAvg: map[string]TokensAvg{}}
		var ships, blocked, ciIDs, ciRed, finN, loopN, loopSum int
		var fin [4]int
		steps := map[string]int{}
		var agent, wait int
		tokSum := map[string]TokensAvg{}
		tokN := map[string]int{}
		for _, it := range win {
			agent += it.AgentMin
			wait += it.Wait.Total
			for m, t := range it.Tokens {
				s := tokSum[m]
				s.In += float64(t.In)
				s.Out += float64(t.Out)
				tokSum[m] = s
				tokN[m]++
			}
			firstCI := ""
			maxRound := 0
			for _, e := range works[it.ID].events {
				switch e.Type {
				case "ship":
					ships++
					if e.F[0] == "blocked" {
						blocked++
						if e.F[1] != "-" {
							steps[e.F[1]]++
						}
					}
					if f, ok := parseFindings(e.F[2]); ok {
						finN++
						for i := range fin {
							fin[i] += f[i]
						}
					} else if e.F[2] != "-" {
						res.Malformed++
					}
					if e.F[3] != "-" {
						loopN++
						loopSum += atoiDash(e.F[3])
					}
				case "ci":
					if firstCI == "" {
						firstCI = e.F[0]
					}
				case "fix":
					if k == "cook" {
						continue
					}
					fixRows++
					if e.F[1] == "wide" {
						wide++
					}
					if e.F[2] == "test" {
						test++
					}
					if r := atoiDash(e.F[0]); r > maxRound {
						maxRound = r
					}
				}
			}
			if firstCI != "" {
				ciIDs++
				if firstCI == "red" {
					ciRed++
				}
			}
			if maxRound > 0 {
				fixIDs++
				roundSum += maxRound
			}
		}
		q.ShipBlockedRate = ratio(blocked, ships)
		q.CIRedFirstRate = ratio(ciRed, ciIDs)
		q.WaitShare = ratio(wait, agent+wait)
		if loopN > 0 {
			v := float64(loopSum) / float64(loopN)
			q.FixLoopAvg = &v
		}
		if finN > 0 {
			d := float64(finN)
			q.FindingsAvg = &Findings{C: float64(fin[0]) / d, H: float64(fin[1]) / d, M: float64(fin[2]) / d, L: float64(fin[3]) / d}
		}
		top, topN := "", 0
		for s, n := range steps {
			if n > topN || (n == topN && s < top) {
				top, topN = s, n
			}
		}
		if topN > 0 {
			q.TopBlockedStep = &top
		}
		for m, s := range tokSum {
			n := float64(tokN[m])
			q.TokensAvg[m] = TokensAvg{In: s.In / n, Out: s.Out / n}
		}
		res.Quality.ByKind[k] = q
	}
	res.Quality.Fix = FixQuality{N: fixIDs, WideRate: ratio(wide, fixRows), RegressionRate: ratio(test, fixRows)}
	if fixIDs > 0 {
		v := float64(roundSum) / float64(fixIDs)
		res.Quality.Fix.RoundAvg = &v
	}
}

func parseFindings(v string) ([4]int, bool) {
	var out [4]int
	p := strings.Split(v, ",")
	if len(p) != 4 {
		return out, false
	}
	for i, s := range p {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
