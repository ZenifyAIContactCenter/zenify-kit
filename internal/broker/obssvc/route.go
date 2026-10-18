package obssvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
)

const (
	defaultSince   = "24h"
	searchLimit    = "20"
	maxSamples     = 5
	resolution     = "log2 buckets"
	noMetricsNote  = "TraceQL metrics unavailable on this env"
	noSpansNote    = "no spans for this route in range — check route spelling with list_tempo_attribute_values span.http.route"
	unfilteredNote = "slow samples not filtered by p95"
)

// Route is the POST /obs/route handler: route health from TraceQL metrics,
// Tempo search samples and a Loki error count.
func (h *Handler) Route() http.Handler { return http.HandlerFunc(h.serveRoute) }

func (h *Handler) serveRoute(w http.ResponseWriter, r *http.Request) {
	start := h.now()
	status, size := http.StatusOK, 0
	var req api.ObsRouteRequest
	since := defaultSince
	defer func() {
		if h.audit != nil {
			h.audit.Log(audit.Entry{TS: start, Login: auth.Login(r.Context()), Role: auth.Role(r.Context()),
				Endpoint: "/obs/route", Env: req.Env, Op: "route",
				Args:  map[string]string{"service": req.Service, "route": req.Route, "since": since},
				Bytes: size, MS: h.now().Sub(start).Milliseconds(), Status: status})
		}
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		fail(http.StatusBadRequest, api.CodeInvalidBody, `body must be {"service","route","since","env"}`)
		return
	}
	if req.Env != catalog.Envs[0] && req.Env != catalog.Envs[1] {
		fail(http.StatusBadRequest, api.CodeInvalidBody, "env must be prd or stg")
		return
	}
	method, path, ok := splitRoute(req.Route)
	if !ok {
		fail(http.StatusBadRequest, api.CodeInvalidBody, `route must be "<METHOD> /<path>"`)
		return
	}
	if req.Since != "" {
		since = req.Since
	}
	d, err := time.ParseDuration(since)
	if err != nil {
		fail(http.StatusBadRequest, api.CodeBadSince, ErrBadSince.Error())
		return
	}
	windows, err := Windows(d, start)
	if err != nil {
		fail(http.StatusBadRequest, api.CodeBadSince, err.Error())
		return
	}
	login := auth.Login(r.Context())
	if !h.slots.Acquire(login, 1) {
		fail(http.StatusTooManyRequests, api.CodeTooManyConcurrent, "an obs request for this login is already running")
		return
	}
	defer h.slots.Release(login)
	c, _, err := h.cat.Current(r.Context())
	if err != nil {
		fail(http.StatusServiceUnavailable, api.CodeCatalogUnavailable, err.Error())
		return
	}
	e := findEntry(c, req.Service)
	if e == nil {
		fail(http.StatusBadRequest, api.CodeUnknownService, fmt.Sprintf("service %q is not in the catalog", req.Service))
		return
	}
	names := envNames(e, req.Env)
	if names == nil || len(names.Tempo) == 0 {
		fail(http.StatusBadRequest, api.CodeNoTempoName, fmt.Sprintf("service %q has no tempo name on %s", req.Service, req.Env))
		return
	}
	ds := c.Datasources[req.Env]
	if ds.Tempo == "" {
		fail(http.StatusBadRequest, api.CodeNoTempoName, "the catalog has no tempo datasource uid for "+req.Env)
		return
	}
	q := routeQuery{c: c, ds: ds, names: names, filter: spanFilter(names.Tempo, method, path), windows: windows, since: d, end: start}
	resp := h.route(r.Context(), q)
	resp.Service, resp.Route, resp.Env = req.Service, req.Route, req.Env
	b, err := CapJSON(cappedRoute{&resp}, maxResponse)
	if err != nil || len(b) > maxResponse {
		fail(http.StatusInternalServerError, api.CodeBrokerMisconfigured, "route summary does not fit the response cap")
		return
	}
	size = len(b)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// splitRoute splits "<METHOD> <path>" at the first space; METHOD must be
// upper case letters and path must start with "/".
func splitRoute(s string) (method, path string, ok bool) {
	method, path, ok = strings.Cut(s, " ")
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		return "", "", false
	}
	for _, c := range method {
		if c < 'A' || c > 'Z' {
			return "", "", false
		}
	}
	return method, path, true
}

