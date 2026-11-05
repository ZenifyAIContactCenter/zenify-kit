package trackstats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// liveTTL is how long a live file stays alive after the mtime of its .beat.
const liveTTL = 30 * time.Second

// LiveTTL is liveTTL for callers that skip reading an expired live file.
const LiveTTL = liveTTL

// maxTitleFetch caps the gh PR-title calls of one Compute run; the items left over show
// "<kind> <repo>" this run and are fetched in a later window.
const maxTitleFetch = 3

// A failed title fetch is cached as titleFailedPrefix + the RFC3339 time of the failure and is
// retried once it is titleRetry old. It is never shown as a brief.
const (
	titleFailedPrefix = "!failed "
	titleRetry        = time.Hour
)

const planSuffix = " — Implementation Plan"

// LiveFile is one live/<session>.json with the mtime of its .beat; a zero
// BeatAt means the .beat file does not exist.
type LiveFile struct {
	Session, JSON string
	BeatAt        time.Time
}

type CookedItem struct {
	ID      string   `json:"id"`
	Brief   string   `json:"brief"`
	Status  string   `json:"status"`
	EndedAt string   `json:"endedAt"`
	PRs     []string `json:"prs"`
	// Merged: the item has PRs and every one of them is merged (closed unmerged or unknown is not).
	Merged  bool   `json:"merged"`
	Est     string `json:"est"`
	WallMin int    `json:"wallMin"`
}

type BoilItem struct {
	Session string `json:"session"`
	Tab     string `json:"tab"`
	Brief   string `json:"brief"`
	State   string `json:"state"`
	Since   string `json:"since"`
}

// CookingItem is a live tab that is working right now (state busy).
type CookingItem struct {
	Session string `json:"session"`
	Tab     string `json:"tab"`
	Brief   string `json:"brief"`
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Phase   string `json:"phase"`
	Since   string `json:"since"`
}

// CarriedItem is an item started before today that has no end yet.
type CarriedItem struct {
	ID        string `json:"id"`
	Brief     string `json:"brief"`
	Kind      string `json:"kind"`
	Repo      string `json:"repo"`
	Phase     string `json:"phase"`
	StartedAt string `json:"startedAt"`
	WallMin   int    `json:"wallMin"`
	Stale     bool   `json:"stale"`
	Worktree  string `json:"worktree"`
	Tab       string `json:"tab"`
	Session   string `json:"session"`
}

// LastDay summarises the latest working day before today.
type LastDay struct {
	Date       string `json:"date"`
	Done       int    `json:"done"`
	Cancelled  int    `json:"cancelled"`
	RolledBack int    `json:"rolledBack"`
	PRsOpened  int    `json:"prsOpened"`
	LeftOpen   int    `json:"leftOpen"`
}

// Kitchen is what was cooked today and which tabs wait for a person.
type Kitchen struct {
	Carried     []CarriedItem `json:"carried"`
	LastDay     *LastDay      `json:"lastDay"`
	Cooked      []CookedItem  `json:"cooked"`
	RollingBoil []BoilItem    `json:"rollingBoil"`
	Cooking     []CookingItem `json:"cooking"`
	TitleError  string        `json:"titleError"`
}

type liveDoc struct {
	State string `json:"state"`
	Since string `json:"since"`
	Cwd   string `json:"cwd"`
	ID    string `json:"id"`
}

