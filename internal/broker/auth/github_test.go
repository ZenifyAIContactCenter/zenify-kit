package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
func statusGitHub(t *testing.T, userCode, memberCode int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			if userCode != http.StatusOK {
				http.Error(w, "{}", userCode)
				return
			}
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
			return
		}
		http.Error(w, "{}", memberCode)
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
	}
	for _, c := range cases {
		srv := statusGitHub(t, c.userCode, c.memberCd)
		_, err := GitHub{BaseURL: srv.URL, HTTP: srv.Client()}.Member(context.Background(), "good-token", "test-org")
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}
