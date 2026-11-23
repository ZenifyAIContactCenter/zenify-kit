package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

// planeProjectWorkers bounds the projects fetched at once.
const planeProjectWorkers = 4

// trackPlaneClient sends the backlog's Plane calls; a test points it at an httptest server.
// Like planeHTTP it never follows a redirect, so X-API-Key cannot reach another host.
var trackPlaneClient = &http.Client{CheckRedirect: planeHTTP.CheckRedirect}

// planeOpenGroups are the state groups of a work item still to be done.
var planeOpenGroups = []string{"backlog", "unstarted", "started"}

// trackPlaneSource returns the fetch of the developer's open Plane work items,
// or nil when the manifest has no Plane or no PAT is set: the source is then
// skipped without an error. A manifest or .env that cannot be read is an error.
func trackPlaneSource(dir, wsDir string, now time.Time) (func() ([]trackstats.PlaneItem, error), error) {
	c, err := planeConnect(wsDir)
	if err != nil || c == nil {
		return nil, err
	}
	return func() ([]trackstats.PlaneItem, error) {
		ctx, cancel := context.WithTimeout(context.Background(), trackGhTimeout)
		defer cancel()
		states, serr := loadPlaneStates(dir, now) // unreadable: refetched below and overwritten, but reported once
		items, err := fetchPlaneMine(c.api(ctx), c.ws, c.browse, states)
		err = errors.Join(err, serr, states.save()) // save even after a 429: the states it fetched are not lost
		if ctx.Err() != nil {
			// Keep the cause: a 429 seen before the deadline must still set a rate-limit backoff.
			return items, errors.Join(fmt.Errorf("timed out after %s", trackGhTimeout), err)
		}
		return items, err
	}, nil
}

// planeConn is the manifest's Plane plus the developer's PAT.
type planeConn struct {
	browse, ws, pat string
}

func (c *planeConn) api(ctx context.Context) planeAPI {
	return planeAPI{ctx: ctx, client: trackPlaneClient, base: c.browse + "/api/v1", pat: c.pat}
}

// planeConnect returns nil when the manifest has no Plane or no PAT is set.
// A manifest or .env that cannot be read is an error.
func planeConnect(wsDir string) (*planeConn, error) {
	man, err := brokerManifest("", "")
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if man.Plane == nil {
		return nil, nil
	}
	pat, err := planePAT(os.Getenv, wsDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", planePATKey, err)
	}
	if pat == "" {
		return nil, nil
	}
	return &planeConn{browse: strings.TrimRight(man.Plane.URL, "/"), ws: man.Plane.Workspace, pat: pat}, nil
}

type planePage[T any] struct {
	Results    []T    `json:"results"`
	Next       bool   `json:"next_page_results"`
	NextCursor string `json:"next_cursor"`
}

// planeList reads every page of a Plane list endpoint. path has no query yet.
func planeList[T any](p planeAPI, path string) ([]T, error) {
	var all []T
	cursor := ""
	for {
		q := url.Values{"per_page": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page planePage[T]
		if err := p.do(http.MethodGet, path+"?"+q.Encode(), nil, &page); err != nil {
			return all, err
		}
		all = append(all, page.Results...)
		if !page.Next || page.NextCursor == "" || page.NextCursor == cursor {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// fetchPlaneMine lists the work items of every project in workspace ws that
// are assigned to the PAT's owner and still open, skipping projects the owner
// is not a member of. browse is the Plane web root
// the item URLs are built on. A failed project is named in the error and the
// items of the other projects are still returned.
func fetchPlaneMine(p planeAPI, ws, browse string, states *planeStatesCache) ([]trackstats.PlaneItem, error) {
	var me struct {
		ID string `json:"id"`
	}
	if err := p.do(http.MethodGet, "/users/me/", nil, &me); err != nil {
		return nil, err
	}
	if me.ID == "" {
		return nil, errors.New("/users/me/ returned no id")
	}
	wsPath := "/workspaces/" + url.PathEscape(ws) + "/projects/"
	projects, err := planeList[struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
		IsMember   *bool  `json:"is_member"`
	}](p, wsPath)
	if err != nil {
		return nil, err
	}
	// Projects run in parallel (bounded) so a workspace with many of them fits the one deadline;
	// results keep the project order.
	type result struct {
		items []trackstats.PlaneItem
		err   error
	}
	res := make([]result, len(projects))
	sem := make(chan struct{}, planeProjectWorkers)
	var wg sync.WaitGroup
	var limited atomic.Bool // a 429 stops the projects not yet started: more requests only deepen the limit
	for i, pr := range projects {
		if pr.IsMember != nil && !*pr.IsMember {
			continue // a public project the developer is not in answers 403 and holds none of their items
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if limited.Load() {
				return // the error of the 429 itself is reported
			}
			got, err := planeProjectMine(p, wsPath+url.PathEscape(pr.ID)+"/", pr.ID, pr.Identifier, me.ID, ws, browse, states)
			if rl, _ := planeRateLimit(err); rl {
				limited.Store(true)
			}
			res[i] = result{got, err}
		}()
	}
	wg.Wait()
	var items []trackstats.PlaneItem
	var errs []string
	var causes []error
	for i, r := range res {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("project %s: %v", projects[i].Identifier, r.err))
			causes = append(causes, r.err)
		}
		items = append(items, r.items...)
	}
	if len(errs) > 0 {
		return items, &multiErr{msg: strings.Join(errs, "; "), errs: causes}
	}
	return items, nil
}

func planeProjectMine(p planeAPI, proj, projID, ident, me, ws, browse string, cache *planeStatesCache) ([]trackstats.PlaneItem, error) {
	openIDs, ok := cache.get(projID)
	if !ok {
		states, err := planeList[planeState](p, proj+"states/")
		if err != nil {
			return nil, err
		}
		openIDs = []string{}
		for _, s := range states {
			if slices.Contains(planeOpenGroups, s.Group) {
				openIDs = append(openIDs, s.ID)
			}
		}
		cache.put(projID, openIDs)
	}
	open := map[string]bool{}
	for _, id := range openIDs {
		open[id] = true
	}
	wis, err := planeList[struct {
		SequenceID int       `json:"sequence_id"`
		Name       string    `json:"name"`
		State      string    `json:"state"`
		Assignees  []string  `json:"assignees"`
		CreatedAt  time.Time `json:"created_at"`
	}](p, proj+"work-items/")
	var items []trackstats.PlaneItem
	for _, w := range wis {
		if !open[w.State] || !slices.Contains(w.Assignees, me) {
			continue
		}
		key := ident + "-" + strconv.Itoa(w.SequenceID)
		items = append(items, trackstats.PlaneItem{Key: key, URL: browse + "/" + ws + "/browse/" + key + "/", Title: w.Name, CreatedAt: w.CreatedAt})
	}
	return items, err
}
