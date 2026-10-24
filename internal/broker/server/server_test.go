package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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

const ghToken = "gho_fake-test-token"

var (
	testKey = []byte("test-key-0123456789abcdef0123456")
	t0      = time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
)

// fakeGitHub: ghToken is octocat; membership in "test-org" has the given state
// and octocat is an active member of team "leaders".
func fakeGitHub(t *testing.T, state string, calls *int32) *httptest.Server {
	t.Helper()
	return fakeGitHubTeams(t, "octocat", state, map[string]string{"leaders": "active"}, calls)
}

// fakeGitHubTeams: ghToken is login; org membership has state; teams maps a
// team slug to login's membership state there (absent = 404).
func fakeGitHubTeams(t *testing.T, login, state string, teams map[string]string, calls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		if r.Header.Get("Authorization") != "Bearer "+ghToken {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{"login":"` + login + `"}`))
		case "/user/memberships/orgs/test-org":
			_, _ = w.Write([]byte(`{"state":"` + state + `"}`))
		default:
			team, ok := strings.CutPrefix(r.URL.Path, "/orgs/test-org/teams/")
			team, ok2 := strings.CutSuffix(team, "/memberships/"+login)
			if st, in := teams[team]; ok && ok2 && in {
				_, _ = w.Write([]byte(`{"state":"` + st + `","role":"member"}`))
				return
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newServer(gh *httptest.Server, out *bytes.Buffer) http.Handler {
	return New(Deps{
		JWTKey:     testKey,
		Org:        "test-org",
		LeadTeam:   "leaders",
		DevTeam:    "zenify-team",
		DenyLogins: []string{"zenifydevteam", "ZenifyDeveloper"},
		GitHub:     auth.GitHub{BaseURL: gh.URL, HTTP: gh.Client()},
		Audit:      &audit.Logger{W: out},
		Now:        func() time.Time { return t0 },
	})
}

func postToken(h http.Handler, ghTok, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"github_token":"`+ghTok+`"}`))
	if ip != "" {
		req.Header.Set("X-Real-IP", ip)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return e["error"]
}