// spanFilter is the FR-2.2 span condition, without the braces.
func spanFilter(tempo []string, method, path string) string {
	re := make([]string, len(tempo))
	for i, n := range tempo {
		re[i] = regexp.QuoteMeta(n)
	}
	m := quote(method)
	return fmt.Sprintf(`resource.service.name =~ "%s" && kind = server && span.http.route = "%s" && (span.http.method = "%s" || span.http.request.method = "%s")`,
		quote(strings.Join(re, "|")), quote(path), m, m)
}

type routeQuery struct {
	c       *catalog.Catalog
	ds      catalog.Datasources
	names   *catalog.EnvNames
	filter  string
	windows []Window
	since   time.Duration
	end     time.Time
}

// route runs the metric windows, the error search and the Loki count at most
// `parallel` at a time, then the slow search (it needs p95). A failing call
// only adds a note.
func (h *Handler) route(ctx context.Context, q routeQuery) api.ObsRouteResponse {
	resp := api.ObsRouteResponse{Notes: []string{}, Logs: api.ObsLogCount{Scope: "app"},
		Samples: api.ObsSamples{Slowest: []api.ObsSample{}, Errors: []api.ObsSample{}}}
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		hist     = Hist{}
		byStatus = map[string]float64{}
		okDur    time.Duration
		failed   int
		errs     []api.ObsSample
		errsErr  error
		lokiErr  error
	)
	sem := make(chan struct{}, parallel)
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			f()
		}()
	}
	for _, win := range q.windows {
		run(func() {
			series, err := h.metrics(ctx, q.ds.Tempo, q.filter, win)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			okDur += win.End.Sub(win.Start)
			for _, s := range series {
				hist[s.bucket] += s.value
				byStatus[s.status] += s.value
			}
		})
	}
	run(func() {
		s, err := h.search(ctx, q, " && (span.http.status_code >= 500 || span.http.response.status_code >= 500 || status = error)")
		mu.Lock()
		errs, errsErr = s, err
		mu.Unlock()
	})
	apps := []string(nil)
	if q.ds.Loki != "" && q.c.Labels.LokiApp != "" {
		apps = q.names.Loki
	}
	if len(apps) > 0 {
		run(func() {
			n, err := h.lokiErrors(ctx, q, apps)
			mu.Lock()
			resp.Logs.ErrorLines, lokiErr = n, err
			mu.Unlock()
		})
	}
	wg.Wait()

	if failed > 0 {
		resp.Notes = append(resp.Notes, fmt.Sprintf("%d of %d metric windows failed", failed, len(q.windows)))
	}
	if len(apps) == 0 {
		resp.Notes = append(resp.Notes, "no loki app")
	} else if lokiErr != nil {
		resp.Notes = append(resp.Notes, "loki error count failed: "+lokiErr.Error())
	}
	var total float64
	for _, n := range hist {
		total += n
	}
	slowCond := ""
	if total > 0 {
		slowCond = " && duration > " + strconv.FormatFloat(Quantile(hist, 0.95)*1000, 'f', 3, 64) + "ms"
	} else {
		resp.Notes = append(resp.Notes, unfilteredNote)
	}
	slow, slowErr := h.search(ctx, q, slowCond)
	if slowErr != nil {
		resp.Notes = append(resp.Notes, "slow sample search failed: "+slowErr.Error())
	}
	if errsErr != nil {
		resp.Notes = append(resp.Notes, "error sample search failed: "+errsErr.Error())
	}
	resp.Samples.Slowest, resp.Samples.Errors = top(slow), top(errs)
	resp.SinceEffective = okDur.String()

	switch {
	case okDur == 0: // every window failed: no stats, the failure note says why
	case total == 0 && len(slow)+len(errs) > 0:
		resp.StatsNote = noMetricsNote
	case total == 0:
		resp.Stats = &api.ObsStats{Resolution: resolution, Status: map[string]int64{}}
		resp.Notes = append(resp.Notes, noSpansNote)
	default:
		resp.Stats = stats(hist, byStatus, total, okDur)
	}
	return resp
}

