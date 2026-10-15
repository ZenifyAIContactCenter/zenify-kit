package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

const ghToken = "gho_fake-test-token"

var (
	testKey = []byte("test-key")
	t0      = time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
)

// fakeGitHub: ghToken is octocat; membership in "test-org" has the given state.
func fakeGitHub(t *testing.T, state string, calls *int32) *httptest.Server {
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
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
		case "/user/memberships/orgs/test-org":
			_, _ = w.Write([]byte(`{"state":"` + state + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newServer(gh *httptest.Server, out *bytes.Buffer) http.Handler {
	return New(Deps{
		JWTKey: testKey,
		Org:    "test-org",
		GitHub: auth.GitHub{BaseURL: gh.URL, HTTP: gh.Client()},
		Audit:  &audit.Logger{W: out},
		Now:    func() time.Time { return t0 },
	})
}

func postToken(h http.Handler, ghTok, cfIP string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"github_token":"`+ghTok+`"}`))
	if cfIP != "" {
		req.Header.Set("CF-Connecting-IP", cfIP)
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
	login, err := auth.Verify(testKey, resp["token"], t0.Add(time.Minute))
	if err != nil || login != "octocat" {
		t.Errorf("Verify = %q, %v", login, err)
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

func TestToken_RateLimitPerCFConnectingIP(t *testing.T) {
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
