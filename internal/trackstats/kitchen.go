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
}

type BoilItem struct {
	Session string `json:"session"`
	Brief   string `json:"brief"`
	State   string `json:"state"`
	Since   string `json:"since"`
}

// Kitchen is what was cooked today and which tabs wait for a person.
type Kitchen struct {
	Cooked      []CookedItem `json:"cooked"`
	RollingBoil []BoilItem   `json:"rollingBoil"`
	TitleError  string       `json:"titleError"`
}

type liveDoc struct {
	State string `json:"state"`
	Since string `json:"since"`
	Cwd   string `json:"cwd"`
	ID    string `json:"id"`
}

// computeKitchen lists the items ended today (newest first) and the live tabs
// in state ask or permission (longest waiting first). It reads files and gh
// only through the Input funcs; the first title error is kept in TitleError.
func computeKitchen(in Input, logEv []Event, works map[string]*work, order []string, items []Item, issues []Issue, ghDown bool, res *Result) Kitchen {
	k := Kitchen{Cooked: []CookedItem{}, RollingBoil: []BoilItem{}}
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
					// A PR that cannot be read (no access, deleted) is cached as empty, so it
					// stops using up the fetch cap of the PRs behind it.
					prTitles[u] = ""
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
		done = append(done, cooked{w.end.At, CookedItem{ID: id, Brief: brief(it, true), Status: it.Status, EndedAt: it.EndedAt, PRs: it.PRs}})
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].at.After(done[j].at) })
	for _, c := range done {
		k.Cooked = append(k.Cooked, c.item)
	}

	type boil struct {
		at   time.Time
		item BoilItem
	}
	var waiting []boil
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
		if perr != nil || (d.State != "ask" && d.State != "permission" && d.State != "busy") {
			res.Malformed++
			continue
		}
		if d.State == "busy" {
			continue
		}
		var b string
		if it, ok := itemOf[d.ID]; ok && d.ID != "" {
			b = brief(it, false)
		} else {
			b = cwdLabel(in.Workspace, d.Cwd)
		}
		waiting = append(waiting, boil{since, BoilItem{Session: lf.Session, Brief: b, State: d.State, Since: d.Since}})
	}
	sort.SliceStable(waiting, func(i, j int) bool { return waiting[i].at.Before(waiting[j].at) })
	for _, w := range waiting {
		k.RollingBoil = append(k.RollingBoil, w.item)
	}
	return k
}

// cwdLabel is "<repo>/<slug>" under <ws>/repos/<repo>/.worktrees/<slug>,
// "<repo>" under <ws>/repos/<repo>, otherwise the last directory name.
func cwdLabel(ws, cwd string) string {
	if ws != "" {
		if rel, err := filepath.Rel(filepath.Join(ws, "repos"), cwd); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) >= 3 && parts[1] == ".worktrees" {
				return parts[0] + "/" + parts[2]
			}
			return parts[0]
		}
	}
	return filepath.Base(cwd)
}
