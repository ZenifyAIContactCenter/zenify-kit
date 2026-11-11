package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

// resolveScope picks which stats items a short code may name.
type resolveScope int

const (
	scopeBacklog   resolveScope = iota // open backlog items
	scopePlanAll                       // today's plan, open backlog and running items
	scopePlanToday                     // today's plan only
)

// resolveRef turns one short code into a full id; see resolveRefs.
func resolveRef(dir, wsDir, ref string, scope resolveScope, now time.Time) (string, error) {
	ids, err := resolveRefs(dir, wsDir, []string{ref}, scope, now)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// resolveRefs turns short codes into full ids from the items the pane shows, reading the stats
// once for all of them. A full id is returned as is; when every ref is a full id no stats are read.
func resolveRefs(dir, wsDir string, refs []string, scope resolveScope, now time.Time) ([]string, error) {
	ids := make([]string, len(refs))
	var cands []trackstats.Candidate
	loaded := false
	for i, ref := range refs {
		if trackstats.IsFullID(ref) {
			ids[i] = ref
			continue
		}
		if !loaded {
			var err error
			if cands, err = trackCandidates(dir, wsDir, scope, now); err != nil {
				return nil, err
			}
			loaded = true
		}
		id, err := resolveFrom(ref, cands)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

func resolveFrom(ref string, cands []trackstats.Candidate) (string, error) {
	c, many, err := trackstats.Resolve(ref, cands)
	switch {
	case errors.Is(err, trackstats.ErrNoMatch):
		return "", badTrack("track: no item matches %s", ref)
	case errors.Is(err, trackstats.ErrAmbiguous):
		var b strings.Builder
		fmt.Fprintf(&b, "track: %s matches %d items:\n", ref, len(many))
		for _, m := range many {
			fmt.Fprintf(&b, "  %s · %s · %s\n", stripCtl(m.Code), stripCtl(m.Repo), stripCtl(m.Title))
		}
		b.WriteString("use <repo>#<n> or the full id")
		return "", badTrack("%s", b.String())
	case err != nil:
		return "", err
	}
	return c.ID, nil
}

// stripCtl drops control characters so a third-party title cannot inject terminal escapes.
func stripCtl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// trackCandidates lists the items of scope from the same stats the pane shows.
func trackCandidates(dir, wsDir string, scope resolveScope, now time.Time) ([]trackstats.Candidate, error) {
	res, err := computeTrackStats(dir, wsDir, now.Local().Format("2006-01-02"), now, false)
	if err != nil {
		return nil, err
	}
	var cands []trackstats.Candidate
	add := func(id, code, repo, title string) {
		if repo == "" || repo == "-" {
			repo = trackstats.URLRepo(id)
		}
		cands = append(cands, trackstats.Candidate{ID: id, Code: code, Repo: repo, Title: title})
	}
	if scope != scopeBacklog && res.Today.DailyPlan != nil {
		for _, it := range res.Today.DailyPlan.Items {
			add(it.ID, it.Code, it.Repo, it.Title)
		}
	}
	if scope != scopePlanToday {
		for _, it := range res.Backlog.Items {
			add(it.ID, it.Code, it.Repo, it.Title)
		}
	}
	if scope == scopePlanAll {
		for _, it := range res.Running {
			add(it.ID, trackstats.Code(it.ID), it.Repo, it.Kind+" "+it.Repo)
		}
	}
	return cands, nil
}
