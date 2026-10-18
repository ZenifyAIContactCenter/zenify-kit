package grafanaproxy_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