func TestHealthz(t *testing.T) {
	var out bytes.Buffer
	rec := httptest.NewRecorder()
	newServer(fakeGitHub(t, "active", nil), &out).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestToken_Issues(t *testing.T) {
	var out bytes.Buffer
	h := newServer(fakeGitHub(t, "active", nil), &out)
	rec := postToken(h, ghToken, "203.0.113.7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["login"] != "octocat" || resp["expires_at"] != "2026-10-15T08:15:00Z" {
		t.Errorf("resp = %v", resp)
	}
	login, role, err := auth.Verify(testKey, resp["token"], t0.Add(time.Minute))
	if err != nil || login != "octocat" || role != api.RoleLead || resp["role"] != api.RoleLead {
		t.Errorf("Verify = %q %q, %v; resp role %q", login, role, err, resp["role"])
	}
}

func TestToken_NotMember403(t *testing.T) {
	var out bytes.Buffer
	h := newServer(fakeGitHub(t, "pending", nil), &out)
	rec := postToken(h, ghToken, "203.0.113.7")
	if rec.Code != http.StatusForbidden || errCode(t, rec) != api.CodeNotOrgMember {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestToken_BadGitHubToken401(t *testing.T) {
	var out bytes.Buffer
	h := newServer(fakeGitHub(t, "active", nil), &out)
	for _, tok := range []string{"gho_wrong", ""} {
		rec := postToken(h, tok, "203.0.113.7")
		if rec.Code != http.StatusUnauthorized || errCode(t, rec) != api.CodeGitHubTokenInvalid {
			t.Errorf("token %q: got %d %s", tok, rec.Code, rec.Body.String())
		}
	}
}

// The bucket key is X-Real-IP (set by the TLS proxy); a forged
// CF-Connecting-IP no longer picks a fresh bucket.
func TestToken_RateLimitIgnoresCFConnectingIP(t *testing.T) {
	var out bytes.Buffer
	h := newServer(fakeGitHub(t, "active", nil), &out)
	var rec *httptest.ResponseRecorder
	for i := 1; i <= 31; i++ {
		req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"github_token":"`+ghToken+`"}`))
		req.Header.Set("X-Real-IP", "198.51.100.9")
		req.Header.Set("CF-Connecting-IP", fmt.Sprintf("203.0.113.%d", i))
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("31st with rotating CF-Connecting-IP: got %d, want 429", rec.Code)
	}
}

func TestToken_RateLimitPerClientIP(t *testing.T) {
	var out bytes.Buffer
	var calls int32
	h := newServer(fakeGitHub(t, "active", &calls), &out)
	for i := 1; i <= 30; i++ {
		if rec := postToken(h, ghToken, "198.51.100.1"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, rec.Code)
		}
	}
	before := atomic.LoadInt32(&calls)
	rec := postToken(h, ghToken, "198.51.100.1")
	if rec.Code != http.StatusTooManyRequests || errCode(t, rec) != api.CodeRateLimited {
		t.Errorf("31st: got %d %s", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&calls) != before {
		t.Error("rate-limited request still reached GitHub")
	}
	if rec := postToken(h, ghToken, "198.51.100.2"); rec.Code != http.StatusOK {
		t.Errorf("other IP: status %d", rec.Code)
	}
}

func TestToken_RateLimitFallsBackToRemoteAddr(t *testing.T) {
	var out bytes.Buffer
	h := newServer(fakeGitHub(t, "active", nil), &out)
	send := func(remote string) int {
		req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"github_token":"`+ghToken+`"}`))
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 30; i++ {
		send("192.0.2.9:" + strconv.Itoa(40000+i)) // same host, varying port
	}
	if code := send("192.0.2.9:4444"); code != http.StatusTooManyRequests {
		t.Errorf("31st from same host: %d, want 429", code)
	}
	if code := send("192.0.2.10:4444"); code != http.StatusOK {
		t.Errorf("other host: %d, want 200", code)
	}
}

func TestAudit_NoSecrets(t *testing.T) {
	var out bytes.Buffer
	h := newServer(fakeGitHub(t, "active", nil), &out)
	rec := postToken(h, ghToken, "203.0.113.7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp api.TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("resp %s", rec.Body.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("audit lines = %d: %q", len(lines), out.String())
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("not JSON: %s", lines[0])
	}
	if e["endpoint"] != "/token" || e["login"] != "octocat" || e["status"] != float64(200) {
		t.Errorf("entry = %s", lines[0])
	}
	if strings.Contains(out.String(), ghToken) || strings.Contains(out.String(), resp.Token) {
		t.Errorf("audit line leaks a secret: %s", lines[0])
	}
}

// codeGitHub answers /user as octocat and the membership endpoint with code.
func codeGitHub(t *testing.T, code int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
			return
		}
		http.Error(w, "{}", code)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestToken_GitHubFailureMapping(t *testing.T) {
	cases := []struct {
		ghCode, wantStatus int
		wantCode           string
	}{
		{http.StatusInternalServerError, http.StatusBadGateway, api.CodeGitHubUnavailable},
		{http.StatusBadGateway, http.StatusBadGateway, api.CodeGitHubUnavailable},
		{http.StatusForbidden, http.StatusForbidden, api.CodeNotOrgMember},
		{http.StatusTooManyRequests, http.StatusTooManyRequests, api.CodeRateLimited},
	}
	for _, c := range cases {
		var out bytes.Buffer
		rec := postToken(newServer(codeGitHub(t, c.ghCode), &out), ghToken, "203.0.113.7")
		if rec.Code != c.wantStatus || errCode(t, rec) != c.wantCode {
			t.Errorf("GitHub %d: got %d %s", c.ghCode, rec.Code, rec.Body.String())
		}
		assertOneAuditLine(t, &out, c.wantStatus)
	}
}

func TestToken_GitHubSecondaryRateLimit429(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
			return
		}
		w.Header().Set("Retry-After", "60")
		http.Error(w, "{}", http.StatusForbidden)
	}))
	t.Cleanup(gh.Close)
	var out bytes.Buffer
	rec := postToken(newServer(gh, &out), ghToken, "203.0.113.7")
	if rec.Code != http.StatusTooManyRequests || errCode(t, rec) != api.CodeRateLimited {
		t.Fatalf("got %d %s, want 429 rate_limited", rec.Code, rec.Body.String())
	}
}

