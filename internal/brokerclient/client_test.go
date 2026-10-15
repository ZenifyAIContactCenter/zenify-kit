package brokerclient

import (
	"context"
	"encoding/json"
	"errors"
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
// replayed to the Location host; the 3xx becomes an APIError with a WARP hint.
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
	if !errors.As(err, &ae) || ae.Status != http.StatusTemporaryRedirect || !strings.Contains(ae.Message, "WARP") {
		t.Fatalf("err = %v, want APIError 307 with a WARP hint", err)
	}
	if elsewhere != 0 {
		t.Fatalf("redirect target got %d requests, want 0", elsewhere)
	}
}

func TestToken_NonJSONReplyIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>Cloudflare Access</html>"))
	}))
	t.Cleanup(srv.Close)
	now := t0
	_, err := newClient(srv.URL, t.TempDir(), &now).Token(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || !strings.Contains(ae.Message, "WARP") {
		t.Fatalf("err = %v, want APIError with a WARP hint", err)
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
