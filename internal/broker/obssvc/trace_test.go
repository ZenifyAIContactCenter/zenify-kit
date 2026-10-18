package obssvc_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/obssvc"
)

const (
	tracePrefix = "/api/datasources/proxy/uid/uid-tempo/api/v2/traces/"
	lokiRange   = "/api/datasources/proxy/uid/uid-loki/loki/api/v1/query_range"
	// id29 is a 29-char exemplar id; pad29 is it left-padded to 32.
	id29  = "abcdef0123456789abcdef0123456"
	pad29 = "000abcdef0123456789abcdef0123456"
	// fixtureStartNS / fixtureEndNS bound trace-109.json: the root span runs
	// 2026-10-17T23:50:00Z for 800 ms and every other span ends before it.
	fixtureStartNS = "1792281000000000000"
	fixtureEndNS   = "1792281000800000000"
)

// fakeTempo answers the trace fetch and the Loki range query by path and
// records what it was asked.
type fakeTempo struct {
	mu          sync.Mutex
	trace       string
	traceStatus int
	loki        string
	lokiHang    bool // the Loki query hangs until the caller gives up
	tracePaths  []string
	lokiQueries []url.Values
}

func (f *fakeTempo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.HasPrefix(r.URL.Path, tracePrefix):
		f.tracePaths = append(f.tracePaths, r.URL.Path)
		if f.traceStatus != 0 {
			w.WriteHeader(f.traceStatus)
			return
		}
		_, _ = w.Write([]byte(f.trace))
	case r.URL.Path == lokiRange:
		f.lokiQueries = append(f.lokiQueries, r.URL.Query())
		if f.lokiHang {
			f.mu.Unlock()
			hang(r)
			f.mu.Lock()
			return
		}
		_, _ = w.Write([]byte(f.loki))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeTempo) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tracePaths) + len(f.lokiQueries)
}

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/trace-109.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const emptyLoki = `{"status":"success","data":{"resultType":"streams","result":[]}}`

func newTempo(t *testing.T) *fakeTempo {
	return &fakeTempo{trace: fixture(t), loki: emptyLoki}
}

func postTrace(h http.Handler, body string) (int, api.ObsTraceResponse, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/obs/trace", strings.NewReader(body)))
	var resp api.ObsTraceResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp, rec.Body.String()
}

const traceBody = `{"trace_id":"` + id29 + `","env":"prd"}`