func TestToken_ShortKey500(t *testing.T) {
	var out bytes.Buffer
	gh := fakeGitHub(t, "active", nil)
	h := New(Deps{JWTKey: testKey[:auth.MinKeyLen-1], Org: "test-org", LeadTeam: "leaders",
		GitHub: auth.GitHub{BaseURL: gh.URL, HTTP: gh.Client()}, Audit: &audit.Logger{W: &out}, Now: func() time.Time { return t0 }})
	rec := postToken(h, ghToken, "203.0.113.7")
	if rec.Code != http.StatusInternalServerError || errCode(t, rec) != api.CodeBrokerMisconfigured {
		t.Fatalf("got %d %s, want 500 broker_misconfigured", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token\"") {
		t.Fatalf("body carries a token: %s", rec.Body.String())
	}
	assertOneAuditLine(t, &out, http.StatusInternalServerError)
}

func TestLimiter_LongKeyStoredHashed(t *testing.T) {
	l := newLimiter(1, time.Minute)
	long := strings.Repeat("x", 1<<20)
	if !l.allow(long, t0) {
		t.Fatal("first request refused")
	}
	if l.allow(long, t0) {
		t.Fatal("second request with the same long key allowed: hashing must keep one bucket per key")
	}
	for k := range l.buckets {
		if len(k) > maxKeyLen {
			t.Fatalf("stored key is %d bytes, want <= %d", len(k), maxKeyLen)
		}
	}
	if !l.allow(long+"y", t0) {
		t.Fatal("a different long key shares the bucket")
	}
}

func TestToken_GitHubUnreachable502(t *testing.T) {
	gh := httptest.NewServer(http.NotFoundHandler())
	gh.Close() // connection refused
	var out bytes.Buffer
	rec := postToken(newServer(gh, &out), ghToken, "203.0.113.7")
	if rec.Code != http.StatusBadGateway || errCode(t, rec) != api.CodeGitHubUnavailable {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
	assertOneAuditLine(t, &out, http.StatusBadGateway)
}

func TestToken_AuditOnRateLimitAndNotMember(t *testing.T) {
	var out bytes.Buffer
	rec := postToken(newServer(fakeGitHub(t, "pending", nil), &out), ghToken, "203.0.113.7")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	assertOneAuditLine(t, &out, http.StatusForbidden)

	out.Reset()
	h := newServer(fakeGitHub(t, "active", nil), &out)
	for i := 0; i < 31; i++ {
		postToken(h, ghToken, "198.51.100.3")
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 31 || !strings.Contains(lines[30], `"status":429`) {
		t.Errorf("got %d lines, last %q", len(lines), lines[len(lines)-1])
	}
}

func TestNew_NilAuditDefaults(t *testing.T) {
	// The default logger writes to os.Stdout as of New; point it at a file.
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = f
	t.Cleanup(func() { os.Stdout = orig; f.Close() })
	h := New(Deps{JWTKey: testKey, Org: "test-org", GitHub: auth.GitHub{BaseURL: codeGitHub(t, 500).URL}})
	rec := postToken(h, ghToken, "203.0.113.7") // must not panic
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status %d", rec.Code)
	}
	if b, _ := os.ReadFile(f.Name()); !strings.Contains(string(b), `"status":502`) {
		t.Errorf("stdout audit = %q", b)
	}
}

func assertOneAuditLine(t *testing.T, out *bytes.Buffer, status int) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("audit lines = %d: %q", len(lines), out.String())
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("not JSON: %s", lines[0])
	}
	if e["status"] != float64(status) || e["endpoint"] != "/token" {
		t.Errorf("entry = %s, want status %d", lines[0], status)
	}
	if strings.Contains(lines[0], ghToken) {
		t.Errorf("audit line leaks the GitHub token: %s", lines[0])
	}
}

func TestQueryRouteRequiresAuth(t *testing.T) {
	var hit int32
	h := New(Deps{JWTKey: testKey, Query: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&hit, 1) })})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/query", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized || atomic.LoadInt32(&hit) != 0 {
		t.Fatalf("got %d, handler hits %d", rec.Code, hit)
	}
}

func TestRedisRouteRequiresAuthAndIsAbsentWhenNil(t *testing.T) {
	var hit int32
	h := New(Deps{JWTKey: testKey, Redis: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&hit, 1) })})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/redis", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized || atomic.LoadInt32(&hit) != 0 {
		t.Fatalf("got %d, handler hits %d", rec.Code, hit)
	}
	rec = httptest.NewRecorder()
	New(Deps{JWTKey: testKey}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/redis", strings.NewReader("{}")))
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("route should be absent when Deps.Redis is nil, got %d", rec.Code)
	}
}

func TestE2ERouteRequiresAuthAndIsAbsentWhenNil(t *testing.T) {
	var hit int32
	h := New(Deps{JWTKey: testKey, E2E: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&hit, 1) })})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/e2e/account", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized || atomic.LoadInt32(&hit) != 0 {
		t.Fatalf("got %d, handler hits %d", rec.Code, hit)
	}
	rec = httptest.NewRecorder()
	New(Deps{JWTKey: testKey}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/e2e/account", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("route should be absent when Deps.E2E is nil, got %d", rec.Code)
	}
}

func TestCredsRouteRequiresAuthAndIsAbsentWhenNil(t *testing.T) {
	var hit int32
	h := New(Deps{JWTKey: testKey, Creds: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&hit, 1) })})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/creds", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized || atomic.LoadInt32(&hit) != 0 {
		t.Fatalf("got %d, handler hits %d", rec.Code, hit)
	}
	rec = httptest.NewRecorder()
	New(Deps{JWTKey: testKey}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/creds", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("route should be absent when Deps.Creds is nil, got %d", rec.Code)
	}
}

func TestServer_CatalogRouteAbsentWithoutHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	New(Deps{JWTKey: testKey}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/catalog", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("route should be absent when Deps.Catalog is nil, got %d", rec.Code)
	}
}