// computeKitchen lists the items ended today (newest first), the live tabs
// in state ask or permission (longest waiting first) and those in state busy
// (longest working first). It reads files and gh
// only through the Input funcs; the first title error is kept in TitleError.
// It also fills res.Running[i].Tab, besides building Carried and LastDay.
func computeKitchen(in Input, logEv []Event, works map[string]*work, order []string, items []Item, issues []Issue, ghDown bool, mergedAt map[string]string, res *Result) Kitchen {
	k := Kitchen{Carried: []CarriedItem{}, Cooked: []CookedItem{}, RollingBoil: []BoilItem{}, Cooking: []CookingItem{}}
	backlogTitle := map[string]string{}
	for _, e := range logEv {
		if e.Type == "backlog" {
			backlogTitle[e.ID] = e.F[3]
		}
	}
	for _, is := range issues {
		if _, ok := backlogTitle[is.URL]; !ok {
			backlogTitle[is.URL] = is.Title
		}
	}
	prTitles := in.PRTitles
	if prTitles == nil {
		prTitles = map[string]string{}
	}
	fail := func(err error) {
		if k.TitleError == "" {
			k.TitleError = err.Error()
		}
	}
	tabs := map[string]string{}
	tabOf := func(session string) string {
		if in.TabTitle == nil || session == "-" {
			return ""
		}
		if t, ok := tabs[session]; ok {
			return t
		}
		t, err := in.TabTitle(session)
		if err != nil {
			fail(fmt.Errorf("tab title %s: %w", session, err))
		}
		tabs[session] = t
		return t
	}
	// An item's tab is read only while its session still beats: a closed tab has no name to show,
	// and each lookup reads up to 1 MiB of transcript.
	liveTab := func(session string) string {
		if b, ok := in.Beats[session]; !ok || in.Now.Sub(b) > liveTTL {
			return ""
		}
		return tabOf(session)
	}
	for i := range res.Running {
		res.Running[i].Tab = liveTab(res.Running[i].Session)
	}
	itemOf := map[string]Item{}
	for _, it := range items {
		itemOf[it.ID] = it
	}
	briefs := map[string]string{}
	fetched := 0
	// brief follows plan, backlog, PR, then "<kind> <repo>"; askPR lets it call gh.
	brief := func(it Item, askPR bool) string {
		if b, ok := briefs[it.ID]; ok {
			return b
		}
		w := works[it.ID]
		var b string
		if w.end != nil && len(w.end.F) > 3 && in.PlanTitle != nil {
			title, err := in.PlanTitle(w.end.F[3])
			if err != nil {
				fail(fmt.Errorf("plan title %s: %w", w.end.F[3], err))
			}
			b = strings.TrimSpace(strings.TrimSuffix(title, planSuffix))
		}
		if b == "" && w.from != "" {
			b = backlogTitle[w.from]
		}
		if b == "" && len(it.PRs) > 0 {
			u := it.PRs[0]
			t, known := prTitles[u]
			if failed, ok := strings.CutPrefix(t, titleFailedPrefix); ok {
				at, perr := time.Parse(time.RFC3339, failed)
				if perr != nil || in.Now.Sub(at) >= titleRetry {
					known = false
				}
				t = ""
			}
			b = t
			if !known && askPR && in.PRTitle != nil && !ghDown && fetched < maxTitleFetch {
				fetched++
				title, err := in.PRTitle(u)
				switch {
				case err == nil:
					b, prTitles[u] = title, title
				case errors.Is(err, ErrGhTimeout) || errors.Is(err, exec.ErrNotFound):
					// gh is hanging or missing: stop asking this run and do not cache the URL.
					ghDown = true
					fail(fmt.Errorf("pr title %s: %w", u, err))
				default:
					// A PR that cannot be read (no access, deleted, a gh blip) is cached as failed, so
					// it stops using up the fetch cap of the PRs behind it until titleRetry passes.
					prTitles[u] = titleFailedPrefix + in.Now.UTC().Format(time.RFC3339)
					fail(fmt.Errorf("pr title %s: %w", u, err))
				}
			}
		}
		if b == "" {
			b = it.Kind + " " + it.Repo
		}
		if askPR {
			briefs[it.ID] = b
		}
		return b
	}

	type cooked struct {
		at   time.Time
		item CookedItem
	}
	var done []cooked
	for _, id := range order {
		w := works[id]
		it := itemOf[id]
		if w.end == nil || it.Status == "running" || it.Status == "stale" || w.end.At.In(in.Now.Location()).Format(dateLayout) != in.Today {
			continue
		}
		done = append(done, cooked{w.end.At, CookedItem{ID: id, Brief: brief(it, true), Status: it.Status, EndedAt: it.EndedAt, PRs: it.PRs, Merged: allMerged(it.PRs, mergedAt), Est: it.Est, WallMin: it.WallMin}})
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].at.After(done[j].at) })
	for _, c := range done {
		k.Cooked = append(k.Cooked, c.item)
	}

	type boil struct {
		at   time.Time
		item BoilItem
	}
	type cook struct {
		at   time.Time
		item CookingItem
	}
	var waiting []boil
	var cooking []cook
	for _, lf := range in.Live {
		if lf.BeatAt.IsZero() || in.Now.Sub(lf.BeatAt) > liveTTL {
			continue
		}
		var d liveDoc
		if err := json.Unmarshal([]byte(lf.JSON), &d); err != nil {
			res.Malformed++
			continue
		}
		since, perr := time.Parse(time.RFC3339, d.Since)
		if perr != nil || (d.State != "ask" && d.State != "permission" && d.State != "busy" && d.State != "idle") {
			res.Malformed++
			continue
		}
		if d.State == "idle" {
			continue
		}
		var b string
		it, bound := itemOf[d.ID]
		if bound {
			b = brief(it, false)
		} else {
			b = cwdLabel(in.Workspace, d.Cwd)
		}
		if d.State == "busy" {
			cooking = append(cooking, cook{since, CookingItem{Session: lf.Session, Tab: tabOf(lf.Session), Brief: b, ID: it.ID, Kind: it.Kind, Phase: none(it.Phase), Since: d.Since}})
			continue
		}
		waiting = append(waiting, boil{since, BoilItem{Session: lf.Session, Tab: tabOf(lf.Session), Brief: b, State: d.State, Since: d.Since}})
	}
	sort.SliceStable(waiting, func(i, j int) bool { return waiting[i].at.Before(waiting[j].at) })
	for _, w := range waiting {
		k.RollingBoil = append(k.RollingBoil, w.item)
	}
	sort.SliceStable(cooking, func(i, j int) bool { return cooking[i].at.Before(cooking[j].at) })
	for _, c := range cooking {
		k.Cooking = append(k.Cooking, c.item)
	}
	k.Carried = carriedItems(in, works, order, itemOf, brief, liveTab)
	k.LastDay = lastDay(in, works, order, itemOf)
	return k
}

