package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGitHub answers /user for "good-*" tokens and the membership endpoint for
// org "test-org" with the given state ("" = 404).
func fakeGitHub(t *testing.T, state string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{"login":"octocat","id":1}`))
		case "/user/memberships/orgs/test-org":
			if state == "" {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"state":"` + state + `","role":"member"}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestGitHub_Member(t *testing.T) {
	cases := []struct {
		name, state, token string
		wantLogin          string
		wantErr            error
	}{
		{"active", "active", "good-token", "octocat", nil},
		{"pending", "pending", "good-token", "", ErrNotMember},
		{"404", "", "good-token", "", ErrNotMember},
		{"user401", "active", "bad-token", "", ErrTokenInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := fakeGitHub(t, c.state)
			defer srv.Close()
			g := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}
			login, err := g.Member(context.Background(), c.token, "test-org")
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if login != c.wantLogin {
				t.Errorf("login = %q, want %q", login, c.wantLogin)
			}
		})
	}
}

func TestGitHub_Member_UpstreamErrorIsNotMembershipVerdict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}.Member(context.Background(), "good-token", "test-org")
	if err == nil || errors.Is(err, ErrNotMember) || errors.Is(err, ErrTokenInvalid) {
		t.Errorf("err = %v, want a plain upstream error", err)
	}
}

// statusGitHub answers /user normally and the membership endpoint with code.
// Code -1 is a 403 with Retry-After, -2 a 403 with X-RateLimit-Remaining: 0.
func statusGitHub(t *testing.T, userCode, memberCode int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			if userCode == -1 {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "{}", http.StatusForbidden)
				return
			}
			if userCode != http.StatusOK {
				http.Error(w, "{}", userCode)
				return
			}
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
			return
		}
		code := memberCode
		switch code {
		case -1: // secondary rate limit: 403 with Retry-After
			w.Header().Set("Retry-After", "60")
			code = http.StatusForbidden
		case -2: // primary rate limit: 403 with no remaining requests
			w.Header().Set("X-RateLimit-Remaining", "0")
			code = http.StatusForbidden
		}
		http.Error(w, "{}", code)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHub_Member_StatusMapping(t *testing.T) {
	cases := []struct {
		name               string
		userCode, memberCd int
		want               error
	}{
		{"membership403", 200, http.StatusForbidden, ErrOrgAccessDenied},
		{"user429", http.StatusTooManyRequests, 200, ErrGitHubRateLimited},
		{"membership429", 200, http.StatusTooManyRequests, ErrGitHubRateLimited},
		{"membership403RetryAfter", 200, -1, ErrGitHubRateLimited},
		{"membership403RemainingZero", 200, -2, ErrGitHubRateLimited},
		{"user403RetryAfter", -1, 200, ErrGitHubRateLimited},
	}
	for _, c := range cases {
		srv := statusGitHub(t, c.userCode, c.memberCd)
		_, err := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}.Member(context.Background(), "good-token", "test-org")
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// graphqlGitHub answers POST /graphql with code, body and headers, and records
// what it saw.
func graphqlGitHub(t *testing.T, code int, body string, hdr map[string]string, seen *http.Request) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = *r.Clone(context.Background())
		}
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestViewer_OK(t *testing.T) {
	var seen http.Request
	srv := graphqlGitHub(t, http.StatusOK, `{"data":{"viewer":{"login":"zenie-dev[bot]","databaseId":346015963}}}`, nil, &seen)
	login, id, err := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}.Viewer(context.Background(), "ghs_abc")
	if err != nil || login != "zenie-dev[bot]" || id != 346015963 {
		t.Fatalf("Viewer = %q %d %v", login, id, err)
	}
	if seen.Method != http.MethodPost || seen.URL.Path != "/graphql" || seen.Header.Get("Authorization") != "Bearer ghs_abc" {
		t.Errorf("request = %s %s auth %q", seen.Method, seen.URL.Path, seen.Header.Get("Authorization"))
	}
}

func TestViewer_ErrorMapping(t *testing.T) {
	for name, c := range map[string]struct {
		code int
		body string
		hdr  map[string]string
		want error
	}{
		"graphql errors": {http.StatusOK, `{"errors":[{"message":"x"}]}`, nil, ErrTokenInvalid},
		"no viewer":      {http.StatusOK, `{"data":{}}`, nil, ErrTokenInvalid},
		"401":            {http.StatusUnauthorized, `{"message":"Bad credentials"}`, nil, ErrTokenInvalid},
		"403 ratelimit":  {http.StatusForbidden, `{}`, map[string]string{"X-RateLimit-Remaining": "0"}, ErrGitHubRateLimited},
		"429":            {http.StatusTooManyRequests, `{}`, nil, ErrGitHubRateLimited},
	} {
		srv := graphqlGitHub(t, c.code, c.body, c.hdr, nil)
		_, _, err := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}.Viewer(context.Background(), "ghs_secret")
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	srv := graphqlGitHub(t, http.StatusInternalServerError, `{}`, nil, nil)
	_, _, err := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}.Viewer(context.Background(), "ghs_secret")
	if err == nil || errors.Is(err, ErrTokenInvalid) || errors.Is(err, ErrGitHubRateLimited) {
		t.Errorf("500: err = %v, want a plain error", err)
	}
	if err != nil && strings.Contains(err.Error(), "ghs_secret") {
		t.Errorf("error leaks the token: %v", err)
	}
}