func TestServer_CatalogRouteAudited(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("version: 1\n"))
	}))
	t.Cleanup(gh.Close)
	var buf bytes.Buffer
	log := &audit.Logger{W: &buf}
	h := New(Deps{JWTKey: testKey, Audit: log,
		Catalog: catalogsvc.New(config.Catalog{Repo: "org/m", Path: "c.yaml", Ref: "main"}, "tok",
			catalogsvc.WithBaseURL(gh.URL), catalogsvc.WithAudit(log))})
	call := func(bearer string) int {
		req := httptest.NewRequest(http.MethodPost, "/catalog", strings.NewReader("{}"))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, role := range []string{"dev", "lead"} {
		tok, _, err := auth.Issue(testKey, "octocat", role, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if got := call(tok); got != http.StatusOK {
			t.Errorf("role %s: got %d, want 200", role, got)
		}
	}
	if got := call(""); got != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", got)
	}
	rows := map[string]int{}
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e struct {
			Endpoint, Role string
			Status         int
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil || e.Endpoint != "/catalog" {
			t.Fatalf("bad audit row %q: %v", l, err)
		}
		rows[fmt.Sprintf("%s/%d", e.Role, e.Status)]++
	}
	if rows["dev/200"] != 1 || rows["lead/200"] != 1 || rows["/401"] != 1 || len(rows) != 3 {
		t.Errorf("audit rows = %v, want one each of dev/200, lead/200, /401", rows)
	}
}

func TestServer_CatalogDriftRouteAbsentWithoutHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	New(Deps{JWTKey: testKey}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/catalog/drift", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("route should be absent when Deps.CatalogDrift is nil, got %d", rec.Code)
	}
}

func TestServer_CatalogDriftRouteRequiresAuth(t *testing.T) {
	var buf bytes.Buffer
	log := &audit.Logger{W: &buf}
	served := false
	h := New(Deps{JWTKey: testKey, Audit: log, CatalogDrift: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served = true
	})})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/catalog/drift", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized || served {
		t.Fatalf("no token: got %d served=%v, want 401 and not served", rec.Code, served)
	}
	if !strings.Contains(buf.String(), `"endpoint":"/catalog/drift"`) {
		t.Errorf("401 not audited under /catalog/drift: %s", buf.String())
	}
	tok, _, err := auth.Issue(testKey, "octocat", "dev", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/catalog/drift", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !served {
		t.Fatalf("with token: got %d served=%v", rec.Code, served)
	}
}

func TestRequire401IsAudited(t *testing.T) { // FR-6.1 / SC-8
	noop := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	expired, _, _ := auth.Issue(testKey, "octocat", "lead", time.Now().Add(-2*time.Hour))
	for path, endpoint := range map[string]string{"/query": "/query", "/redis": "/redis", "/e2e/account": "/e2e/account", "/creds": "/creds",
		"/mcp/grafana": "/mcp/grafana", "/mcp/grafana/sub/path": "/mcp/grafana", "/mcp/evil-injected": "/mcp/?"} {
		for _, hdr := range []string{"", "Bearer " + expired} {
			var out bytes.Buffer
			h := New(Deps{JWTKey: testKey, Audit: &audit.Logger{W: &out}, Now: func() time.Time { return t0 },
				Query: noop, Redis: noop, E2E: noop, Creds: noop, MCP: map[string]http.Handler{"grafana": noop}})
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			if hdr != "" {
				req.Header.Set("Authorization", hdr)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if rec.Code != http.StatusUnauthorized || len(lines) != 1 {
				t.Fatalf("%s %q: got %d, audit %q", path, hdr, rec.Code, out.String())
			}
			var e audit.Entry
			if err := json.Unmarshal([]byte(lines[0]), &e); err != nil || e.Status != 401 || e.Endpoint != endpoint || e.Login != "" {
				t.Fatalf("%s %q: audit line %s", path, hdr, lines[0])
			}
			if hdr != "" && strings.Contains(lines[0], expired) {
				t.Fatalf("audit line leaks the token: %s", lines[0])
			}
		}
	}
}

// tokenFor posts /token against a fake GitHub where login has the given team
// states (org membership active) and returns the response and audit buffer.
func tokenFor(t *testing.T, login string, teams map[string]string, calls *int32) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	rec := postToken(newServer(fakeGitHubTeams(t, login, "active", teams, calls), &out), ghToken, "203.0.113.7")
	return rec, &out
}

