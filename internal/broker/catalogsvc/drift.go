package catalogsvc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/perlogin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
)

const (
	driftWindow = 7 * 24 * time.Hour
	// tempoWindow: Tempo refuses a tag-values range over 72h (staging returned
	// "exceeds 72h0m0s"), and keeps about 2 days anyway.
	tempoWindow  = 72 * time.Hour
	maxDriftBody = 4 << 10
	tempoTag     = "resource.service.name"
	promLabel    = "job"
)

// Drift is the POST /catalog/drift handler: it compares the catalog with the
// label values Grafana saw in the last 7 days (Tempo: 72 hours). It must sit behind
// auth.RequireAudited.
type Drift struct {
	h     *Handler
	g     *grafana
	now   func() time.Time
	audit *audit.Logger // nil = no audit rows

	maxConcurrent int // per login
	slots         perlogin.Slots
}

// NewDrift builds the drift handler over h's catalog. Of the Options, only
// WithClock, WithAudit and WithSlots apply; the Grafana base URL is grafanaURL.
func NewDrift(h *Handler, grafanaURL, grafanaToken string, opts ...Option) *Drift {
	o := &Handler{now: time.Now, slots: 1}
	for _, opt := range opts {
		opt(o)
	}
	return &Drift{h: h, g: newGrafana(grafanaURL, grafanaToken), now: o.now, audit: o.audit, maxConcurrent: o.slots}
}

func (d *Drift) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := d.now()
	status := http.StatusOK
	var req api.DriftRequest
	defer func() {
		if d.audit != nil {
			d.audit.Log(audit.Entry{TS: start, Login: auth.Login(r.Context()), Role: auth.Role(r.Context()),
				Endpoint: "/catalog/drift", Env: req.Env, Op: r.Method, Status: status})
		}
	}()
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDriftBody)).Decode(&req); err != nil {
		status = http.StatusBadRequest
		auth.WriteError(w, status, api.CodeInvalidBody, `body must be {"env":"prd"|"stg"|""}`)
		return
	}
	envs := catalog.Envs
	switch req.Env {
	case "":
	case catalog.Envs[0], catalog.Envs[1]:
		envs = []string{req.Env}
	default:
		status = http.StatusBadRequest
		auth.WriteError(w, status, api.CodeInvalidBody, "env must be prd, stg or empty")
		return
	}
	// Each drift fans out one Grafana label-values query per env x source.
	login := auth.Login(r.Context())
	if !d.slots.Acquire(login, d.maxConcurrent) {
		status = http.StatusTooManyRequests
		auth.WriteError(w, status, api.CodeTooManyConcurrent, "a catalog drift for this login is already running")
		return
	}
	defer d.slots.Release(login)
	c, _, err := d.h.Current(r.Context())
	if err != nil {
		status = http.StatusServiceUnavailable
		auth.WriteError(w, status, api.CodeCatalogUnavailable, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.DriftResponse{Envs: d.compare(r.Context(), c, envs, start)})
}

// source is one kind of live name: how to read it and where the catalog keeps it.
type source struct {
	name    string
	listed  func(*catalog.EnvNames) []string
	ignored func(catalog.Names) []string
	live    func(context.Context, *grafana, *catalog.Catalog, catalog.Datasources, time.Time, time.Time) ([]string, error)
	window  time.Duration
}

var sources = []source{
	{"loki",
		func(n *catalog.EnvNames) []string { return n.Loki },
		func(n catalog.Names) []string { return n.Loki },
		func(ctx context.Context, g *grafana, c *catalog.Catalog, ds catalog.Datasources, s, e time.Time) ([]string, error) {
			if ds.Loki == "" {
				return nil, errors.New("no loki datasource uid for this env")
			}
			if c.Labels.LokiApp == "" {
				return nil, errors.New("labels.loki_app is not set")
			}
			return g.lokiLabelValues(ctx, ds.Loki, c.Labels.LokiApp, s, e)
		}, driftWindow},
	{"tempo",
		func(n *catalog.EnvNames) []string { return n.Tempo },
		func(n catalog.Names) []string { return n.Tempo },
		func(ctx context.Context, g *grafana, _ *catalog.Catalog, ds catalog.Datasources, s, e time.Time) ([]string, error) {
			if ds.Tempo == "" {
				return nil, errors.New("no tempo datasource uid for this env")
			}
			return g.tempoTagValues(ctx, ds.Tempo, tempoTag, s, e)
		}, tempoWindow},
	{"prometheus",
		func(n *catalog.EnvNames) []string { return n.PrometheusJob },
		func(n catalog.Names) []string { return n.PrometheusJob },
		func(ctx context.Context, g *grafana, _ *catalog.Catalog, ds catalog.Datasources, s, e time.Time) ([]string, error) {
			if ds.Prometheus == "" {
				return nil, errors.New("no prometheus datasource uid for this env")
			}
			return g.promLabelValues(ctx, ds.Prometheus, promLabel, s, e)
		}, driftWindow},
}

// compare asks every env x source in parallel; one source failing sets only
// its own Error.
func (d *Drift) compare(ctx context.Context, c *catalog.Catalog, envs []string, end time.Time) map[string]map[string]api.DriftResult {
	out := map[string]map[string]api.DriftResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, env := range envs {
		// Goroutines write only m, under mu; out itself is written here only.
		m := map[string]api.DriftResult{}
		out[env] = m
		for _, src := range sources {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := api.DriftResult{Unlisted: []string{}, Missing: []string{}}
				live, err := src.live(ctx, d.g, c, c.Datasources[env], end.Add(-src.window), end)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.Unlisted, res.Missing = diff(live, listedNames(c, env, src), src.ignored(c.Ignore[env]))
				}
				mu.Lock()
				m[src.name] = res
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return out
}

func listedNames(c *catalog.Catalog, env string, src source) []string {
	var names []string
	for _, e := range append(append([]catalog.Entry(nil), c.Services...), c.Datastores...) {
		n := e.Prd
		if env == "stg" {
			n = e.Stg
		}
		if n != nil {
			names = append(names, src.listed(n)...)
		}
	}
	return names
}

// diff: unlisted = live - (listed + ignored); missing = listed - live. Both
// sorted, deduplicated and non-nil.
func diff(live, listed, ignored []string) (unlisted, missing []string) {
	set := func(xs ...[]string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			for _, s := range x {
				m[s] = true
			}
		}
		return m
	}
	liveSet, known := set(live), set(listed, ignored)
	unlisted, missing = []string{}, []string{}
	for s := range liveSet {
		if !known[s] {
			unlisted = append(unlisted, s)
		}
	}
	for s := range set(listed) {
		if !liveSet[s] {
			missing = append(missing, s)
		}
	}
	sort.Strings(unlisted)
	sort.Strings(missing)
	return unlisted, missing
}
