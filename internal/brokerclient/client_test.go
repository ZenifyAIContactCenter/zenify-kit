package brokerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

var t0 = time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

// fakeBroker issues "jwt-N" tokens valid for 15 minutes from t0 and counts calls.
func fakeBroker(t *testing.T, tokenCalls *int32, onPost func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(tokenCalls, 1)
		var req api.TokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.GitHubToken != "test-gh" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeGitHubTokenInvalid, Message: "bad"})
			return
		}
		_ = json.NewEncoder(w).Encode(api.TokenResponse{
			Token:     "jwt-" + string(rune('0'+n)),
			ExpiresAt: t0.Add(15 * time.Minute).Format(time.RFC3339),
			Login:     "octo",
			Role:      "dev",
		})
	})
	if onPost != nil {
		mux.HandleFunc("POST /query", onPost)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newClient(url, home string, now *time.Time) *Client {
	return &Client{
		URL:     url,
		Home:    home,
		GHToken: func(context.Context) (string, error) { return "test-gh", nil },
		HTTP:    http.DefaultClient,
		Now:     func() time.Time { return *now },
	}
}

func TestToken_UsesCacheUntil60s(t *testing.T) {
	var calls int32
	srv := fakeBroker(t, &calls, nil)
	now := t0
	c := newClient(srv.URL, t.TempDir(), &now)

	a, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now = t0.Add(10 * time.Minute) // 5 min left
	b, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a != "jwt-1" || b != "jwt-1" || calls != 1 {
		t.Fatalf("a=%q b=%q calls=%d, want jwt-1 jwt-1 1", a, b, calls)
	}
	now = t0.Add(14*time.Minute + 30*time.Second) // 30 s left -> refresh
	d, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d != "jwt-2" || calls != 2 {
		t.Fatalf("d=%q calls=%d, want jwt-2 2", d, calls)
	}
}

func TestToken_CacheMode0600(t *testing.T) {
	var calls int32
	srv := fakeBroker(t, &calls, nil)
	now := t0
	home := t.TempDir()
	if _, err := newClient(srv.URL, home, &now).Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(home, ".zenify", "broker", "token.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestPost_Retries401Once(t *testing.T) {
	var calls, posts int32
	var seen []string
	srv := fakeBroker(t, &calls, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		seen = append(seen, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer jwt-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeUnauthorized, Message: "expired"})
			return
		}
		_ = json.NewEncoder(w).Encode(api.QueryResponse{Stdout: "ok", Rows: 1})
	})
	now := t0
	c := newClient(srv.URL, t.TempDir(), &now)
	var out api.QueryResponse
	if err := c.Post(context.Background(), "/query", api.QueryRequest{Op: "tables"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Stdout != "ok" || posts != 2 || calls != 2 {
		t.Fatalf("out=%+v posts=%d tokenCalls=%d, want ok 2 2 (%v)", out, posts, calls, seen)
	}
}

func TestPost_APIErrorAndNoSecondRetry(t *testing.T) {
	var calls, posts int32
	srv := fakeBroker(t, &calls, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeUnauthorized, Message: "nope"})
	})
	now := t0
	c := newClient(srv.URL, t.TempDir(), &now)
	err := c.Post(context.Background(), "/query", api.QueryRequest{}, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 401 || ae.Code != api.CodeUnauthorized || ae.Message != "nope" {
		t.Fatalf("err = %#v, want APIError 401 unauthorized nope", err)
	}
	if posts != 2 {
		t.Fatalf("posts = %d, want exactly 2 (one retry)", posts)
	}
}

// A cached JWT is only reused for the broker URL that issued it.
func TestToken_CacheKeyedByURL(t *testing.T) {
	var calls1, calls2 int32
	a, b := fakeBroker(t, &calls1, nil), fakeBroker(t, &calls2, nil)
	now := t0
	home := t.TempDir()
	if _, err := newClient(a.URL, home, &now).Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := newClient(b.URL, home, &now).Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls1 != 1 || calls2 != 1 {
		t.Fatalf("token calls a=%d b=%d, want 1 1 (no cross-broker cache hit)", calls1, calls2)
	}
}

// The default client never follows a redirect, so the GitHub token body is not
// replayed to the Location host; the 3xx becomes an APIError with a broker.url hint.
func TestToken_RedirectNotFollowed(t *testing.T) {
	var elsewhere int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&elsewhere, 1) }))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/token", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	now := t0
	c := newClient(srv.URL, t.TempDir(), &now)
	c.HTTP = nil // the real redirect policy
	_, err := c.Token(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != http.StatusTemporaryRedirect || !strings.Contains(ae.Message, "broker.url") {
		t.Fatalf("err = %v, want APIError 307 with a broker.url hint", err)
	}
	if elsewhere != 0 {
		t.Fatalf("redirect target got %d requests, want 0", elsewhere)
	}
}