func stats(hist Hist, byStatus map[string]float64, total float64, d time.Duration) *api.ObsStats {
	s := &api.ObsStats{Count: int64(total), RatePerMin: total / d.Minutes(),
		P50: Quantile(hist, 0.5), P95: Quantile(hist, 0.95), P99: Quantile(hist, 0.99),
		Resolution: resolution, Status: map[string]int64{}}
	var bad float64
	for k, n := range byStatus {
		s.Status[k] = int64(n)
		if code, err := strconv.Atoi(k); err == nil && code >= 500 {
			bad += n
		}
	}
	s.ErrorRatio = bad / total
	return s
}

// tqValue is an OTLP-JSON attribute value as Tempo returns it.
type tqValue struct {
	IntValue    string  `json:"intValue"`
	DoubleValue float64 `json:"doubleValue"`
	StringValue string  `json:"stringValue"`
}

type tqAttr struct {
	Key   string  `json:"key"`
	Value tqValue `json:"value"`
}

type point struct {
	status        string
	bucket, value float64
}

// metrics runs one instant histogram query over win.
func (h *Handler) metrics(ctx context.Context, uid, filter string, win Window) ([]point, error) {
	var out struct {
		Series []struct {
			Labels []tqAttr `json:"labels"`
			Value  float64  `json:"value"`
		} `json:"series"`
	}
	v := url.Values{
		"q":     {"{ " + filter + " } | histogram_over_time(span:duration) by (span.http.status_code, span.http.response.status_code)"},
		"start": {strconv.FormatInt(win.Start.Unix(), 10)},
		"end":   {strconv.FormatInt(win.End.Unix(), 10)},
	}
	if err := h.g.Get(ctx, uid, "/api/metrics/query", v, maxUpstream, &out); err != nil {
		return nil, err
	}
	pts := make([]point, 0, len(out.Series))
	for _, s := range out.Series {
		p := point{value: s.Value}
		var code, respCode string
		for _, l := range s.Labels {
			switch l.Key {
			case "__bucket":
				p.bucket = l.Value.DoubleValue
			case "span.http.status_code":
				code = l.Value.IntValue
			case "span.http.response.status_code":
				respCode = l.Value.IntValue
			}
		}
		p.status = firstOf(code, respCode, "unset")
		pts = append(pts, p)
	}
	return pts, nil
}

// search runs one Tempo search over the whole range with cond appended to
// the span filter.
func (h *Handler) search(ctx context.Context, q routeQuery, cond string) ([]api.ObsSample, error) {
	type spanSet struct {
		Spans []struct {
			Attributes []tqAttr `json:"attributes"`
		} `json:"spans"`
	}
	var out struct {
		Traces []struct {
			TraceID           string    `json:"traceID"`
			StartTimeUnixNano string    `json:"startTimeUnixNano"`
			DurationMs        float64   `json:"durationMs"`
			SpanSet           *spanSet  `json:"spanSet"`
			SpanSets          []spanSet `json:"spanSets"`
		} `json:"traces"`
	}
	v := url.Values{
		// Tempo returns only the attributes a query names; select asks for the status ones.
		"q":     {"{ " + q.filter + cond + " } | select(span.http.status_code, span.http.response.status_code, status)"},
		"start": {strconv.FormatInt(q.windows[0].Start.Unix(), 10)},
		"end":   {strconv.FormatInt(q.end.Unix(), 10)},
		"limit": {searchLimit},
		"spss":  {"1"},
	}
	if err := h.g.Get(ctx, q.ds.Tempo, "/api/search", v, maxUpstream, &out); err != nil {
		return nil, err
	}
	samples := []api.ObsSample{}
	for _, t := range out.Traces {
		id, _, err := PadTraceID(t.TraceID)
		if err != nil {
			continue
		}
		s := api.ObsSample{TraceID: id, DurationMS: t.DurationMs, Status: "unset"}
		if ns, err := strconv.ParseInt(t.StartTimeUnixNano, 10, 64); err == nil {
			s.Start = time.Unix(0, ns).UTC().Format(time.RFC3339)
		}
		sets := t.SpanSets
		if t.SpanSet != nil {
			sets = append(sets, *t.SpanSet)
		}
		for _, set := range sets {
			for _, sp := range set.Spans {
				if st := spanStatus(sp.Attributes); st != "" {
					s.Status = st
				}
			}
		}
		samples = append(samples, s)
	}
	return samples, nil
}

