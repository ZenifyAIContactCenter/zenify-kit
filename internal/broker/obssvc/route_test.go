package obssvc_test

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/catalogsvc"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/config"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/obssvc"
)

// routeYAML: svc-a has a Tempo and a Loki name on prd; svc-b has no Tempo name.
const routeYAML = `version: 1
datasources:
  prd: {loki: uid-loki, tempo: uid-tempo}
labels: {loki_app: app}
attributes: {tenant.id: "span attribute"}
services:
  - id: svc-a
    repo: org/svc-a
    prd: {loki: [app-a], tempo: [svc-a]}
  - id: svc-b
    repo: org/svc-b
    prd: {loki: [app-b]}
`

const (
	metricsPath = "/api/datasources/proxy/uid/uid-tempo/api/metrics/query"
	searchPath  = "/api/datasources/proxy/uid/uid-tempo/api/search"
	lokiPath    = "/api/datasources/proxy/uid/uid-loki/loki/api/v1/query"
)

// One window: 200 x 50 at 0.033554432s, 200 x 50 at 0.067108864s, 500 x 2 at
// 0.134217728s (the 500 rides on the newer semconv label).
const metricsBody = `{"series":[
{"labels":[{"key":"span.http.status_code","value":{"intValue":"200"}},{"key":"__bucket","value":{"doubleValue":0.033554432}}],"value":50},
{"labels":[{"key":"span.http.status_code","value":{"intValue":"200"}},{"key":"__bucket","value":{"doubleValue":0.067108864}}],"value":50},
{"labels":[{"key":"span.http.response.status_code","value":{"intValue":"500"}},{"key":"__bucket","value":{"doubleValue":0.134217728}}],"value":2}
]}`

// searchOne: one trace, 29-char id, started 2026-10-17T23:50:00Z, 120 ms, 500.
const searchOne = `{"traces":[{"traceID":"abcdef0123456789abcdef0123456","rootServiceName":"svc-a","startTimeUnixNano":"1792281000000000000","durationMs":120,
"spanSets":[{"spans":[{"spanID":"s1","attributes":[{"key":"http.status_code","value":{"intValue":"500"}}]}],"matched":1}]}]}`

const lokiBody = `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1792281600,"7"]}]}}`

// routeNow is the fake clock: 2026-10-18T00:00:00Z.
var routeNow = time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)

// fakeGrafana answers metrics, both searches and Loki; it counts calls and
// keeps every query string.
type fakeGrafana struct {
	mu          sync.Mutex
	metrics     string
	slow, errs  string
	failMetrics int32 // this many metric calls answer 500
	hangMetrics int32 // this many metric calls hang until the caller gives up
	hangErrs    bool  // the error search hangs
	hangLoki    bool  // the Loki count hangs
	calls       int
	queries     []string
}

// hang blocks until the caller cancels the request (or 10 s pass).
func hang(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(10 * time.Second):
	}
}

func (g *fakeGrafana) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	g.mu.Lock()
	g.calls++
	g.queries = append(g.queries, q.Get("q")+q.Get("query"))
	metrics, slow, errs := g.metrics, g.slow, g.errs
	hangErrs, hangLoki := g.hangErrs, g.hangLoki
	g.mu.Unlock()
	switch r.URL.Path {
	case metricsPath:
		if atomic.AddInt32(&g.hangMetrics, -1) >= 0 {
			hang(r)
			return
		}
		if atomic.AddInt32(&g.failMetrics, -1) >= 0 {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(metrics))
	case searchPath:
		if strings.Contains(q.Get("q"), ">= 500") {
			if hangErrs {
				hang(r)
				return
			}
			_, _ = w.Write([]byte(errs))
		} else if strings.Contains(q.Get("q"), "| select(") {
			_, _ = w.Write([]byte(slow))
		} else {
			// Tempo returns only the attributes a query names.
			_, _ = w.Write([]byte(strings.Replace(slow, `{"key":"http.status_code","value":{"intValue":"500"}}`, "", 1)))
		}
	case lokiPath:
		if hangLoki {
			hang(r)
			return
		}
		_, _ = w.Write([]byte(lokiBody))
	default:
		http.NotFound(w, r)
	}
}

func (g *fakeGrafana) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *fakeGrafana) all() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.queries...)
}