func TestToken_NonJSONReplyIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>proxy login</html>"))
	}))
	t.Cleanup(srv.Close)
	now := t0
	_, err := newClient(srv.URL, t.TempDir(), &now).Token(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || !strings.Contains(ae.Message, "broker.url") {
		t.Fatalf("err = %v, want APIError with a broker.url hint", err)
	}
}

func TestToken_BadReplyNotCached(t *testing.T) {
	for name, body := range map[string]api.TokenResponse{
		"empty token": {Token: "", ExpiresAt: t0.Add(time.Hour).Format(time.RFC3339)},
		"bad expiry":  {Token: "jwt", ExpiresAt: "soon"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(body)
		}))
		now := t0
		home := t.TempDir()
		if _, err := newClient(srv.URL, home, &now).Token(context.Background()); err == nil {
			t.Errorf("%s: want error", name)
		}
		if _, err := os.Stat(filepath.Join(home, ".zenify", "broker", "token.json")); err == nil {
			t.Errorf("%s: token.json written", name)
		}
		srv.Close()
	}
}

// A cache write the sandbox denies only warns; the token is still returned.
func TestToken_CacheWriteFailureWarns(t *testing.T) {
	var calls int32
	srv := fakeBroker(t, &calls, nil)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".zenify"), nil, 0o600); err != nil { // a file where the dir goes
		t.Fatal(err)
	}
	now := t0
	c := newClient(srv.URL, home, &now)
	var warn strings.Builder
	c.Stderr = &warn
	tok, err := c.Token(context.Background())
	if err != nil || tok != "jwt-1" {
		t.Fatalf("tok=%q err=%v, want jwt-1 nil", tok, err)
	}
	if !strings.Contains(warn.String(), "not cached") {
		t.Fatalf("warning = %q", warn.String())
	}
}

// GHTokenExec carries gh's stderr; PATH holds only a fake gh.
func TestGHTokenExec_IncludesStderr(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'not logged in to github.com' >&2\nexit 1\n"), 0o700); err != nil { //nolint:gosec // test fake must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	_, err := GHTokenExec(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v, want gh stderr in it", err)
	}
}

// Only 401/403 non-JSON replies get the broker.url hint; a non-JSON 5xx says the
// broker failed, and a JSON 500 (broker_misconfigured) keeps its own message.
func TestDo_ErrorClassification(t *testing.T) {
	cases := []struct {
		name, body     string
		status         int
		code, contains string
		hint           bool
	}{
		{"403 html", "<html>Access</html>", http.StatusForbidden, "not_json", "broker.url", true},
		{"401 html", "<html>Access</html>", http.StatusUnauthorized, "not_json", "broker.url", true},
		{"502 html", "<html>Bad gateway</html>", http.StatusBadGateway, "not_json", "broker lỗi HTTP 502", false}, //znf:allow-lang
		{"500 misconfigured", `{"error":"broker_misconfigured","message":"JWT signing key too short"}`, http.StatusInternalServerError, api.CodeBrokerMisconfigured, "JWT signing key too short", false},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		now := t0
		_, err := newClient(srv.URL, t.TempDir(), &now).Token(context.Background())
		srv.Close()
		var ae *APIError
		if !errors.As(err, &ae) || ae.Status != tc.status || ae.Code != tc.code || !strings.Contains(ae.Message, tc.contains) || strings.Contains(ae.Message, "broker.url") != tc.hint {
			t.Errorf("%s: err = %#v", tc.name, err)
		}
	}
}

// A reply over maxBody is an error, never a silently cut body.
func TestDo_OversizedReplyRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"stdout":"`))
		_, _ = w.Write(make([]byte, maxBody))
	}))
	t.Cleanup(srv.Close)
	now := t0
	c := newClient(srv.URL, t.TempDir(), &now)
	err := c.do(context.Background(), "/query", "jwt", struct{}{}, &api.QueryResponse{})
	if err == nil || !strings.Contains(err.Error(), "over") {
		t.Fatalf("err = %v, want an over-size error", err)
	}
}

// Refresh skips a valid cached JWT, caches the new one and returns the login.
func TestRefresh_IgnoresCache(t *testing.T) {
	var calls int32
	srv := fakeBroker(t, &calls, nil)
	now := t0
	c := newClient(srv.URL, t.TempDir(), &now)
	if _, err := c.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	tok, login, _, err := c.Refresh(context.Background())
	if err != nil || tok != "jwt-2" || login != "octo" || calls != 2 {
		t.Fatalf("tok=%q login=%q err=%v calls=%d, want jwt-2 octo nil 2", tok, login, err, calls)
	}
	if again, _ := c.Token(context.Background()); again != "jwt-2" || calls != 2 {
		t.Fatalf("cached after refresh = %q calls=%d, want jwt-2 2", again, calls)
	}
}