// none turns the log's "-" (no phase yet, no bound session) into "", the JSON's one "absent" value.
func none(v string) string {
	if v == "-" {
		return ""
	}
	return v
}

// carriedItems lists the items started before today with no end, oldest start first.
func carriedItems(in Input, works map[string]*work, order []string, itemOf map[string]Item, brief func(Item, bool) string, tabOf func(string) string) []CarriedItem {
	type carried struct {
		at   time.Time
		item CarriedItem
	}
	var found []carried
	for _, id := range order {
		w := works[id]
		if w.end != nil || w.start.In(in.Now.Location()).Format(dateLayout) >= in.Today {
			continue
		}
		it := itemOf[id]
		found = append(found, carried{w.start, CarriedItem{
			ID: id, Brief: brief(it, false), Kind: it.Kind, Repo: it.Repo, Phase: none(it.Phase),
			StartedAt: it.StartedAt, WallMin: it.WallMin, Stale: it.Status == "stale",
			Worktree: worktreeLabel(in.Workspace, it.Cwd), Tab: tabOf(it.Session), Session: none(it.Session),
		}})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].at.Before(found[j].at) })
	out := make([]CarriedItem, 0, len(found))
	for _, c := range found {
		out = append(out, c.item)
	}
	return out
}

// lastDay summarises the latest local day before today with a start or end row, nil when none.
func lastDay(in Input, works map[string]*work, order []string, itemOf map[string]Item) *LastDay {
	day := func(t time.Time) string { return t.In(in.Now.Location()).Format(dateLayout) }
	date := ""
	for _, id := range order {
		w := works[id]
		days := []string{day(w.start)}
		if w.end != nil {
			days = append(days, day(w.end.At))
		}
		for _, d := range days {
			if d < in.Today && d > date {
				date = d
			}
		}
	}
	if date == "" {
		return nil
	}
	ld := &LastDay{Date: date}
	firstEnd := map[string]time.Time{}
	for _, id := range order {
		w := works[id]
		if w.end == nil || day(w.end.At) > date {
			if day(w.start) <= date {
				ld.LeftOpen++
			}
			continue
		}
		it := itemOf[id]
		for _, p := range it.PRs {
			if at, seen := firstEnd[p]; !seen || w.end.At.Before(at) {
				firstEnd[p] = w.end.At
			}
		}
		if day(w.end.At) != date {
			continue
		}
		switch it.Status {
		case "cancelled":
			ld.Cancelled++
		case "rollback":
			ld.RolledBack++
		default:
			ld.Done++
		}
	}
	for _, at := range firstEnd {
		if day(at) == date {
			ld.PRsOpened++
		}
	}
	return ld
}

// worktreeLabel is "<repo>/<slug>" for a cwd under <ws>/repos/<repo>/.worktrees/<slug>, otherwise "".
func worktreeLabel(ws, cwd string) string {
	if ws == "" || cwd == "" {
		return ""
	}
	rel, err := filepath.Rel(filepath.Join(ws, "repos"), cwd)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 3 && parts[1] == ".worktrees" {
		return parts[0] + "/" + parts[2]
	}
	return ""
}

// cwdLabel is "<repo>/<slug>" under <ws>/repos/<repo>/.worktrees/<slug>,
// "<repo>" under <ws>/repos/<repo>, otherwise the last directory name.
func cwdLabel(ws, cwd string) string {
	if wt := worktreeLabel(ws, cwd); wt != "" {
		return wt
	}
	if ws != "" {
		if rel, err := filepath.Rel(filepath.Join(ws, "repos"), cwd); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return strings.Split(filepath.ToSlash(rel), "/")[0]
		}
	}
	return filepath.Base(cwd)
}

// allMerged reports whether prs is non-empty and every PR has a merge time in mergedAt.
func allMerged(prs []string, mergedAt map[string]string) bool {
	if len(prs) == 0 {
		return false
	}
	for _, u := range prs {
		if at, ok := mergedAt[u]; !ok || at == ClosedUnmerged {
			return false
		}
	}
	return true
}