func TestToken_DevTeamGetsRoleDev(t *testing.T) { // SC-19
	rec, out := tokenFor(t, "devgal", map[string]string{"zenify-team": "active", "ript": "active"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp api.TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	login, role, err := auth.Verify(testKey, resp.Token, t0.Add(time.Minute))
	if err != nil || login != "devgal" || role != api.RoleDev || resp.Role != api.RoleDev {
		t.Fatalf("Verify = %q %q %v, resp.Role = %q; want devgal dev", login, role, err, resp.Role)
	}
	if !strings.Contains(out.String(), `"role":"dev"`) {
		t.Errorf("audit line lacks role=dev: %s", out.String())
	}
}

func TestToken_LeadWinsOverDev(t *testing.T) {
	rec, _ := tokenFor(t, "boss", map[string]string{"leaders": "active", "zenify-team": "active"}, nil)
	var resp api.TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Role != api.RoleLead {
		t.Fatalf("got %d %s, want role lead", rec.Code, rec.Body.String())
	}
}

func TestToken_DenyAndNoTeam403(t *testing.T) { // SC-22
	for _, c := range []struct {
		login string
		teams map[string]string
		code  string
	}{
		{"zenifydevteam", map[string]string{"leaders": "active"}, api.CodeLoginDenied},
		{"ZENIFYDEVTEAM", map[string]string{"zenify-team": "active"}, api.CodeLoginDenied}, // case-insensitive
		{"zenifydeveloper", nil, api.CodeLoginDenied},                                      // listed as ZenifyDeveloper
		{"outsider", map[string]string{"ript": "active"}, api.CodeNotAllowed},
		{"newbie", map[string]string{"leaders": "pending", "zenify-team": "pending"}, api.CodeNotAllowed}, // pending = not a member
	} {
		var calls int32
		rec, out := tokenFor(t, c.login, c.teams, &calls)
		if rec.Code != http.StatusForbidden || errCode(t, rec) != c.code {
			t.Errorf("%s: got %d %s, want 403 %s", c.login, rec.Code, rec.Body.String(), c.code)
		}
		if strings.Contains(rec.Body.String(), `"token"`) {
			t.Errorf("%s: body carries a token", c.login)
		}
		assertOneAuditLine(t, out, http.StatusForbidden)
		// /user + org membership only: a denied login never reaches the team checks.
		if c.code == api.CodeLoginDenied && atomic.LoadInt32(&calls) != 2 {
			t.Errorf("%s: %d GitHub calls, want 2", c.login, calls)
		}
	}
}

func TestToken_TeamCheckFailureMapping(t *testing.T) {
	for _, c := range []struct {
		ghCode, wantStatus int
		wantCode           string
	}{
		{http.StatusInternalServerError, http.StatusBadGateway, api.CodeGitHubUnavailable},
		{http.StatusTooManyRequests, http.StatusTooManyRequests, api.CodeRateLimited},
		{http.StatusUnauthorized, http.StatusUnauthorized, api.CodeGitHubTokenInvalid},
		// The caller is a confirmed org member: a 403 on the team check is
		// not not_org_member.
		{http.StatusForbidden, http.StatusBadGateway, api.CodeGitHubUnavailable},
	} {
		gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/user":
				_, _ = w.Write([]byte(`{"login":"octocat"}`))
			case "/user/memberships/orgs/test-org":
				_, _ = w.Write([]byte(`{"state":"active"}`))
			default:
				http.Error(w, "{}", c.ghCode)
			}
		}))
		t.Cleanup(gh.Close)
		var out bytes.Buffer
		rec := postToken(newServer(gh, &out), ghToken, "203.0.113.7")
		if rec.Code != c.wantStatus || errCode(t, rec) != c.wantCode {
			t.Errorf("team check %d: got %d %s", c.ghCode, rec.Code, rec.Body.String())
		}
	}
}

func TestServer_ObsRoutesAbsentWithoutHandler(t *testing.T) {
	for _, path := range []string{"/obs/route", "/obs/trace"} {
		rec := httptest.NewRecorder()
		New(Deps{JWTKey: testKey}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s should be absent when its Deps field is nil, got %d", path, rec.Code)
		}
	}
}

// The real obssvc handlers behind the real mux: 401 without a token (audited
// under the route), and role dev reaches the handler (anything but 401/403/404).
func TestServer_ObsRoutesRequireAuthAndAllowDev(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("version: 1\n"))
	}))
	t.Cleanup(gh.Close)
	var buf bytes.Buffer
	log := &audit.Logger{W: &buf}
	cat := catalogsvc.New(config.Catalog{Repo: "org/m", Path: "c.yaml", Ref: "main"}, "tok",
		catalogsvc.WithBaseURL(gh.URL), catalogsvc.WithAudit(log))
	oh := obssvc.New(cat, "http://127.0.0.1:1", "gtok", obssvc.WithAudit(log))
	h := New(Deps{JWTKey: testKey, Audit: log, ObsRoute: oh.Route(), ObsTrace: oh.Trace()})
	dev, _, err := auth.Issue(testKey, "octocat", "dev", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/obs/route", "/obs/trace"} {
		buf.Reset()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s no token: got %d, want 401", path, rec.Code)
		}
		if !strings.Contains(buf.String(), `"endpoint":"`+path+`"`) || !strings.Contains(buf.String(), `"status":401`) {
			t.Errorf("%s 401 not audited: %s", path, buf.String())
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+dev)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		switch rec.Code {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			t.Errorf("%s role dev: got %d, want the handler to run", path, rec.Code)
		}
	}
}

const agentToken = "ghs_fake-installation-token"

