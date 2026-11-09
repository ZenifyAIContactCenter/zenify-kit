package grafanaproxy_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/grafanaproxy"
)

func TestGet_BuildsProxyURLAndAuth(t *testing.T) {
	var path, q, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, q, auth = r.URL.Path, r.URL.Query().Get("q"), r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out struct{ OK bool }
	err := grafanaproxy.New(srv.URL+"/", "tok").Get(context.Background(), "uid-tempo", "/api/search", url.Values{"q": {"{ }"}}, 1<<10, &out)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/datasources/proxy/uid/uid-tempo/api/search" {
		t.Errorf("path = %q", path)
	}
	if q != "{ }" {
		t.Errorf("q = %q", q)
	}
	if auth != "Bearer tok" {
		t.Errorf("auth = %q", auth)
	}
	if !out.OK {
		t.Error("body not decoded")
	}
}

func TestGet_ErrorNeverCarriesTokenOrBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("secret-body"))
	}))
	defer srv.Close()

	err := grafanaproxy.New(srv.URL, "tok").Get(context.Background(), "u", "/x", nil, 1<<10, nil)
	if err == nil || err.Error() != "grafana returned 500" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "tok") || strings.Contains(err.Error(), "secret-body") {
		t.Errorf("leak: %v", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()

	err := grafanaproxy.New(srv.URL, "tok").Get(context.Background(), "u", "/x", nil, 1<<10, nil)
	if !errors.Is(err, grafanaproxy.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// A deadline hit while Grafana is still answering is a timeout, not an
// unreachable Grafana: obs's 45 s budget surfaced as "grafana unreachable".
func TestGet_DeadlineIsTimeoutNotUnreachable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := grafanaproxy.New(srv.URL, "tok").Get(ctx, "u", "/x", nil, 1<<10, nil)
	if !errors.Is(err, grafanaproxy.ErrTimeout) || err.Error() != "grafana timed out" {
		t.Fatalf("err = %v", err)
	}
}

// A caller that cancels (it went away) is not a timeout.
func TestGet_CanceledIsNotTimeout(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := grafanaproxy.New(srv.URL, "tok").Get(ctx, "u", "/x", nil, 1<<10, nil)
	if err == nil || errors.Is(err, grafanaproxy.ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestGet_ConnectionRefusedIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()

	err := grafanaproxy.New(u, "tok").Get(context.Background(), "u", "/x", nil, 1<<10, nil)
	if err == nil || err.Error() != "grafana unreachable" || errors.Is(err, grafanaproxy.ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestGet_MaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"pad":"` + strings.Repeat("a", 2<<10) + `"}`))
	}))
	defer srv.Close()

	var out map[string]string
	err := grafanaproxy.New(srv.URL, "tok").Get(context.Background(), "u", "/x", nil, 1<<10, &out)
	if !errors.Is(err, grafanaproxy.ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}
