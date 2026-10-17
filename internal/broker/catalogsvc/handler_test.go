package catalogsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/config"
)

const goodYAML = "version: 1\nservices:\n  - id: svc-a\n    repo: org/svc-a\n"
const goodYAML2 = "version: 1\nservices:\n  - id: svc-b\n    repo: org/svc-b\n"

// stub is a fake GitHub contents API: it counts calls and serves what the test sets.
type stub struct {
	mu     sync.Mutex
	status int
	body   string
	calls  int32
	last   *http.Request
}

func (s *stub) set(status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status, s.body = status, body
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&s.calls, 1)
	s.mu.Lock()
	status, body := s.status, s.body
	s.last = r
	s.mu.Unlock()
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

type clock struct{ t atomic.Int64 }

func (c *clock) now() time.Time          { return time.Unix(c.t.Load(), 0) }
func (c *clock) advance(d time.Duration) { c.t.Add(int64(d / time.Second)) }

func setup(t *testing.T, status int, body string) (*Handler, *stub, *clock) {
	t.Helper()
	st := &stub{status: status, body: body}
	srv := httptest.NewServer(st)
	t.Cleanup(srv.Close)
	clk := &clock{}
	clk.t.Store(1_000_000)
	h := New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token",
		WithBaseURL(srv.URL), WithClock(clk.now))
	return h, st, clk
}

func post(h http.Handler) (int, api.CatalogResponse, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/catalog", strings.NewReader("{}")))
	var resp api.CatalogResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp, rec.Body.String()
}

func TestCatalog_CachesFiveMinutes(t *testing.T) {
	h, st, clk := setup(t, 200, goodYAML)
	c1, r1, b1 := post(h)
	clk.advance(4 * time.Minute)
	c2, r2, b2 := post(h)
	if c1 != 200 || c2 != 200 || b1 != b2 {
		t.Fatalf("codes %d %d, bodies equal=%v", c1, c2, b1 == b2)
	}
	if n := atomic.LoadInt32(&st.calls); n != 1 {
		t.Fatalf("stub calls = %d, want 1", n)
	}
	if r1.StaleAgeSeconds != 0 || r2.StaleAgeSeconds != 0 {
		t.Fatalf("stale ages %d %d, want 0", r1.StaleAgeSeconds, r2.StaleAgeSeconds)
	}
	if r1.Catalog == nil || r1.Catalog.Services[0].ID != "svc-a" {
		t.Fatalf("catalog = %+v", r1.Catalog)
	}
	clk.advance(2 * time.Minute) // 6 min total: past the TTL
	post(h)
	if n := atomic.LoadInt32(&st.calls); n != 2 {
		t.Fatalf("stub calls after TTL = %d, want 2", n)
	}
}

func TestCatalog_RequestShape(t *testing.T) {
	h, st, _ := setup(t, 200, goodYAML)
	post(h)
	r := st.last
	if r.Method != http.MethodGet || r.URL.Path != "/repos/org/monitoring/contents/catalog.yaml" || r.URL.Query().Get("ref") != "main" {
		t.Fatalf("request = %s %s", r.Method, r.URL.String())
	}
	if r.Header.Get("Accept") != "application/vnd.github.raw" || r.Header.Get("Authorization") != "Bearer ghp_secret_token" {
		t.Fatalf("headers = %v", r.Header)
	}
}

func TestCatalog_ServesStaleOnError(t *testing.T) {
	h, st, clk := setup(t, 200, goodYAML)
	post(h)
	clk.advance(6 * time.Minute)
	st.set(500, "boom")
	code, resp, _ := post(h)
	if code != 200 || resp.StaleAgeSeconds != 360 || resp.Catalog.Services[0].ID != "svc-a" {
		t.Fatalf("code %d age %d catalog %+v", code, resp.StaleAgeSeconds, resp.Catalog)
	}
}

func TestCatalog_BadYAMLNotCached(t *testing.T) {
	h, st, clk := setup(t, 200, goodYAML)
	post(h)
	clk.advance(6 * time.Minute)
	st.set(200, "version: [unclosed")
	code, resp, _ := post(h)
	if code != 200 || resp.StaleAgeSeconds != 360 || resp.Catalog.Services[0].ID != "svc-a" {
		t.Fatalf("bad yaml: code %d age %d catalog %+v", code, resp.StaleAgeSeconds, resp.Catalog)
	}
	st.set(200, goodYAML2)
	clk.advance(retryInterval) // a failure holds off the next GitHub call for retryInterval
	code, resp, _ = post(h)
	if code != 200 || resp.StaleAgeSeconds != 0 || resp.Catalog.Services[0].ID != "svc-b" {
		t.Fatalf("recovery: code %d age %d catalog %+v", code, resp.StaleAgeSeconds, resp.Catalog)
	}
}