// fakeGitHubApp answers POST /graphql with graphqlBody and counts calls to
// /user, which an installation token must never reach.
func fakeGitHubApp(t *testing.T, graphqlBody string, userCalls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/graphql" && r.Header.Get("Authorization") == "Bearer "+agentToken:
			_, _ = w.Write([]byte(graphqlBody))
		case r.URL.Path == "/user":
			atomic.AddInt32(userCalls, 1)
			http.Error(w, `{"message":"Resource not accessible by integration"}`, http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestToken_InstallationTokenAgent(t *testing.T) { // FR-1, SC-1, SC-2
	const viewerBot = `{"data":{"viewer":{"login":"zenie-dev[bot]","databaseId":346015963}}}`
	for _, c := range []struct {
		name       string
		graphql    string
		agentIDs   []int64
		deny       []string
		wantStatus int
		wantCode   string
	}{
		{"allowed", viewerBot, []int64{346015963}, nil, http.StatusOK, ""},
		{"other id", `{"data":{"viewer":{"login":"other[bot]","databaseId":999}}}`, []int64{346015963}, nil, http.StatusForbidden, api.CodeNotAllowed},
		{"no agent_ids", viewerBot, nil, nil, http.StatusForbidden, api.CodeNotAllowed},
		{"denied login", viewerBot, []int64{346015963}, []string{"ZENIE-DEV[bot]"}, http.StatusForbidden, api.CodeLoginDenied},
		{"graphql errors", `{"errors":[{"message":"x"}]}`, []int64{346015963}, nil, http.StatusUnauthorized, api.CodeGitHubTokenInvalid},
	} {
		var userCalls int32
		var out bytes.Buffer
		gh := fakeGitHubApp(t, c.graphql, &userCalls)
		h := New(Deps{JWTKey: testKey, Org: "test-org", LeadTeam: "leaders", DevTeam: "zenify-team",
			DenyLogins: c.deny, AgentIDs: c.agentIDs, GitHub: auth.GitHub{BaseURL: gh.URL, HTTP: gh.Client()},
			Audit: &audit.Logger{W: &out}, Now: func() time.Time { return t0 }})
		rec := postToken(h, agentToken, "203.0.113.7")
		if rec.Code != c.wantStatus {
			t.Errorf("%s: got %d %s, want %d", c.name, rec.Code, rec.Body.String(), c.wantStatus)
			continue
		}
		if n := atomic.LoadInt32(&userCalls); n != 0 {
			t.Errorf("%s: %d calls to /user, want 0", c.name, n)
		}
		if strings.Contains(out.String(), agentToken) || strings.Contains(rec.Body.String(), agentToken) {
			t.Errorf("%s: installation token leaked", c.name)
		}
		if c.wantStatus != http.StatusOK {
			if errCode(t, rec) != c.wantCode {
				t.Errorf("%s: code %q, want %q", c.name, errCode(t, rec), c.wantCode)
			}
			continue
		}
		var resp api.TokenResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		login, role, err := auth.Verify(testKey, resp.Token, t0.Add(time.Minute))
		if err != nil || login != "zenie-dev[bot]" || role != "agent" || resp.Role != "agent" || resp.Login != "zenie-dev[bot]" {
			t.Errorf("%s: Verify = %q %q %v; resp %+v", c.name, login, role, err, resp)
		}
		if !strings.Contains(out.String(), `"role":"agent"`) {
			t.Errorf("%s: audit line lacks role=agent: %s", c.name, out.String())
		}
	}
}

const (
	webWorkflow = "ZenifyAIContactCenter/contact-center-web/.github/workflows/e2e-staging.yml@refs/heads/staging"
	oidcToken   = "eyJoidc.fake-actions.token" //nolint:gosec // G101 -- a test value, not a credential
)

// fakeActions returns claims for oidcToken, err otherwise; calls counts Verify.
type fakeActions struct {
	claims auth.ActionsClaims
	err    error
	calls  int32
}

func (f *fakeActions) Verify(_ context.Context, tok string) (auth.ActionsClaims, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.err != nil {
		return auth.ActionsClaims{}, f.err
	}
	if tok != oidcToken {
		return auth.ActionsClaims{}, auth.ErrOIDCInvalid
	}
	return f.claims, nil
}

func webClaims() auth.ActionsClaims {
	return auth.ActionsClaims{Repository: "ZenifyAIContactCenter/contact-center-web", WorkflowRef: webWorkflow, RunID: "18765432101"}
}

func oidcServer(t *testing.T, act *fakeActions, workflows []string, out *bytes.Buffer) http.Handler {
	t.Helper()
	return New(Deps{
		JWTKey: testKey, Org: "test-org", LeadTeam: "leaders", DevTeam: "zenify-team",
		GitHub:  auth.GitHub{BaseURL: fakeGitHub(t, "active", nil).URL},
		Actions: act, E2EWorkflows: workflows,
		Audit: &audit.Logger{W: out}, Now: func() time.Time { return t0 },
	})
}

func postBody(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(body))
	req.Header.Set("X-Real-IP", "203.0.113.9")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestToken_OIDCIssuesE2E(t *testing.T) { // FR-2.3, SC-2
	var out bytes.Buffer
	act := &fakeActions{claims: webClaims()}
	rec := postBody(oidcServer(t, act, []string{"other/repo/.github/workflows/x.yml@refs/heads/main", webWorkflow}, &out), `{"oidc_token":"`+oidcToken+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp api.TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Role != "e2e" || resp.Login != "oidc:ZenifyAIContactCenter/contact-center-web" || resp.ExpiresAt != "2026-10-15T08:15:00Z" {
		t.Errorf("resp = %+v", resp)
	}
	login, role, err := auth.Verify(testKey, resp.Token, t0.Add(time.Minute))
	if err != nil || login != "oidc:ZenifyAIContactCenter/contact-center-web" || role != "e2e" {
		t.Errorf("Verify = %q %q %v", login, role, err)
	}
}

func TestToken_OIDCWorkflowNotAllowed(t *testing.T) { // FR-2.2
	for name, ref := range map[string]string{
		"other workflow": "ZenifyAIContactCenter/contact-center-web/.github/workflows/deploy.yml@refs/heads/staging",
		"other branch":   "ZenifyAIContactCenter/contact-center-web/.github/workflows/e2e-staging.yml@refs/heads/feature",
		"prefix":         "ZenifyAIContactCenter/contact-center-web/.github/workflows/e2e-staging.yml@refs/heads/staging-evil",
	} {
		var out bytes.Buffer
		c := webClaims()
		c.WorkflowRef = ref
		rec := postBody(oidcServer(t, &fakeActions{claims: c}, []string{webWorkflow}, &out), `{"oidc_token":"`+oidcToken+`"}`)
		if rec.Code != http.StatusForbidden || errCode(t, rec) != api.CodeNotAllowed {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestToken_OIDCNoAllowlist(t *testing.T) { // FR-2.5
	for _, wfs := range [][]string{nil, {}} {
		var out bytes.Buffer
		rec := postBody(oidcServer(t, &fakeActions{claims: webClaims()}, wfs, &out), `{"oidc_token":"`+oidcToken+`"}`)
		if rec.Code != http.StatusForbidden || errCode(t, rec) != api.CodeNotAllowed {
			t.Errorf("%v: %d %s", wfs, rec.Code, rec.Body.String())
		}
	}
}

func TestToken_OIDCInvalid(t *testing.T) {
	var out bytes.Buffer
	act := &fakeActions{err: fmt.Errorf("bad aud: %w", auth.ErrOIDCInvalid)}
	rec := postBody(oidcServer(t, act, []string{webWorkflow}, &out), `{"oidc_token":"`+oidcToken+`"}`)
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != api.CodeUnauthorized {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestToken_OIDCJWKSDown(t *testing.T) {
	var out bytes.Buffer
	act := &fakeActions{err: errors.New("fetch jwks: connection refused")}
	rec := postBody(oidcServer(t, act, []string{webWorkflow}, &out), `{"oidc_token":"`+oidcToken+`"}`)
	if rec.Code != http.StatusBadGateway || errCode(t, rec) != api.CodeGitHubUnavailable {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestToken_BothFields(t *testing.T) { // FR-2.5
	var out bytes.Buffer
	act := &fakeActions{claims: webClaims()}
	rec := postBody(oidcServer(t, act, []string{webWorkflow}, &out), `{"github_token":"`+ghToken+`","oidc_token":"`+oidcToken+`"}`)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != api.CodeInvalidBody {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if act.calls != 0 {
		t.Errorf("verifier called %d times", act.calls)
	}
}

func TestToken_GitHubOnlyUnchangedWithOIDCWired(t *testing.T) { // SC-4
	var out bytes.Buffer
	act := &fakeActions{claims: webClaims()}
	h := oidcServer(t, act, []string{webWorkflow}, &out)
	rec := postToken(h, ghToken, "203.0.113.7")
	var resp api.TokenResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || resp.Role != api.RoleLead || resp.Login != "octocat" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := postBody(h, `{}`); rec.Code != http.StatusUnauthorized || errCode(t, rec) != api.CodeGitHubTokenInvalid {
		t.Errorf("empty body: %d %s", rec.Code, rec.Body.String())
	}
	if act.calls != 0 {
		t.Errorf("verifier called %d times", act.calls)
	}
}

func TestAudit_OIDCFields(t *testing.T) { // FR-2.6
	for name, tc := range map[string]struct {
		wfs    []string
		status int
	}{"issued": {[]string{webWorkflow}, 200}, "refused": {[]string{"x/y/.github/workflows/z.yml@refs/heads/main"}, 403}} {
		var out bytes.Buffer
		rec := postBody(oidcServer(t, &fakeActions{claims: webClaims()}, tc.wfs, &out), `{"oidc_token":"`+oidcToken+`"}`)
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		if rec.Code != tc.status || len(lines) != 1 {
			t.Fatalf("%s: %d, audit %q", name, rec.Code, out.String())
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
			t.Fatal(err)
		}
		if e["workflow_ref"] != webWorkflow || e["run_id"] != "18765432101" || e["status"] != float64(tc.status) ||
			e["login"] != "oidc:ZenifyAIContactCenter/contact-center-web" || e["endpoint"] != "/token" {
			t.Errorf("%s: entry = %s", name, lines[0])
		}
		if tc.status == 200 && e["role"] != "e2e" {
			t.Errorf("%s: role = %v", name, e["role"])
		}
		var resp api.TokenResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if strings.Contains(out.String(), oidcToken) || (resp.Token != "" && strings.Contains(out.String(), resp.Token)) {
			t.Errorf("%s: audit leaks a token: %s", name, lines[0])
		}
	}
	// A GitHub-token issue carries neither field.
	var out bytes.Buffer
	postToken(newServer(fakeGitHub(t, "active", nil), &out), ghToken, "203.0.113.7")
	if strings.Contains(out.String(), "workflow_ref") || strings.Contains(out.String(), "run_id") {
		t.Errorf("github issue audit has OIDC fields: %s", out.String())
	}
}

// routePaths maps every http.Handler field of Deps to a path it serves.
// TestRouter_E2ERoleOnlyE2EAccount fails when Deps gains a handler not listed here.
var routePaths = map[string]string{
	"Query": "/query", "Redis": "/redis", "E2E": "/e2e/account", "Creds": "/creds", "Catalog": "/catalog",
	"CatalogDrift": "/catalog/drift", "ObsRoute": "/obs/route", "ObsTrace": "/obs/trace",
}

func TestRouter_E2ERoleOnlyE2EAccount(t *testing.T) { // FR-2.4, SC-3
	var out bytes.Buffer
	hits := map[string]*int32{}
	counter := func(name string) http.Handler {
		n := new(int32)
		hits[name] = n
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(n, 1)
			w.WriteHeader(http.StatusOK)
		})
	}
	d := Deps{JWTKey: testKey, Audit: &audit.Logger{W: &out}, Now: func() time.Time { return t0 },
		MCP: map[string]http.Handler{"grafana": counter("mcp/grafana")}}
	paths := map[string]string{"mcp/grafana": "/mcp/grafana/sub/path"}
	hType := reflect.TypeOf((*http.Handler)(nil)).Elem()
	dv := reflect.ValueOf(&d).Elem()
	for i := 0; i < dv.NumField(); i++ {
		f := dv.Type().Field(i)
		if f.Type != hType {
			continue
		}
		p, ok := routePaths[f.Name]
		if !ok {
			t.Fatalf("Deps.%s is a route handler missing from routePaths", f.Name)
		}
		dv.Field(i).Set(reflect.ValueOf(counter(f.Name)))
		paths[f.Name] = p
	}
	h := New(d)
	e2eTok, _, _ := auth.Issue(testKey, "oidc:ZenifyAIContactCenter/contact-center-web", "e2e", time.Now())
	devTok, _, _ := auth.Issue(testKey, "octocat", api.RoleDev, time.Now())
	for name, path := range paths {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			if method == http.MethodGet && !strings.HasPrefix(path, "/mcp/") {
				continue
			}
			out.Reset()
			req := httptest.NewRequest(method, path, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer "+e2eTok)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if name == "E2E" {
				if rec.Code != http.StatusOK || atomic.LoadInt32(hits[name]) != 1 {
					t.Errorf("%s: e2e role got %d, hits %d", path, rec.Code, atomic.LoadInt32(hits[name]))
				}
				continue
			}
			if rec.Code != http.StatusForbidden || errCode(t, rec) != api.CodeNotAllowed || atomic.LoadInt32(hits[name]) != 0 {
				t.Errorf("%s %s: e2e role got %d %s, hits %d", method, path, rec.Code, rec.Body.String(), atomic.LoadInt32(hits[name]))
			}
			var e audit.Entry
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &e); err != nil || e.Status != 403 || e.Role != "e2e" ||
				e.Login != "oidc:ZenifyAIContactCenter/contact-center-web" {
				t.Errorf("%s: audit %q", path, out.String())
			}
		}
	}
	// An unknown MCP name is refused too, before the 404.
	req := httptest.NewRequest(http.MethodPost, "/mcp/posthog", nil)
	req.Header.Set("Authorization", "Bearer "+e2eTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("/mcp/posthog: %d", rec.Code)
	}
	// SC-4: role dev still reaches every handler.
	for name, path := range paths {
		before := atomic.LoadInt32(hits[name])
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+devTok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || atomic.LoadInt32(hits[name]) != before+1 {
			t.Errorf("%s: dev got %d", path, rec.Code)
		}
	}
}
