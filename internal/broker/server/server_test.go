package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