func TestCatalog_503WhenNeverFetched(t *testing.T) {
	h, _, _ := setup(t, 500, "boom")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/catalog", strings.NewReader("{}")))
	var e api.Error
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if rec.Code != 503 || e.Error != api.CodeCatalogUnavailable {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "ghp_secret_token") {
		t.Fatal("error leaks the token")
	}
}

func TestCatalog_ConcurrentCurrent(t *testing.T) {
	h, st, _ := setup(t, 200, goodYAML)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, err := h.Current(context.Background())
			if err != nil || c == nil || c.Services[0].ID != "svc-a" {
				t.Errorf("Current = %+v, %v", c, err)
			}
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&st.calls); n != 1 {
		t.Fatalf("stub calls = %d, want 1", n)
	}
}

func TestCatalog_AuditsEveryRequest(t *testing.T) {
	st := &stub{status: 200, body: goodYAML}
	srv := httptest.NewServer(st)
	t.Cleanup(srv.Close)
	var buf bytes.Buffer
	h := New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token",
		WithBaseURL(srv.URL), WithAudit(&audit.Logger{W: &buf}), WithTTL(0))
	key := []byte("test-key-0123456789abcdef0123456")
	tok, _, err := auth.Issue(key, "octocat", "dev", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	call := func(h http.Handler) int {
		req := httptest.NewRequest(http.MethodPost, "/catalog", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		auth.RequireAudited(key, &audit.Logger{W: &buf}, func(*http.Request) string { return "/catalog" }, h).ServeHTTP(rec, req)
		return rec.Code
	}
	if c := call(h); c != 200 {
		t.Fatalf("first call = %d", c)
	}
	// A fresh handler that has never fetched: 503.
	st.set(500, "boom")
	h2 := New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token",
		WithBaseURL(srv.URL), WithAudit(&audit.Logger{W: &buf}))
	if c := call(h2); c != 503 {
		t.Fatalf("second call = %d", c)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("audit rows = %d, want 2: %v", len(lines), lines)
	}
	for i, want := range []int{200, 503} {
		var e map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			t.Fatal(err)
		}
		if e["endpoint"] != "/catalog" || e["login"] != "octocat" || e["role"] != "dev" || e["status"] != float64(want) {
			t.Errorf("row %d = %s, want status %d", i, lines[i], want)
		}
	}
	if strings.Contains(buf.String(), "ghp_secret_token") {
		t.Error("audit leaks the token")
	}
}

// TestCatalog_RetryBackoffAfterFailure: after a failed refresh with a stale
// copy, callers get the stale copy without asking GitHub until 30s have passed.
func TestCatalog_RetryBackoffAfterFailure(t *testing.T) {
	h, st, clk := setup(t, 200, goodYAML)
	post(h)
	clk.advance(6 * time.Minute)
	st.set(500, "boom")
	calls := func() int32 { return atomic.LoadInt32(&st.calls) }
	c, age, err := h.Current(context.Background())
	if err != nil || c.Services[0].ID != "svc-a" || age != 360 || calls() != 2 {
		t.Fatalf("first after TTL: age %d err %v calls %d, want 360 nil 2", age, err, calls())
	}
	for _, d := range []time.Duration{0, 10 * time.Second, 19 * time.Second} { // 0s, 10s, 29s after the failure
		clk.advance(d)
		c, age, err = h.Current(context.Background())
		if err != nil || c.Services[0].ID != "svc-a" || calls() != 2 {
			t.Fatalf("within %s: err %v calls %d, want nil 2", retryInterval, err, calls())
		}
	}
	if age != 389 {
		t.Fatalf("age at 29s after the failure = %d, want 389", age)
	}
	clk.advance(time.Second) // 30s after the failure: retry
	if _, age, _ = h.Current(context.Background()); calls() != 3 || age != 390 {
		t.Fatalf("at %s: calls %d age %d, want 3 390", retryInterval, calls(), age)
	}
	st.set(200, goodYAML2)
	clk.advance(retryInterval)
	if c, age, _ = h.Current(context.Background()); calls() != 4 || age != 0 || c.Services[0].ID != "svc-b" {
		t.Fatalf("recovery: calls %d age %d catalog %+v", calls(), age, c)
	}
}

