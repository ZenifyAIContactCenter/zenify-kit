package obssvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/grafanaproxy"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
)

const (
	maxGroups     = 30
	maxErrSpans   = 10
	maxLogLines   = 50
	maxLogLine    = 500
	maxLogRead    = 1024
	maxAttr       = 500
	maxStatement  = 300
	lokiTimeout   = 15 * time.Second
	lokiReserve   = 5 * time.Second
	logPad        = 2 * time.Minute
	noRootNote    = "no root span"
	noLokiAppNote = "no loki app"
	notFoundMsg   = "trace not found — it may be older than Tempo's retention"
	tooLargeMsg   = "trace exceeds 16 MB — use --route or Grafana"
)

// Trace is the POST /obs/trace handler: a masked summary of one trace plus
// the Loki lines that mention its id.
func (h *Handler) Trace() http.Handler { return http.HandlerFunc(h.serveTrace) }

func (h *Handler) serveTrace(w http.ResponseWriter, r *http.Request) {
	start := h.now()
	status, size := http.StatusOK, 0
	var req api.ObsTraceRequest
	padded := ""
	defer func() {
		if h.audit != nil {
			h.audit.Log(audit.Entry{TS: start, Login: auth.Login(r.Context()), Role: auth.Role(r.Context()),
				Endpoint: "/obs/trace", Env: req.Env, Op: "trace", Args: map[string]string{"trace_id": padded},
				Bytes: size, MS: h.now().Sub(start).Milliseconds(), Status: status})
		}
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		fail(http.StatusBadRequest, api.CodeInvalidBody, `body must be {"trace_id","env"}`)
		return
	}
	if req.Env != catalog.Envs[0] && req.Env != catalog.Envs[1] {
		fail(http.StatusBadRequest, api.CodeInvalidBody, "env must be prd or stg")
		return
	}
	padded, orig, err := PadTraceID(req.TraceID)
	if err != nil {
		fail(http.StatusBadRequest, api.CodeBadTraceID, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()
	login := auth.Login(r.Context())
	if !h.slots.Acquire(login, 1) {
		fail(http.StatusTooManyRequests, api.CodeTooManyConcurrent, "an obs request for this login is already running")
		return
	}
	defer h.slots.Release(login)
	c, _, err := h.cat.Current(ctx)
	if err != nil {
		fail(http.StatusServiceUnavailable, api.CodeCatalogUnavailable, err.Error())
		return
	}
	ds := c.Datasources[req.Env]
	if ds.Tempo == "" {
		fail(http.StatusBadRequest, api.CodeNoTempoName, "the catalog has no tempo datasource uid for "+req.Env)
		return
	}
	spans, partial, err := h.fetchTrace(ctx, ds.Tempo, padded)
	switch {
	case errors.Is(err, grafanaproxy.ErrNotFound) || err == nil && len(spans) == 0:
		fail(http.StatusNotFound, api.CodeTraceNotFound, notFoundMsg)
		return
	case errors.Is(err, grafanaproxy.ErrTooLarge):
		fail(http.StatusUnprocessableEntity, api.CodeTraceTooLarge, tooLargeMsg)
		return
	case err != nil:
		fail(http.StatusBadGateway, api.CodeUpstreamUnavailable, "tempo: "+err.Error())
		return
	}
	resp := summarize(spans, c.Attributes)
	resp.TraceID = padded
	if partial != "" {
		resp.Notes = append(resp.Notes, partial)
		resp.Truncated = true
	}
	lctx, lcancel := lokiContext(ctx)
	defer lcancel()
	h.traceLogs(lctx, &resp, c, req.Env, spans, padded, orig)
	b, err := CapJSON(cappedTrace{&resp}, maxResponse)
	if err != nil || len(b) > maxResponse {
		fail(http.StatusInternalServerError, api.CodeBrokerMisconfigured, "trace summary does not fit the response cap")
		return
	}
	size = len(b)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// lokiContext gives the Loki call min(lokiTimeout, remaining budget - lokiReserve),
// so a slow Loki leaves time to send the trace summary.
func lokiContext(ctx context.Context) (context.Context, context.CancelFunc) {
	d := lokiTimeout
	if dl, ok := ctx.Deadline(); ok {
		d = min(d, time.Until(dl)-lokiReserve)
	}
	return context.WithTimeout(ctx, d)
}

// otlpValue is an OTLP-JSON AnyValue; intValue may come quoted or bare.
type otlpValue struct {
	StringValue *string         `json:"stringValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
	BoolValue   *bool           `json:"boolValue"`
}

func (v otlpValue) String() string {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case len(v.IntValue) > 0:
		return unquote(v.IntValue)
	case v.DoubleValue != nil:
		return strconv.FormatFloat(*v.DoubleValue, 'f', -1, 64)
	case v.BoolValue != nil:
		return strconv.FormatBool(*v.BoolValue)
	}
	return ""
}

type otlpAttr struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

// span is one Tempo span flattened with its resource's service.name. svc is
// the raw name (for the catalog lookup); service and name are masked.
type span struct {
	svc, service, name string
	start, end         int64
	root, failed       bool
	attrs              []otlpAttr
}

// fetchTrace reads GET /api/v2/traces/<padded> (OTLP JSON) into flat spans.
// partial is the note to add when Tempo says the trace is incomplete. Only
// failed spans keep their attributes.
func (h *Handler) fetchTrace(ctx context.Context, uid, padded string) (spans []span, partial string, err error) {
	var out struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Trace   struct {
			ResourceSpans []struct {
				Resource struct {
					Attributes []otlpAttr `json:"attributes"`
				} `json:"resource"`
				ScopeSpans []struct {
					Spans []struct {
						Name         string          `json:"name"`
						ParentSpanID string          `json:"parentSpanId"`
						Start        json.RawMessage `json:"startTimeUnixNano"`
						End          json.RawMessage `json:"endTimeUnixNano"`
						Status       struct {
							Code json.RawMessage `json:"code"`
						} `json:"status"`
						Attributes []otlpAttr `json:"attributes"`
					} `json:"spans"`
				} `json:"scopeSpans"`
			} `json:"resourceSpans"`
		} `json:"trace"`
	}
	if err := h.g.Get(ctx, uid, "/api/v2/traces/"+padded, nil, h.traceMax, &out); err != nil {
		return nil, "", err
	}
	if strings.EqualFold(out.Status, "partial") {
		partial = strings.TrimSuffix("tempo returned a partial trace: "+Mask(out.Message), ": ")
	}
	for _, rs := range out.Trace.ResourceSpans {
		svc := ""
		for _, a := range rs.Resource.Attributes {
			if a.Key == "service.name" {
				svc = a.Value.String()
			}
		}
		service := Mask(svc)
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				code := unquote(sp.Status.Code)
				s := span{svc: svc, service: service, name: Mask(sp.Name),
					start: nanos(sp.Start), end: nanos(sp.End), root: sp.ParentSpanID == "",
					failed: code == "STATUS_CODE_ERROR" || code == "2"}
				for _, a := range sp.Attributes {
					if a.Key == "http.status_code" || a.Key == "http.response.status_code" {
						if n, err := strconv.Atoi(a.Value.String()); err == nil && n >= 500 {
							s.failed = true
						}
					}
				}
				if s.failed {
					s.attrs = sp.Attributes
				}
				spans = append(spans, s)
			}
		}
	}
	return spans, partial, nil
}

// summarize builds root, services, groups and error spans (FR-3.3, FR-3.4).
func summarize(spans []span, extra map[string]string) api.ObsTraceResponse {
	resp := api.ObsTraceResponse{SpanCount: len(spans), Services: []string{}, Groups: []api.ObsGroup{},
		Errors: []api.ObsErrSpan{}, Notes: []string{},
		Logs: api.ObsTraceLogs{Lines: []api.ObsLogLine{}, UnmappedServices: []string{}}}
	byStart := append([]span(nil), spans...)
	sort.SliceStable(byStart, func(i, j int) bool { return byStart[i].start < byStart[j].start })

	root := -1
	for i, s := range byStart {
		if s.root {
			root = i
			break
		}
	}
	if root < 0 {
		root = 0
		resp.Notes = append(resp.Notes, noRootNote)
	}
	rs := byStart[root]
	resp.Root = api.ObsSpanRef{Service: rs.service, Name: rs.name,
		Start: time.Unix(0, rs.start).UTC().Format(time.RFC3339Nano), DurationMS: ms(rs.end - rs.start)}

	type key struct{ service, name string }
	groups := map[key]*api.ObsGroup{}
	services := map[string]bool{}
	for _, s := range byStart {
		services[s.service] = true
		g := groups[key{s.service, s.name}]
		if g == nil {
			g = &api.ObsGroup{Service: s.service, Name: s.name}
			groups[key{s.service, s.name}] = g
		}
		d := ms(s.end - s.start)
		g.Count++
		g.TotalMS += d
		if d > g.MaxMS {
			g.MaxMS = d
		}
		if !s.failed {
			continue
		}
		g.Errors++
		if len(resp.Errors) < maxErrSpans {
			resp.Errors = append(resp.Errors, api.ObsErrSpan{Service: s.service, Name: s.name, Attributes: keepAttrs(s.attrs, extra)})
		}
	}
	resp.Services = sortedKeys(services)
	for _, g := range groups {
		resp.Groups = append(resp.Groups, *g)
	}
	sort.Slice(resp.Groups, func(i, j int) bool {
		a, b := resp.Groups[i], resp.Groups[j]
		if a.TotalMS != b.TotalMS {
			return a.TotalMS > b.TotalMS
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		return a.Name < b.Name
	})
	if len(resp.Groups) > maxGroups {
		resp.Groups = resp.Groups[:maxGroups]
	}
	return resp
}

// keepAttrs keeps the allowlisted attributes, masked and cut to 500 B;
// db.statement is cut to 300 B.
func keepAttrs(attrs []otlpAttr, extra map[string]string) map[string]string {
	out := map[string]string{}
	for _, a := range attrs {
		if !AllowAttr(a.Key, extra) {
			continue
		}
		limit := maxAttr
		if a.Key == "db.statement" {
			limit = min(limit, maxStatement)
		}
		out[a.Key] = Truncate(Mask(a.Value.String()), limit)
	}
	return out
}

// traceLogs maps the trace's services to catalog Loki apps and, when there
// are any, reads the lines mentioning the trace id (FR-3.5). A Loki failure
// only adds a note.
func (h *Handler) traceLogs(ctx context.Context, resp *api.ObsTraceResponse, c *catalog.Catalog, env string, spans []span, padded, orig string) {
	byTempo := map[string]*catalog.EnvNames{}
	for _, list := range [][]catalog.Entry{c.Services, c.Datastores} {
		for i := range list {
			if names := envNames(&list[i], env); names != nil {
				for _, t := range names.Tempo {
					byTempo[t] = names
				}
			}
		}
	}
	svcs := map[string]bool{}
	first, last := spans[0].start, spans[0].end
	for _, s := range spans {
		svcs[s.svc] = true
		first, last = min(first, s.start), max(last, s.end)
	}
	var apps []string
	seen := map[string]bool{}
	unmapped := map[string]bool{}
	for _, svc := range sortedKeys(svcs) {
		names, ok := byTempo[svc]
		if !ok {
			unmapped[Mask(svc)] = true
			continue
		}
		for _, a := range names.Loki {
			if !seen[a] {
				seen[a] = true
				apps = append(apps, a)
			}
		}
	}
	resp.Logs.UnmappedServices = sortedKeys(unmapped)
	ds := c.Datasources[env]
	if len(apps) == 0 || ds.Loki == "" || c.Labels.LokiApp == "" {
		resp.Notes = append(resp.Notes, noLokiAppNote)
		return
	}
	lines, err := h.lokiLines(ctx, ds.Loki, c.Labels.LokiApp, apps, padded, orig, first-int64(logPad), last+int64(logPad))
	if err != nil {
		resp.Notes = append(resp.Notes, "loki query failed: "+err.Error())
		return
	}
	resp.Logs.Lines = lines
}

// lokiLines runs one query_range over the apps for lines matching either id
// form; start and end are epoch ns.
func (h *Handler) lokiLines(ctx context.Context, uid, label string, apps []string, padded, orig string, start, end int64) ([]api.ObsLogLine, error) {
	re := make([]string, len(apps))
	for i, a := range apps {
		re[i] = regexp.QuoteMeta(a)
	}
	ids := padded
	if orig != padded {
		ids += "|" + orig
	}
	var out struct {
		Data struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	v := url.Values{
		"query":     {fmt.Sprintf(`{%s=~"%s"} |~ "%s"`, label, quote(strings.Join(re, "|")), quote(ids))},
		"start":     {strconv.FormatInt(start, 10)},
		"end":       {strconv.FormatInt(end, 10)},
		"limit":     {strconv.Itoa(maxLogLines)},
		"direction": {"forward"},
	}
	if err := h.g.Get(ctx, uid, "/loki/api/v1/query_range", v, maxUpstream, &out); err != nil {
		return nil, err
	}
	type entry struct {
		ns   int64
		line api.ObsLogLine
	}
	var all []entry
	for _, res := range out.Data.Result {
		app := Mask(res.Stream[label])
		for _, val := range res.Values {
			ns, _ := strconv.ParseInt(val[0], 10, 64)
			all = append(all, entry{ns, api.ObsLogLine{TS: time.Unix(0, ns).UTC().Format(time.RFC3339Nano),
				App: app, Line: Truncate(Mask(preCut(val[1], maxLogRead)), maxLogLine)}})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ns < all[j].ns })
	if len(all) > maxLogLines {
		all = all[:maxLogLines]
	}
	lines := make([]api.ObsLogLine, len(all))
	for i, e := range all {
		lines[i] = e.line
	}
	return lines, nil
}

func unquote(raw json.RawMessage) string { return strings.Trim(string(raw), `"`) }

func nanos(raw json.RawMessage) int64 {
	n, _ := strconv.ParseInt(unquote(raw), 10, 64)
	return n
}

func ms(ns int64) float64 { return float64(ns) / 1e6 }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cappedTrace sheds, in order: log lines, error spans, then groups, each
// from the end of its list (FR-4.3).
type cappedTrace struct{ *api.ObsTraceResponse }

func (c cappedTrace) Drop() (string, bool) {
	r := c.ObsTraceResponse
	var kind string
	switch {
	case len(r.Logs.Lines) > 0:
		r.Logs.Lines, kind = r.Logs.Lines[:len(r.Logs.Lines)-1], "log_lines"
	case len(r.Errors) > 0:
		r.Errors, kind = r.Errors[:len(r.Errors)-1], "errors"
	case len(r.Groups) > 0:
		r.Groups, kind = r.Groups[:len(r.Groups)-1], "groups"
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