func TestTrace_Summary(t *testing.T) {
	var buf bytes.Buffer
	f := newTempo(t)
	code, resp, raw := postTrace(setup(t, f, obssvc.WithAudit(&audit.Logger{W: &buf})).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if resp.TraceID != pad29 || resp.SpanCount != 109 {
		t.Errorf("trace_id=%q span_count=%d, want %q 109", resp.TraceID, resp.SpanCount, pad29)
	}
	// The fixture has 25 (service, name) pairs: the root plus 12 names per service.
	if len(resp.Groups) != 25 {
		t.Errorf("groups = %d, want 25", len(resp.Groups))
	}
	sum := 0
	for i, g := range resp.Groups {
		sum += g.Count
		if i > 0 && g.TotalMS > resp.Groups[i-1].TotalMS {
			t.Errorf("groups not sorted by total_ms desc at %d", i)
		}
	}
	if sum != 109 {
		t.Errorf("sum of group counts = %d, want 109", sum)
	}
	if !reflect.DeepEqual(resp.Services, []string{"svc-a", "svc-b"}) {
		t.Errorf("services = %v", resp.Services)
	}
	want := api.ObsSpanRef{Service: "svc-a", Name: "GET /orders/:id", Start: "2026-10-17T23:50:00Z", DurationMS: 800}
	if resp.Root != want {
		t.Errorf("root = %+v, want %+v", resp.Root, want)
	}
	// Two failed spans: svc-b "POST /charge" (status error) and svc-b
	// "GET /quote" (http.status_code 503), in start order.
	if len(resp.Errors) != 2 || resp.Errors[0].Name != "POST /charge" || resp.Errors[1].Name != "GET /quote" {
		t.Errorf("errors = %+v", resp.Errors)
	}
	for _, g := range resp.Groups {
		wantErr := 0
		if g.Service == "svc-b" && (g.Name == "POST /charge" || g.Name == "GET /quote") {
			wantErr = 1
		}
		if g.Errors != wantErr {
			t.Errorf("group %s/%s errors = %d, want %d", g.Service, g.Name, g.Errors, wantErr)
		}
	}
	if resp.Truncated || len(resp.Notes) != 0 {
		t.Errorf("truncated=%v notes=%v", resp.Truncated, resp.Notes)
	}
	var row map[string]any
	if err := json.Unmarshal(buf.Bytes(), &row); err != nil {
		t.Fatalf("audit row: %v (%q)", err, buf.String())
	}
	args, _ := row["args"].(map[string]any)
	if row["endpoint"] != "/obs/trace" || row["op"] != "trace" || args["trace_id"] != pad29 {
		t.Errorf("audit row = %v", row)
	}
}

func TestTrace_Allowlist(t *testing.T) {
	code, resp, raw := postTrace(setup(t, newTempo(t)).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	// FR-3.4 allowlist plus the catalog's tenant.id.
	ok := map[string]bool{"http.status_code": true, "exception.type": true, "exception.message": true, "tenant.id": true}
	for _, e := range resp.Errors {
		for k := range e.Attributes {
			if !ok[k] {
				t.Errorf("%s/%s carries attribute %q", e.Service, e.Name, k)
			}
		}
		if e.Attributes["tenant.id"] != "t-1" {
			t.Errorf("%s tenant.id = %q, want t-1", e.Name, e.Attributes["tenant.id"])
		}
	}
	if len(resp.Errors) == 0 || resp.Errors[0].Attributes["exception.message"] != "card declined for <email>" {
		t.Errorf("errors = %+v", resp.Errors)
	}
	for _, leak := range []string{"db.connection_string", "user.id", "mongodb://", "jane.doe"} {
		if strings.Contains(raw, leak) {
			t.Errorf("response contains %q", leak)
		}
	}
}

func TestTrace_PadsID(t *testing.T) {
	f := newTempo(t)
	if code, _, raw := postTrace(setup(t, f).Trace(), traceBody); code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if len(f.tracePaths) != 1 || f.tracePaths[0] != tracePrefix+pad29 {
		t.Errorf("trace paths = %v, want one %s%s", f.tracePaths, tracePrefix, pad29)
	}
}

func TestTrace_NotFound(t *testing.T) {
	f := newTempo(t)
	f.traceStatus = 404
	code, _, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 404 || errCode(raw) != api.CodeTraceNotFound || !strings.Contains(raw, "retention") {
		t.Errorf("code %d: %s", code, raw)
	}
}

func TestTrace_EmptyIsNotFound(t *testing.T) {
	f := newTempo(t)
	f.trace = `{"trace":{"resourceSpans":[]}}`
	code, _, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 404 || errCode(raw) != api.CodeTraceNotFound {
		t.Errorf("code %d: %s", code, raw)
	}
}

func TestTrace_BadID(t *testing.T) {
	f := newTempo(t)
	code, _, raw := postTrace(setup(t, f).Trace(), `{"trace_id":"xyz","env":"prd"}`)
	if code != 400 || errCode(raw) != api.CodeBadTraceID {
		t.Errorf("code %d: %s", code, raw)
	}
	if n := f.calls(); n != 0 {
		t.Errorf("grafana calls = %d, want 0", n)
	}
}

func TestTrace_TooLarge(t *testing.T) {
	code, _, raw := postTrace(setup(t, newTempo(t), obssvc.WithTraceMaxBytes(1024)).Trace(), traceBody)
	if code != 422 || errCode(raw) != api.CodeTraceTooLarge {
		t.Errorf("code %d: %s", code, raw)
	}
}

func TestTrace_NoRoot(t *testing.T) {
	f := newTempo(t)
	var kept []string
	for _, l := range strings.Split(f.trace, "\n") {
		if !strings.Contains(l, `"spanId":"s000"`) {
			kept = append(kept, l)
		}
	}
	f.trace = strings.Join(kept, "\n")
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	// Without s000 the earliest span is svc-a "mongo find orders" at +1 ms, 1 ms long.
	want := api.ObsSpanRef{Service: "svc-a", Name: "mongo find orders", Start: "2026-10-17T23:50:00.001Z", DurationMS: 1}
	if resp.Root != want || resp.SpanCount != 108 {
		t.Errorf("root = %+v span_count = %d, want %+v 108", resp.Root, resp.SpanCount, want)
	}
	if !reflect.DeepEqual(resp.Notes, []string{"no root span"}) {
		t.Errorf("notes = %v", resp.Notes)
	}
}

func TestTrace_LogsMappedOnly(t *testing.T) {
	f := newTempo(t)
	f.loki = `{"status":"success","data":{"resultType":"streams","result":[
{"stream":{"app":"app-a"},"values":[["1792281000002000000","second for bob@example.com"]]},
{"stream":{"app":"app-a"},"values":[["1792281000001000000","first"]]}]}}`
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if !reflect.DeepEqual(resp.Logs.UnmappedServices, []string{"svc-b"}) {
		t.Errorf("unmapped = %v, want [svc-b]", resp.Logs.UnmappedServices)
	}
	if len(f.lokiQueries) != 1 {
		t.Fatalf("loki calls = %d, want 1", len(f.lokiQueries))
	}
	q := f.lokiQueries[0]
	if got := q.Get("query"); got != `{app=~"app-a"} |~ "`+pad29+`|`+id29+`"` || strings.Contains(got, `=~".+"`) {
		t.Errorf("loki query = %q", got)
	}
	// start = trace start - 2m, end = trace end + 2m, in epoch ns.
	if q.Get("start") != "1792280880000000000" || q.Get("end") != "1792281120800000000" ||
		q.Get("limit") != "50" || q.Get("direction") != "forward" {
		t.Errorf("loki params = %v (trace %s..%s)", q, fixtureStartNS, fixtureEndNS)
	}
	want := []api.ObsLogLine{
		{TS: "2026-10-17T23:50:00.001Z", App: "app-a", Line: "first"},
		{TS: "2026-10-17T23:50:00.002Z", App: "app-a", Line: "second for <email>"},
	}
	if !reflect.DeepEqual(resp.Logs.Lines, want) {
		t.Errorf("lines = %+v", resp.Logs.Lines)
	}
}

func TestTrace_NoAppsSkipsLokiAndMasksNames(t *testing.T) {
	f := newTempo(t)
	f.trace = `{"trace":{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-b"}}]},
"scopeSpans":[{"spans":[{"spanId":"r","parentSpanId":"","name":"mail to ann@example.com","startTimeUnixNano":"1792281000000000000","endTimeUnixNano":"1792281000005000000",
"status":{"code":"STATUS_CODE_ERROR"},"attributes":[{"key":"db.statement","value":{"stringValue":"find ann@example.com ` + strings.Repeat("z", 400) + `"}}]}]}]}]}}`
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if len(f.lokiQueries) != 0 {
		t.Errorf("loki calls = %d, want 0", len(f.lokiQueries))
	}
	if resp.Root.Name != "mail to <email>" || len(resp.Groups) != 1 || resp.Groups[0].Name != "mail to <email>" {
		t.Errorf("root = %+v groups = %+v", resp.Root, resp.Groups)
	}
	// db.statement is masked, then cut to 300 B.
	if len(resp.Errors) != 1 || len(resp.Errors[0].Attributes["db.statement"]) != 300 ||
		!strings.HasPrefix(resp.Errors[0].Attributes["db.statement"], "find <email> zzz") {
		t.Errorf("errors = %+v", resp.Errors)
	}
	if !reflect.DeepEqual(resp.Notes, []string{"no loki app"}) || resp.Logs.Lines == nil {
		t.Errorf("notes = %v lines = %v", resp.Notes, resp.Logs.Lines)
	}
}

func TestTrace_SizeCap(t *testing.T) {
	f := newTempo(t)
	// 50 lines of 2 KB of quoted JSON: each keeps 500 B after truncation and
	// grows by its escaped quotes, so the lines alone exceed 32 KB.
	line := strings.Repeat(`{"k":"v"},`, 205)
	vals := make([]string, 50)
	for i := range vals {
		b, _ := json.Marshal([]string{strconv.Itoa(1792281000000000000 + i*1000), line})
		vals[i] = string(b)
	}
	f.loki = `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"app":"app-a"},"values":[` +
		strings.Join(vals, ",") + `]}]}}`
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if len(raw) > 32768 {
		t.Errorf("body = %d B, want <= 32768", len(raw))
	}
	if !resp.Truncated || resp.Omitted["log_lines"] == 0 {
		t.Errorf("truncated=%v omitted=%v", resp.Truncated, resp.Omitted)
	}
	if resp.Omitted["errors"] != 0 || resp.Omitted["groups"] != 0 {
		t.Errorf("omitted = %v, want only log_lines", resp.Omitted)
	}
	for _, l := range resp.Logs.Lines {
		if len(l.Line) > 500 {
			t.Errorf("line = %d B, want <= 500", len(l.Line))
		}
	}
	if kept := len(resp.Logs.Lines); kept+resp.Omitted["log_lines"] != 50 {
		t.Errorf("kept %d + omitted %d != 50", kept, resp.Omitted["log_lines"])
	}
	ts := func(i int) time.Time {
		v, err := time.Parse(time.RFC3339Nano, resp.Logs.Lines[i].TS)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if !sort.SliceIsSorted(resp.Logs.Lines, func(i, j int) bool { return ts(i).Before(ts(j)) }) {
		t.Error("lines not sorted by ts")
	}
}

func TestTrace_CapsErrorsThenGroups(t *testing.T) {
	f := newTempo(t)
	// 40 failed svc-b spans with distinct names, each with 8 allowlisted 4 KB
	// attributes cut to 500 B: 30 groups and 10 error spans survive the fixed
	// caps, and 10 x 8 x 500 B is over 32 KB, so error spans are shed before
	// any group.
	msg := strings.Repeat("x", 4096)
	var attrs []string
	for _, k := range []string{"exception.message", "exception.type", "otel.status_description", "http.route",
		"rpc.service", "rpc.method", "messaging.destination.name", "db.name"} {
		attrs = append(attrs, `{"key":"`+k+`","value":{"stringValue":"`+msg+`"}}`)
	}
	spans := make([]string, 40)
	for i := range spans {
		spans[i] = `{"spanId":"s` + strconv.Itoa(i) + `","parentSpanId":"p","name":"op-` + strconv.Itoa(i) +
			`","startTimeUnixNano":"` + strconv.Itoa(1792281000000000000+i*1000000) + `","endTimeUnixNano":"` +
			strconv.Itoa(1792281000000000000+i*1000000+(i+1)*1000) + `","status":{"code":"STATUS_CODE_ERROR"},` +
			`"attributes":[` + strings.Join(attrs, ",") + `]}`
	}
	f.trace = `{"trace":{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-b"}}]},` +
		`"scopeSpans":[{"spans":[` + strings.Join(spans, ",") + `]}]}]}}`
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if len(raw) > 32768 || !resp.Truncated || resp.SpanCount != 40 {
		t.Errorf("body=%d truncated=%v span_count=%d", len(raw), resp.Truncated, resp.SpanCount)
	}
	sum := 0
	for _, g := range resp.Groups {
		sum += g.Count
	}
	if len(resp.Groups) != 30 || sum != 30 || resp.Omitted["groups"] != 0 {
		t.Errorf("groups=%d sum=%d omitted=%v", len(resp.Groups), sum, resp.Omitted)
	}
	if resp.Omitted["errors"] == 0 || len(resp.Errors)+resp.Omitted["errors"] != 10 {
		t.Errorf("errors=%d omitted=%v, want 10 in all with some omitted", len(resp.Errors), resp.Omitted)
	}
}

// errSpanTrace is one failed svc-b root span carrying attrs (JSON list body),
// wrapped with the given extra top-level fields.
func errSpanTrace(attrs, extra string) string {
	return `{"trace":{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-b"}}]},` +
		`"scopeSpans":[{"spans":[{"spanId":"r","parentSpanId":"","name":"op","startTimeUnixNano":"1792281000000000000",` +
		`"endTimeUnixNano":"1792281000005000000","status":{"code":"STATUS_CODE_ERROR"},"attributes":[` + attrs + `]}]}]}]}` + extra + `}`
}

func TestTrace_PartialTrace(t *testing.T) {
	f := newTempo(t)
	f.trace = errSpanTrace(``, `,"status":"partial","message":"trace exceeds max size, ops@example.com"`)
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if !resp.Truncated || !contains(resp.Notes, "tempo returned a partial trace: trace exceeds max size, <email>") {
		t.Errorf("truncated=%v notes=%v", resp.Truncated, resp.Notes)
	}
	f.trace = errSpanTrace(``, `,"status":"complete"`)
	if _, resp, _ := postTrace(setup(t, f).Trace(), traceBody); resp.Truncated || contains(resp.Notes, "partial") {
		t.Errorf("complete: truncated=%v notes=%v", resp.Truncated, resp.Notes)
	}
}

// TestTrace_HugeAttributeCut: a 100 KB exception.message is cut to 500 B, so
// the error span survives the 32 KB cap.
func TestTrace_HugeAttributeCut(t *testing.T) {
	f := newTempo(t)
	f.trace = errSpanTrace(`{"key":"exception.message","value":{"stringValue":"`+strings.Repeat("y", 100<<10)+`"}}`, ``)
	code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
	if code != 200 || len(raw) > 32768 {
		t.Fatalf("code %d, %d bytes", code, len(raw))
	}
	if len(resp.Errors) != 1 || resp.Omitted["errors"] != 0 {
		t.Fatalf("errors=%d omitted=%v, want the error span kept", len(resp.Errors), resp.Omitted)
	}
	if n := len(resp.Errors[0].Attributes["exception.message"]); n == 0 || n > 500 {
		t.Errorf("exception.message = %d B, want 1-500", n)
	}
}

// TestTrace_LogLinePreCut: only the first 1024 B of a line are masked and
// kept, so text past that never reaches the reply even when masking shortens
// the head.
// A token straddling the 1024 B cut must not survive as an unmasked fragment.
func TestTrace_LogLinePreCut(t *testing.T) {
	cases := []struct {
		name, line string
		leaks      []string
	}{
		// 23 chunks of 43 B end at 989; the cut lands in "@exam|ple.com"
		{"email", strings.Repeat(strings.Repeat("a", 30)+"@example.com ", 30) + "TAIL", []string{"TAIL", "example.com", "@exam"}},
		// 1018 B that mask to under 500, then "0912 345 678": the cut lands in "0912 3|45 678"
		{"phone", strings.Repeat(strings.Repeat("a", 30)+"@example.com ", 23) + strings.Repeat("w", 26) + " x 0912 345 678 TAIL", []string{"TAIL", "0912"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newTempo(t)
			b, _ := json.Marshal(c.line)
			f.loki = `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"app":"app-a"},"values":[["1792281000001000000",` +
				string(b) + `]]}]}}`
			code, resp, raw := postTrace(setup(t, f).Trace(), traceBody)
			if code != 200 || len(resp.Logs.Lines) != 1 {
				t.Fatalf("code %d: %s", code, raw)
			}
			l := resp.Logs.Lines[0].Line
			for _, leak := range c.leaks {
				if strings.Contains(l, leak) {
					t.Errorf("line contains %q: %q", leak, l)
				}
			}
			if len(l) > 500 {
				t.Errorf("len %d > 500", len(l))
			}
		})
	}
}

// TestTrace_BudgetCutsLoki: a hung Loki query is cut at min(15 s, budget-5 s)
// and becomes the Loki note; the summary is still returned.
func TestTrace_BudgetCutsLoki(t *testing.T) {
	const budget = 5*time.Second + 300*time.Millisecond
	f := newTempo(t)
	f.lokiHang = true
	t0 := time.Now()
	code, resp, raw := postTrace(setup(t, f, obssvc.WithBudget(budget)).Trace(), traceBody)
	took := time.Since(t0)
	if code != 200 || resp.SpanCount != 109 {
		t.Fatalf("code %d: %s", code, raw)
	}
	if took > budget-4*time.Second {
		t.Errorf("took %v, want about 300ms (budget %v minus the 5 s reserve)", took, budget)
	}
	if !contains(resp.Notes, "loki query failed") {
		t.Errorf("notes = %v", resp.Notes)
	}
}