// TestCatalog_ColdStartBackoff: with no copy to serve, a failure is returned
// at once for retryInterval without asking GitHub, then GitHub is asked again.
func TestCatalog_ColdStartBackoff(t *testing.T) {
	h, st, clk := setup(t, 500, "boom")
	calls := func() int32 { return atomic.LoadInt32(&st.calls) }
	_, _, first := h.Current(context.Background())
	if first == nil || calls() != 1 {
		t.Fatalf("first: err %v calls %d, want error and 1", first, calls())
	}
	for _, d := range []time.Duration{0, 29 * time.Second} {
		clk.advance(d)
		if _, _, err := h.Current(context.Background()); err == nil || err.Error() != first.Error() || calls() != 1 {
			t.Fatalf("within %s: err %v calls %d, want %q and 1", retryInterval, err, calls(), first)
		}
	}
	code, _, raw := post(h)
	if code != 503 || !strings.Contains(raw, api.CodeCatalogUnavailable) || strings.Contains(raw, "ghp_secret_token") || calls() != 1 {
		t.Fatalf("ServeHTTP within hold-off: %d %s calls %d", code, raw, calls())
	}
	clk.advance(time.Second) // 30s after the failure
	if _, _, err := h.Current(context.Background()); err == nil || calls() != 2 {
		t.Fatalf("at %s: err %v calls %d, want error and 2", retryInterval, err, calls())
	}
	st.set(200, goodYAML)
	clk.advance(retryInterval)
	if c, age, err := h.Current(context.Background()); err != nil || age != 0 || c.Services[0].ID != "svc-a" || calls() != 3 {
		t.Fatalf("recovery: %+v %d %v calls %d", c, age, err, calls())
	}
}

func TestCatalog_ConcurrentColdStartAsksOnce(t *testing.T) {
	h, st, _ := setup(t, 500, "boom")
	var wg sync.WaitGroup
	codes := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, _, _ := post(h); codes <- c }()
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 503 {
			t.Errorf("code = %d, want 503", c)
		}
	}
	if n := atomic.LoadInt32(&st.calls); n != 1 {
		t.Fatalf("stub calls = %d, want 1", n)
	}
}

// TestCatalog_HoldOffCountsFromFetchEnd: lastFailure is taken after the
// fetch returns, so a slow failing fetch still gets the full hold-off.
func TestCatalog_HoldOffCountsFromFetchEnd(t *testing.T) {
	clk := &clock{}
	clk.t.Store(1_000_000)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		clk.advance(10 * time.Second) // the fetch "takes" 10s
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	h := New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token",
		WithBaseURL(srv.URL), WithClock(clk.now))
	_, _, _ = h.Current(context.Background())
	clk.advance(25 * time.Second) // 35s after the fetch began, 25s after it failed
	_, _, _ = h.Current(context.Background())
	if n := calls.Load(); n != 1 {
		t.Fatalf("calls = %d, want 1 (hold-off counts from the end of the fetch)", n)
	}
}

func TestCatalog_CancelledCallerDoesNotHoldOff(t *testing.T) {
	h, st, _ := setup(t, 200, goodYAML)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := h.Current(ctx); err == nil {
		t.Fatal("want error for a cancelled caller")
	}
	if c, _, err := h.Current(context.Background()); err != nil || c.Services[0].ID != "svc-a" {
		t.Fatalf("next caller: %+v %v", c, err)
	}
	if n := atomic.LoadInt32(&st.calls); n != 1 {
		t.Fatalf("stub calls = %d, want 1", n)
	}
}

func TestCatalog_ConcurrentFailingRefreshAsksOnce(t *testing.T) {
	h, st, clk := setup(t, 200, goodYAML)
	post(h)
	clk.advance(6 * time.Minute)
	st.set(500, "boom")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, age, err := h.Current(context.Background())
			if err != nil || c == nil || c.Services[0].ID != "svc-a" || age != 360 {
				t.Errorf("Current = %+v, %d, %v", c, age, err)
			}
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&st.calls); n != 2 { // 1 prime + 1 failed refresh
		t.Fatalf("stub calls = %d, want 2", n)
	}
}

func TestCatalog_OversizeFileRefused(t *testing.T) {
	big := goodYAML + "# " + strings.Repeat("x", maxFileBytes) + "\n"
	// Never fetched: 503.
	h, _, _ := setup(t, 200, big)
	code, _, raw := post(h)
	if code != 503 || !strings.Contains(raw, "over") {
		t.Fatalf("never fetched: got %d %s", code, raw)
	}
	// With a stale copy: the stale copy.
	h, st, clk := setup(t, 200, goodYAML)
	post(h)
	clk.advance(6 * time.Minute)
	st.set(200, big)
	code, resp, _ := post(h)
	if code != 200 || resp.StaleAgeSeconds != 360 || resp.Catalog.Services[0].ID != "svc-a" {
		t.Fatalf("stale: code %d age %d", code, resp.StaleAgeSeconds)
	}
	// Exactly maxFileBytes is accepted.
	exact := goodYAML + "#" + strings.Repeat("x", maxFileBytes-len(goodYAML)-1)
	h, _, _ = setup(t, 200, exact)
	if code, _, raw := post(h); code != 200 {
		t.Fatalf("exactly %d bytes: got %d %s", maxFileBytes, code, raw)
	}
}
