package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// redirectServer answers every request with a 302 to the tag page, like
// github.com/<owner>/<repo>/releases/latest does, and counts hits.
func redirectServer(t *testing.T, tag string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Location", "/ZenifyAIContactCenter/zenify-kit/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func readCache(t *testing.T, dir string) cacheEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, CacheFile))
	if err != nil {
		t.Fatalf("cache missing: %v", err)
	}
	var c cacheEntry
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("cache not json: %v", err)
	}
	return c
}

func writeCache(t *testing.T, dir string, c cacheEntry) {
	t.Helper()
	raw, _ := json.Marshal(c)
	if err := os.WriteFile(filepath.Join(dir, CacheFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLatest_FollowsRedirectLocation(t *testing.T) {
	srv, _ := redirectServer(t, "v9.9.9")
	got, err := Latest(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v9.9.9" {
		t.Fatalf("Latest = %q, want v9.9.9", got)
	}
}

func TestLatest_NonRedirectIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	if _, err := Latest(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("500 must be an error")
	}
}

func TestLatest_LocationWithoutTagIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/somewhere/else")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	if _, err := Latest(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("Location without /tag/ must be an error")
	}
}

// SC-1: empty cache → network → newer + cache written.
func TestCheck_ColdCacheHitsNetworkAndWrites(t *testing.T) {
	srv, hits := redirectServer(t, "v9.9.9")
	dir := filepath.Join(t.TempDir(), "zhome")
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	res := Check(Options{Current: "0.17.3", CacheDir: dir, URL: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Latest != "v9.9.9" || !res.Newer {
		t.Fatalf("got %+v", res)
	}
	if *hits != 1 {
		t.Fatalf("hits = %d, want 1", *hits)
	}
	c := readCache(t, dir)
	if c.Latest != "v9.9.9" || !c.CheckedAt.Equal(now) {
		t.Fatalf("cache = %+v", c)
	}
}

// SC-2: fresh cache → no network; stale (25h) → one request.
func TestCheck_FreshCacheSkipsNetwork(t *testing.T) {
	srv, hits := redirectServer(t, "v9.9.9")
	dir := t.TempDir()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	writeCache(t, dir, cacheEntry{CheckedAt: now.Add(-1 * time.Hour), Latest: "v9.9.9"})
	opts := Options{Current: "0.17.3", CacheDir: dir, URL: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		res := Check(opts)
		if res.Err != nil || !res.Newer || res.Latest != "v9.9.9" {
			t.Fatalf("run %d: %+v", i, res)
		}
	}
	if *hits != 0 {
		t.Fatalf("hits = %d, want 0", *hits)
	}
	writeCache(t, dir, cacheEntry{CheckedAt: now.Add(-25 * time.Hour), Latest: "v9.9.9"})
	if res := Check(opts); res.Err != nil || !res.Newer {
		t.Fatalf("stale: %+v", res)
	}
	if *hits != 1 {
		t.Fatalf("hits after stale = %d, want 1", *hits)
	}
}

// SC-2 (Force): fresh cache but Force → network.
func TestCheck_ForceBypassesCache(t *testing.T) {
	srv, hits := redirectServer(t, "v9.9.9")
	dir := t.TempDir()
	now := time.Now()
	writeCache(t, dir, cacheEntry{CheckedAt: now, Latest: "v0.0.1"})
	res := Check(Options{Current: "0.17.3", CacheDir: dir, URL: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }, Force: true})
	if res.Latest != "v9.9.9" || *hits != 1 {
		t.Fatalf("got %+v hits=%d", res, *hits)
	}
}

// SC-3: failure → Err, Newer=false, empty cache; back-off 1h.
func TestCheck_FailureWritesEmptyCacheAndBacksOff(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base
	opts := Options{Current: "0.17.3", CacheDir: dir, URL: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }}

	res := Check(opts)
	if res.Err == nil || res.Newer {
		t.Fatalf("failure must set Err and Newer=false: %+v", res)
	}
	if c := readCache(t, dir); c.Latest != "" || !c.CheckedAt.Equal(base) {
		t.Fatalf("cache = %+v", c)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}

	now = base.Add(30 * time.Minute)
	res = Check(opts)
	if res.Err == nil || res.Newer || hits != 1 {
		t.Fatalf("within back-off: %+v hits=%d", res, hits)
	}

	now = base.Add(61 * time.Minute)
	_ = Check(opts)
	if hits != 2 {
		t.Fatalf("after back-off hits = %d, want 2", hits)
	}
}

// SC-5: equal versions differing only by the v prefix are not newer; semver, not string order.
func TestCheck_SemverNotStringCompare(t *testing.T) {
	cases := []struct {
		tag, current string
		newer        bool
	}{
		{"v0.17.3", "0.17.3", false},
		{"v0.17.9", "0.17.10", false},
		{"v0.17.10", "0.17.9", true},
	}
	for _, c := range cases {
		srv, _ := redirectServer(t, c.tag)
		res := Check(Options{Current: c.current, CacheDir: t.TempDir(), URL: srv.URL, Client: srv.Client()})
		if res.Err != nil || res.Newer != c.newer {
			t.Errorf("tag %s current %s: %+v", c.tag, c.current, res)
		}
	}
}

// Empty CacheDir → no cache read/write, still works.
func TestCheck_NoCacheDir(t *testing.T) {
	srv, hits := redirectServer(t, "v9.9.9")
	res := Check(Options{Current: "0.17.3", URL: srv.URL, Client: srv.Client()})
	if res.Err != nil || !res.Newer || *hits != 1 {
		t.Fatalf("got %+v hits=%d", res, *hits)
	}
}

// Unreachable URL (closed port) → Err, no panic.
func TestCheck_UnreachableIsError(t *testing.T) {
	srv, _ := redirectServer(t, "v9.9.9")
	url := srv.URL
	srv.Close()
	res := Check(Options{Current: "0.17.3", URL: url, Client: &http.Client{Timeout: time.Second}})
	if res.Err == nil || res.Newer {
		t.Fatalf("got %+v", res)
	}
}