// GHTokenExec returns gh's trimmed stdout; empty output is an error.
func TestGHTokenExec_Output(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake")
	}
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	t.Setenv("PATH", bin)
	if err := os.WriteFile(gh, []byte("#!/bin/sh\necho '  gho_test  '\n"), 0o700); err != nil { //nolint:gosec // test fake must be executable
		t.Fatal(err)
	}
	if tok, err := GHTokenExec(context.Background()); err != nil || tok != "gho_test" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { //nolint:gosec // test fake must be executable
		t.Fatal(err)
	}
	if _, err := GHTokenExec(context.Background()); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want empty-output error", err)
	}
}

// A gh that never exits is stopped by ctx.
func TestGHTokenExec_ContextBound(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil { //nolint:gosec // test fake must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/bin")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := GHTokenExec(ctx); err == nil {
		t.Fatal("want an error")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestFromManifest(t *testing.T) {
	if _, ok := FromManifest(nil); ok {
		t.Fatal("nil manifest: ok")
	}
	if _, ok := FromManifest(&manifest.Manifest{}); ok {
		t.Fatal("no broker: ok")
	}
	c, ok := FromManifest(&manifest.Manifest{Broker: &manifest.Broker{URL: "https://b.example"}})
	if !ok || c.URL != "https://b.example" {
		t.Fatalf("c=%+v ok=%v", c, ok)
	}
}

// A cache entry written before roles existed holds a JWT the broker now
// refuses, so it is a miss; the new entry carries the role.
func TestTokenInfo_RolelessCacheIsMiss(t *testing.T) {
	var calls int32
	srv := fakeBroker(t, &calls, nil)
	now := t0
	home := t.TempDir()
	dir := filepath.Join(home, ".zenify", "broker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := fmt.Sprintf(`{"url":%q,"token":"old-jwt","expires_at":"2099-01-01T00:00:00Z","login":"octo"}`, srv.URL)
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newClient(srv.URL, home, &now)
	tok, login, role, err := c.TokenInfo(context.Background())
	if err != nil || tok == "old-jwt" || login != "octo" || role != "dev" || calls != 1 {
		t.Fatalf("tok=%q login=%q role=%q err=%v calls=%d, want fresh dev token", tok, login, role, err, calls)
	}
	if tok2, _, role2, _ := c.TokenInfo(context.Background()); tok2 != tok || role2 != "dev" || calls != 1 {
		t.Fatalf("second call tok=%q role=%q calls=%d, want cache hit", tok2, role2, calls)
	}
}

// CachedRole reads only the cache, ignores expiry, and trusts no other broker's entry.
func TestCachedRole_NoNetworkURLScoped(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".zenify", "broker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"url":"https://b.example","token":"t","expires_at":"2000-01-01T00:00:00Z","login":"devgal","role":"dev"}`
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	noNet := func(context.Context) (string, error) { t.Fatal("gh token asked"); return "", nil }
	if r := (&Client{URL: "https://b.example", Home: home, GHToken: noNet}).CachedRole(); r != "dev" {
		t.Errorf("same URL: role %q, want dev", r)
	}
	if r := (&Client{URL: "https://other.example", Home: home, GHToken: noNet}).CachedRole(); r != "" {
		t.Errorf("other URL: role %q, want empty", r)
	}
	if r := (&Client{URL: "https://b.example", Home: t.TempDir(), GHToken: noNet}).CachedRole(); r != "" {
		t.Errorf("no cache: role %q, want empty", r)
	}
	if u, l, r, ok := CachedIdentity(home); !ok || u != "https://b.example" || l != "devgal" || r != "dev" {
		t.Errorf("CachedIdentity = %q %q %q %v", u, l, r, ok)
	}
}

// GH_TOKEN (the agent's installation token) wins over `gh`, which must not run.
func TestGHTokenExec_PrefersGHTokenEnv(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no gh on PATH: running it would fail
	t.Setenv("GH_TOKEN", "  ghs_abc \n")
	tok, err := GHTokenExec(context.Background())
	if err != nil || tok != "ghs_abc" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestDo_SendsTaskHeader(t *testing.T) {
	for name, c := range map[string]struct{ env, want string }{
		"set":      {"ZEN-12", "ZEN-12"},
		"unset":    {"", ""},
		"invalid":  {"bad value!", ""},
		"too long": {strings.Repeat("a", 65), ""},
	} {
		t.Setenv("ZNF_TASK_ID", c.env)
		var got string
		var seen bool
		var calls int32
		srv := fakeBroker(t, &calls, func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("X-Zenify-Task")
			_, seen = r.Header["X-Zenify-Task"]
			_, _ = w.Write([]byte("{}"))
		})
		now := t0
		if err := newClient(srv.URL, t.TempDir(), &now).Post(context.Background(), "/query", map[string]string{}, nil); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != c.want || (c.want == "" && seen) {
			t.Errorf("%s: header = %q (present %v), want %q", name, got, seen, c.want)
		}
	}
}