func newFake() *fakeGrafana {
	return &fakeGrafana{metrics: metricsBody, slow: searchOne, errs: searchOne}
}

func setup(t *testing.T, g http.Handler, opts ...obssvc.Option) *obssvc.Handler {
	t.Helper()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(routeYAML))
	}))
	t.Cleanup(gh.Close)
	gs := httptest.NewServer(g)
	t.Cleanup(gs.Close)
	now := func() time.Time { return routeNow }
	cat := catalogsvc.New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token",
		catalogsvc.WithBaseURL(gh.URL), catalogsvc.WithClock(now))
	return obssvc.New(cat, "http://127.0.0.1:1", "glsa_secret_token",
		append([]obssvc.Option{obssvc.WithClock(now), obssvc.WithGrafanaBaseURL(gs.URL)}, opts...)...)
}

func post(h http.Handler, body string) (int, api.ObsRouteResponse, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/obs/route", strings.NewReader(body)))
	var resp api.ObsRouteResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp, rec.Body.String()
}

func errCode(raw string) string {
	var e api.Error
	_ = json.Unmarshal([]byte(raw), &e)
	return e.Error
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

const okBody = `{"service":"svc-a","route":"GET /a/:id","env":"prd"}`

func TestRoute_StatsFromHistogram(t *testing.T) {
	var buf bytes.Buffer
	g := newFake()
	h := setup(t, g, obssvc.WithAudit(&audit.Logger{W: &buf}))
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	s := resp.Stats
	if s == nil {
		t.Fatalf("stats nil: %s", raw)
	}
	if s.Count != 1224 || s.Status["200"] != 1200 || s.Status["500"] != 24 {
		t.Errorf("count=%d status=%v, want 1224 {200:1200 500:24}", s.Count, s.Status)
	}
	if !near(s.ErrorRatio, 24.0/1224) || !near(s.RatePerMin, 1224.0/1440) {
		t.Errorf("error_ratio=%v rate=%v", s.ErrorRatio, s.RatePerMin)
	}
	// Buckets 0.0336:600, 0.0671:600, 0.1342:24 → ranks 612, 1162.8, 1211.76.
	if !near(s.P50, 0.033554432*1.02) || !near(s.P95, 0.033554432*1.938) || !near(s.P99, 0.067108864*1.49) {
		t.Errorf("p50=%v p95=%v p99=%v", s.P50, s.P95, s.P99)
	}
	if s.Resolution != "log2 buckets" || resp.SinceEffective != "24h0m0s" {
		t.Errorf("resolution=%q since_effective=%q", s.Resolution, resp.SinceEffective)
	}
	if n := g.count(); n != 15 {
		t.Errorf("grafana calls = %d, want 15", n)
	}
	if len(resp.Samples.Slowest) != 1 || resp.Samples.Slowest[0].Status != "500" {
		t.Errorf("slowest = %+v, want one sample with status 500", resp.Samples.Slowest)
	}
	if resp.Logs.ErrorLines != 7 || resp.Logs.Scope != "app" {
		t.Errorf("logs = %+v", resp.Logs)
	}
	var slowQ, lokiQ string
	for _, q := range g.all() {
		if strings.Contains(q, "duration >") {
			slowQ = q
		}
		if strings.Contains(q, "count_over_time") {
			lokiQ = q
		}
	}
	if !strings.Contains(slowQ, "&& duration > 65.028ms }") {
		t.Errorf("slow search q = %q, want the p95 condition", slowQ)
	}
	if lokiQ != `sum(count_over_time({app=~"app-a"} |~ "(?i)error" |~ "(?i)\\berror\\b" [86400s]))` {
		t.Errorf("loki query = %q", lokiQ)
	}
	var row map[string]any
	if err := json.Unmarshal(buf.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	args, _ := row["args"].(map[string]any)
	if row["endpoint"] != "/obs/route" || row["op"] != "route" || row["env"] != "prd" || row["status"] != float64(200) ||
		args["service"] != "svc-a" || args["route"] != "GET /a/:id" || args["since"] != "24h" || row["bytes"] != float64(len(raw)) {
		t.Errorf("audit row = %s (response %d bytes)", buf.String(), len(raw))
	}
}

func TestRoute_Budget48h(t *testing.T) {
	g := newFake()
	h := setup(t, g)
	code, resp, raw := post(h.Route(), `{"service":"svc-a","route":"GET /a","since":"48h","env":"prd"}`)
	if code != 200 || resp.SinceEffective != "48h0m0s" {
		t.Fatalf("code %d: %s", code, raw)
	}
	if n := g.count(); n != 27 {
		t.Errorf("grafana calls = %d, want 27", n)
	}
	code, _, raw = post(h.Route(), `{"service":"svc-a","route":"GET /a","since":"49h","env":"prd"}`)
	if code != 400 || errCode(raw) != api.CodeBadSince {
		t.Errorf("49h: %d %s", code, raw)
	}
}

func TestRoute_BadInput(t *testing.T) {
	g := newFake()
	h := setup(t, g)
	for _, c := range []struct{ body, code string }{
		{`{"service":"svc-a","route":"GET /a","env":"dev"}`, api.CodeInvalidBody},
		{`{"service":"svc-a","route":"GET /a"}`, api.CodeInvalidBody},
		{`{"service":"svc-a","route":"get /a","env":"prd"}`, api.CodeInvalidBody},
		{`{"service":"svc-a","route":"GET a","env":"prd"}`, api.CodeInvalidBody},
		{`{"service":"svc-a","route":"/a","env":"prd"}`, api.CodeInvalidBody},
		{`{"service":"svc-a","route":"GET /a","since":"soon","env":"prd"}`, api.CodeBadSince},
		{`{"service":"svc-a","route":"GET /a","since":"59s","env":"prd"}`, api.CodeBadSince},
		{`not json`, api.CodeInvalidBody},
	} {
		code, _, raw := post(h.Route(), c.body)
		if code != 400 || errCode(raw) != c.code {
			t.Errorf("%s: %d %s, want 400 %s", c.body, code, raw, c.code)
		}
	}
	if n := g.count(); n != 0 {
		t.Errorf("grafana calls = %d, want 0", n)
	}
}

func TestRoute_UnknownService(t *testing.T) {
	h := setup(t, newFake())
	code, _, raw := post(h.Route(), `{"service":"svc-x","route":"GET /a","env":"prd"}`)
	if code != 400 || errCode(raw) != api.CodeUnknownService {
		t.Fatalf("got %d %s", code, raw)
	}
}

func TestRoute_NoTempoName(t *testing.T) {
	g := newFake()
	h := setup(t, g)
	code, _, raw := post(h.Route(), `{"service":"svc-b","route":"GET /a","env":"prd"}`)
	if code != 400 || errCode(raw) != api.CodeNoTempoName {
		t.Fatalf("got %d %s", code, raw)
	}
	// stg has no tempo names for svc-a either.
	code, _, raw = post(h.Route(), `{"service":"svc-a","route":"GET /a","env":"stg"}`)
	if code != 400 || errCode(raw) != api.CodeNoTempoName {
		t.Fatalf("stg: got %d %s", code, raw)
	}
	if n := g.count(); n != 0 {
		t.Errorf("grafana calls = %d, want 0", n)
	}
}

func TestRoute_MetricsUnavailable(t *testing.T) {
	g := newFake()
	g.metrics = `{"series":[]}`
	h := setup(t, g)
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if resp.Stats != nil || resp.StatsNote != "TraceQL metrics unavailable on this env" {
		t.Errorf("stats=%+v note=%q", resp.Stats, resp.StatsNote)
	}
	if !strings.Contains(raw, `"stats":null`) {
		t.Errorf("stats not a literal null: %s", raw)
	}
	if len(resp.Samples.Slowest) != 1 {
		t.Errorf("slowest = %+v", resp.Samples.Slowest)
	}
	if !contains(resp.Notes, "slow samples not filtered by p95") {
		t.Errorf("notes = %v", resp.Notes)
	}
	for _, q := range g.all() {
		if strings.Contains(q, "duration >") {
			t.Errorf("slow search filtered without stats: %q", q)
		}
	}
}

func TestRoute_NoSpans(t *testing.T) {
	g := newFake()
	g.metrics, g.slow, g.errs = `{"series":[]}`, `{"traces":[]}`, `{"traces":[]}`
	h := setup(t, g)
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 || resp.Stats == nil || resp.Stats.Count != 0 || resp.StatsNote != "" {
		t.Fatalf("code %d: %s", code, raw)
	}
	if !contains(resp.Notes, "no spans for this route in range") {
		t.Errorf("notes = %v", resp.Notes)
	}
}

func TestRoute_PartialWindowFailure(t *testing.T) {
	g := newFake()
	g.failMetrics = 1
	h := setup(t, g)
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if !contains(resp.Notes, "1 of 12 metric windows failed") || resp.SinceEffective != "22h0m0s" {
		t.Errorf("notes=%v since_effective=%q", resp.Notes, resp.SinceEffective)
	}
	if resp.Stats == nil || resp.Stats.Count != 1122 {
		t.Errorf("stats = %+v, want count 1122 (11 windows)", resp.Stats)
	}
	if strings.Contains(raw, "glsa_secret_token") {
		t.Error("response leaks the token")
	}
}

func TestRoute_AllWindowsFail(t *testing.T) {
	g := newFake()
	g.failMetrics = 12
	h := setup(t, g)
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 || resp.Stats != nil || resp.SinceEffective != "0s" || resp.StatsNote != "" {
		t.Fatalf("code %d: %s", code, raw)
	}
	if !contains(resp.Notes, "12 of 12 metric windows failed") {
		t.Errorf("notes = %v", resp.Notes)
	}
}

func TestRoute_EscapesRoute(t *testing.T) {
	g := newFake()
	h := setup(t, g)
	if code, _, raw := post(h.Route(), `{"service":"svc-a","route":"GET /a\"b","env":"prd"}`); code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	want := `{ resource.service.name =~ "svc-a" && kind = server && span.http.route = "/a\"b" && (span.http.method = "GET" || span.http.request.method = "GET")`
	for _, q := range g.all() {
		if !strings.Contains(q, "count_over_time") && !strings.Contains(q, want) {
			t.Errorf("q = %q, want it to contain %q", q, want)
		}
	}
}

func TestRoute_TraceIDPadded(t *testing.T) {
	h := setup(t, newFake())
	_, resp, raw := post(h.Route(), okBody)
	if len(resp.Samples.Errors) != 1 {
		t.Fatalf("errors = %s", raw)
	}
	got := resp.Samples.Errors[0]
	want := api.ObsSample{TraceID: "000abcdef0123456789abcdef0123456", Start: "2026-10-17T23:50:00Z", DurationMS: 120, Status: "500"}
	if got != want {
		t.Errorf("sample = %+v, want %+v", got, want)
	}
}

// gated holds every request until release is closed.
type gated struct {
	next    http.Handler
	arrived chan struct{}
	release chan struct{}
}

func (g *gated) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.arrived <- struct{}{}
	<-g.release
	g.next.ServeHTTP(w, r)
}

func TestRoute_OneAtATimePerLogin(t *testing.T) {
	gate := &gated{next: newFake(), arrived: make(chan struct{}, 64), release: make(chan struct{})}
	var buf bytes.Buffer
	log := &audit.Logger{W: &buf}
	h := setup(t, gate, obssvc.WithAudit(log))
	key := []byte("test-key-0123456789abcdef0123456")
	call := func(login string) (int, string) {
		tok, _, err := auth.Issue(key, login, "dev", time.Now())
		if err != nil {
			t.Error(err)
			return 0, ""
		}
		req := httptest.NewRequest(http.MethodPost, "/obs/route", strings.NewReader(okBody))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		auth.RequireAudited(key, log, func(*http.Request) string { return "/obs/route" }, h.Route()).ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	first := make(chan int, 1)
	go func() { c, _ := call("octocat"); first <- c }()
	select {
	case <-gate.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("first request never reached Grafana")
	}
	if c, raw := call("octocat"); c != http.StatusTooManyRequests || errCode(raw) != api.CodeTooManyConcurrent {
		t.Fatalf("second request = %d %s, want 429 %s", c, raw, api.CodeTooManyConcurrent)
	}
	close(gate.release)
	if c := <-first; c != 200 {
		t.Errorf("first request = %d", c)
	}
	if c, _ := call("octocat"); c != 200 {
		t.Errorf("after release = %d", c)
	}
}

func contains(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

// TestRoute_CapsResponse: 3000 distinct statuses push the body over 32 KB, so
// the error sample goes first, then the slow one, then status entries.
func TestRoute_CapsResponse(t *testing.T) {
	var series []string
	for i := 0; i < 3000; i++ {
		series = append(series, `{"labels":[{"key":"span.http.status_code","value":{"intValue":"`+strconv.Itoa(100000+i)+
			`"}},{"key":"__bucket","value":{"doubleValue":0.5}}],"value":1}`)
	}
	g := newFake()
	g.metrics = `{"series":[` + strings.Join(series, ",") + `]}`
	h := setup(t, g)
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 || len(raw) > 32<<10 {
		t.Fatalf("code %d, %d bytes", code, len(raw))
	}
	if !resp.Truncated || resp.Omitted["errors"] != 1 || resp.Omitted["slowest"] != 1 || resp.Omitted["status"] == 0 {
		t.Errorf("truncated=%v omitted=%v", resp.Truncated, resp.Omitted)
	}
	if resp.Stats == nil || resp.Stats.Count != 36000 || len(resp.Stats.Status)+resp.Omitted["status"] != 3000 {
		t.Errorf("stats count=%d status entries=%d", resp.Stats.Count, len(resp.Stats.Status))
	}
}

// TestRoute_StringStatusLabel: a status label sent as stringValue still counts.
func TestRoute_StringStatusLabel(t *testing.T) {
	g := newFake()
	g.metrics = `{"series":[{"labels":[{"key":"span.http.status_code","value":{"stringValue":"503"}},{"key":"__bucket","value":{"doubleValue":0.5}}],"value":4}]}`
	g.errs = strings.Replace(searchOne, `{"intValue":"500"}`, `{"stringValue":"502"}`, 1)
	h := setup(t, g)
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 || resp.Stats == nil {
		t.Fatalf("code %d: %s", code, raw)
	}
	if resp.Stats.Status["503"] != 48 || resp.Stats.ErrorRatio != 1 {
		t.Errorf("status=%v error_ratio=%v, want {503:48} 1", resp.Stats.Status, resp.Stats.ErrorRatio)
	}
	if len(resp.Samples.Errors) != 1 || resp.Samples.Errors[0].Status != "502" {
		t.Errorf("errors = %+v, want status 502", resp.Samples.Errors)
	}
}

// TestRoute_BudgetCutsHungWindows: hung metric windows end at the request
// budget and count as failed; the slow search, which waits for them, fails
// too. Still a 200 with the stats of the other windows.
func TestRoute_BudgetCutsHungWindows(t *testing.T) {
	const budget = 300 * time.Millisecond
	g := newFake()
	g.hangMetrics = 2
	h := setup(t, g, obssvc.WithBudget(budget))
	t0 := time.Now()
	code, resp, raw := post(h.Route(), okBody)
	took := time.Since(t0)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	// The deadline is the budget itself; allow the time to write the reply.
	if took > budget+200*time.Millisecond {
		t.Errorf("took %v, want about the %v budget", took, budget)
	}
	for _, n := range []string{"2 of 12 metric windows failed", "slow sample search failed"} {
		if !contains(resp.Notes, n) {
			t.Errorf("notes = %v, want %q", resp.Notes, n)
		}
	}
	if resp.Stats == nil || resp.SinceEffective != "20h0m0s" {
		t.Errorf("stats=%+v since_effective=%q", resp.Stats, resp.SinceEffective)
	}
}

// TestRoute_SlowSearchDoesNotWaitForLoki: the slow search starts once the
// metric windows are done, so a hung Loki count and error search do not
// starve it; both end at the budget as failure notes.
func TestRoute_SlowSearchDoesNotWaitForLoki(t *testing.T) {
	const budget = 300 * time.Millisecond
	g := newFake()
	g.hangErrs, g.hangLoki = true, true
	h := setup(t, g, obssvc.WithBudget(budget))
	t0 := time.Now()
	code, resp, raw := post(h.Route(), okBody)
	if code != 200 || time.Since(t0) > budget+200*time.Millisecond {
		t.Fatalf("code %d after %v: %s", code, time.Since(t0), raw)
	}
	if len(resp.Samples.Slowest) != 1 || contains(resp.Notes, "slow sample search failed") {
		t.Errorf("slowest=%+v notes=%v, want the slow sample", resp.Samples.Slowest, resp.Notes)
	}
	if !contains(resp.Notes, "error sample search failed") || !contains(resp.Notes, "loki error count failed") {
		t.Errorf("notes = %v, want the error search and Loki failures", resp.Notes)
	}
}