// spanStatus is the span's HTTP status, else "error" for status = error, else "".
func spanStatus(attrs []tqAttr) string {
	var code, errStatus string
	for _, a := range attrs {
		switch strings.TrimPrefix(a.Key, "span.") {
		case "http.status_code", "http.response.status_code":
			if code == "" {
				code = a.Value.IntValue
			}
		case "status":
			if a.Value.StringValue == "error" {
				errStatus = "error"
			}
		}
	}
	return Mask(firstOf(code, errStatus, ""))
}

// lokiErrors counts the apps' lines matching \berror\b over since.
func (h *Handler) lokiErrors(ctx context.Context, q routeQuery, apps []string) (int64, error) {
	re := make([]string, len(apps))
	for i, a := range apps {
		re[i] = regexp.QuoteMeta(a)
	}
	secs := int64(q.since / time.Second)
	if secs < 1 {
		secs = 1
	}
	var out struct {
		Data struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	v := url.Values{
		"query": {fmt.Sprintf(`sum(count_over_time({%s=~"%s"} |~ "(?i)\\berror\\b" [%ds]))`, q.c.Labels.LokiApp, quote(strings.Join(re, "|")), secs)},
		"time":  {strconv.FormatInt(q.end.UnixNano(), 10)},
	}
	if err := h.g.Get(ctx, q.ds.Loki, "/loki/api/v1/query", v, maxUpstream, &out); err != nil {
		return 0, err
	}
	if len(out.Data.Result) == 0 || len(out.Data.Result[0].Value) < 2 {
		return 0, nil
	}
	s, _ := out.Data.Result[0].Value[1].(string)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("loki returned a non-numeric count")
	}
	return int64(f), nil
}

// top sorts samples by duration, longest first, and keeps maxSamples.
func top(s []api.ObsSample) []api.ObsSample {
	out := append([]api.ObsSample{}, s...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].DurationMS > out[j].DurationMS })
	if len(out) > maxSamples {
		out = out[:maxSamples]
	}
	return out
}

func firstOf(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// cappedRoute sheds, in order: error samples from the end, slow samples from
// the end, then status entries.
type cappedRoute struct{ *api.ObsRouteResponse }

func (c cappedRoute) Drop() (string, bool) {
	r := c.ObsRouteResponse
	var kind string
	switch {
	case len(r.Samples.Errors) > 0:
		r.Samples.Errors, kind = r.Samples.Errors[:len(r.Samples.Errors)-1], "errors"
	case len(r.Samples.Slowest) > 0:
		r.Samples.Slowest, kind = r.Samples.Slowest[:len(r.Samples.Slowest)-1], "slowest"
	case r.Stats != nil && len(r.Stats.Status) > 0:
		keys := make([]string, 0, len(r.Stats.Status))
		for k := range r.Stats.Status {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		delete(r.Stats.Status, keys[len(keys)-1])
		kind = "status"
	default:
		return "", false
	}
	r.Truncated = true
	if r.Omitted == nil {
		r.Omitted = map[string]int{}
	}
	r.Omitted[kind]++
	return kind, true
}
