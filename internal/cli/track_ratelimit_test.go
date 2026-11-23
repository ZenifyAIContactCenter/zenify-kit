package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

func readCacheFile(t *testing.T, ws string) backlogCacheFile {
	t.Helper()
	c, err := readBacklogCache(filepath.Join(ws, ".zenify", "track", backlogCache))
	if err != nil || c == nil {
		t.Fatalf("read cache: %v %v", c, err)
	}
	return *c
}

// expire makes the cache older than the TTL, keeping the rest (items, backoff).
func expire(t *testing.T, ws string) {
	t.Helper()
	p := filepath.Join(ws, ".zenify", "track", backlogCache)
	c := readCacheFile(t, ws)
	c.FetchedAt = time.Now().Add(-backlogTTL - time.Minute).UTC()
	if err := writeBacklogCache(p, c); err != nil {
		t.Fatal(err)
	}
}

// A 429 keeps the last good Plane and GitHub items, reports the error, and the
// callers that follow (stale cache, even --force) send no request until the
// announced reset.
func TestPlane429KeepsStaleAndBacksOff(t *testing.T) {
	var gh atomic.Int32
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) {
		gh.Add(1)
		return []trackstats.Issue{{URL: "https://github.com/Z/a/issues/9", Repo: "a", Title: "gh item", CreatedAt: time.Now()}}, nil
	})
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "pat-secret")
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	s := planeTrackStats(t, ws)
	if len(planeCodes(s)) != 2 || len(s.Backlog.Items) < 3 {
		t.Fatalf("first run items = %+v", s.Backlog.Items)
	}

	reset := time.Now().Add(90 * time.Second)
	f.mu.Lock()
	f.status, f.hdr = http.StatusTooManyRequests, map[string]string{"X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10)}
	f.mu.Unlock()
	expire(t, ws)
	n, ghCalls := f.count(), gh.Load()
	s = planeTrackStats(t, ws)
	if !strings.Contains(s.Backlog.PlaneError, "429") || s.Backlog.GhError != "" {
		t.Fatalf("planeError = %q ghError = %q", s.Backlog.PlaneError, s.Backlog.GhError)
	}
	if len(planeCodes(s)) != 2 {
		t.Fatalf("stale Plane items dropped: %+v", s.Backlog.Items)
	}
	if gh.Load() == ghCalls || len(s.Backlog.Items) < 3 {
		t.Fatalf("the GitHub side must refresh on its own: calls %d, items %+v", gh.Load()-ghCalls, s.Backlog.Items)
	}
	if got := f.count() - n; got != 1 {
		t.Fatalf("429 run made %d Plane requests; the refresh must stop at the first 429", got)
	}
	st := readCacheFile(t, ws).PlaneState
	if !st.Limited || st.Until.Before(reset.Add(-2*time.Second)) || st.Until.After(reset.Add(2*time.Second)) {
		t.Fatalf("backoff = %+v, want until ~%s", st, reset)
	}

	// Backoff is honoured by later callers, stale cache or --force alike.
	n = f.count()
	expire(t, ws)
	for _, extra := range [][]string{nil, {"--force"}} {
		s = planeTrackStats(t, ws, extra...)
		if f.count() != n {
			t.Fatalf("%v: %d Plane requests during an active 429 backoff", extra, f.count()-n)
		}
		if !strings.Contains(s.Backlog.PlaneError, "429") || len(planeCodes(s)) != 2 {
			t.Fatalf("%v: planeError = %q items = %+v", extra, s.Backlog.PlaneError, s.Backlog.Items)
		}
	}
}

// Without a usable header the wait doubles per failure up to the cap.
func TestBackoffGrowsAndCaps(t *testing.T) {
	now := time.Now()
	var st sourceState
	var prev time.Duration
	for i := 0; i < 8; i++ {
		st = st.failed(os.ErrClosed, true, time.Time{}, now)
		got := st.Until.Sub(now)
		if i == 0 && got < backoffLimitedBase-time.Second {
			t.Fatalf("first wait = %s", got)
		}
		if got < prev || got > backoffMax+time.Second {
			t.Fatalf("wait %d = %s after %s", i, got, prev)
		}
		prev = got
	}
	if prev < backoffMax-time.Second {
		t.Fatalf("wait never reached the cap: %s", prev)
	}
	// A server-announced wait wins, but never beyond the header cap.
	far := now.Add(10 * time.Hour)
	if got := (sourceState{}).failed(os.ErrClosed, true, far, now).Until.Sub(now); got > backoffHeaderMax+time.Second {
		t.Fatalf("announced wait not capped: %s", got)
	}
}

func TestRateLimitUntilHeaders(t *testing.T) {
	now := time.Now()
	h := http.Header{"Retry-After": {"42"}}
	if got := rateLimitUntil(h, now).Sub(now); got != 42*time.Second {
		t.Fatalf("Retry-After -> %s", got)
	}
	if !rateLimitUntil(http.Header{}, now).IsZero() {
		t.Fatal("no header must give zero")
	}
}

// Callers started together refresh once: the loser of the lock serves the cache.
func TestConcurrentCallersRefreshOnce(t *testing.T) {
	var gh atomic.Int32
	release := make(chan struct{})
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) {
		gh.Add(1)
		<-release
		return nil, nil
	})
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "pat-secret")
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	dir := filepath.Join(ws, ".zenify", "track")

	const callers = 6
	var wg sync.WaitGroup
	type res struct {
		gh    func() ([]trackstats.Issue, error)
		plane func() ([]trackstats.PlaneItem, error)
	}
	results := make([]res, callers)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, p, _ := backlogSources(dir, ws, false, time.Now())
			results[i] = res{g, p}
		}()
	}
	// The one holder blocks inside gh; the other five return at once from the cache.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && gh.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	for _, r := range results {
		// A loser with no cache yet says so instead of showing a silent empty backlog.
		if _, err := r.gh(); err != nil && !strings.Contains(err.Error(), "another process is refreshing") {
			t.Fatalf("gh: %v", err)
		}
		if _, err := r.plane(); err != nil {
			t.Fatalf("plane: %v", err)
		}
	}
	// One refresh is 1 owner x 2 roles = 2 searches; without the lock it would be 12.
	if gh.Load() != 2 {
		t.Fatalf("gh searches = %d, want 2 (a single refresh)", gh.Load())
	}
	// /users/me, projects, states, two work-item pages: once, not six times.
	if n := f.count(); n != 5 {
		t.Fatalf("plane requests = %d, want 5 (a single refresh)", n)
	}
}

// A crashed holder's old lock file does not block refreshing forever.
func TestStaleLockIsStolen(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := tryBacklogLock(dir, time.Now()); err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if _, ok, _ := tryBacklogLock(dir, time.Now()); ok {
		t.Fatal("second lock must fail while the first is held")
	}
	if rel, ok, err := tryBacklogLock(dir, time.Now().Add(lockStale+time.Second)); err != nil || !ok {
		t.Fatalf("stale lock not stolen: %v %v", ok, err)
	} else {
		rel()
	}
}

// Each project's states are fetched once per 24h: a refresh costs 2+N requests, then 1+N.
func TestPlaneStatesCached(t *testing.T) {
	noGh(t)
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "pat-secret")
	planeTrackStats(t, ws)
	states := func() (n int) {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, r := range f.requests {
			if strings.Contains(r, "/states/") {
				n++
			}
		}
		return n
	}
	if states() != 1 {
		t.Fatalf("first refresh states requests = %d", states())
	}
	n := f.count()
	planeTrackStats(t, ws, "--force")
	if states() != 1 {
		t.Fatalf("states refetched within the TTL: %d", states())
	}
	if f.count() == n {
		t.Fatal("--force must still refetch the work items")
	}
	if len(planeCodes(planeTrackStats(t, ws))) != 2 {
		t.Fatal("items wrong with cached states")
	}

	// Past the TTL the states are read again.
	p := filepath.Join(ws, ".zenify", "track", planeStatesFile)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Entries map[string]planeStatesEntry `json:"entries"`
	}
	if err := json.Unmarshal(b, &c); err != nil || len(c.Entries) != 1 {
		t.Fatalf("states cache = %s %v", b, err)
	}
	for k, e := range c.Entries {
		e.FetchedAt = e.FetchedAt.Add(-planeStatesTTL - time.Hour)
		c.Entries[k] = e
	}
	b, _ = json.Marshal(c)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	planeTrackStats(t, ws, "--force")
	if states() != 2 {
		t.Fatalf("expired states not refetched: %d", states())
	}
}

// GitHub ages on its own clock: a Plane retry that refreshes FetchedAt must not keep an old gh list alive.
func TestSourcesAgeSeparately(t *testing.T) {
	var gh atomic.Int32
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) { gh.Add(1); return nil, nil })
	ws := trackWS(t, true)
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	dir := filepath.Join(ws, ".zenify", "track")
	now := time.Now()
	seed := func(ghAt time.Time) {
		if err := writeBacklogCache(filepath.Join(dir, backlogCache), backlogCacheFile{FetchedAt: now.UTC(), GhAt: ghAt.UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	seed(now.Add(-time.Minute))
	backlogSources(dir, ws, false, now)
	if gh.Load() != 0 {
		t.Fatalf("fresh gh list refetched: %d", gh.Load())
	}
	seed(now.Add(-backlogTTL - time.Minute))
	g, _, _ := backlogSources(dir, ws, false, now)
	if _, err := g(); err != nil {
		t.Fatal(err)
	}
	if gh.Load() != 2 {
		t.Fatalf("old gh list not refetched although FetchedAt is fresh: %d", gh.Load())
	}
}

// release removes only the lock it took: a lock another process retook is left alone.
func TestLockReleaseKeepsForeignLock(t *testing.T) {
	dir := t.TempDir()
	rel, ok, err := tryBacklogLock(dir, time.Now())
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	p := filepath.Join(dir, backlogLock)
	if err := os.WriteFile(p, []byte("someone-else"), 0o600); err != nil {
		t.Fatal(err)
	}
	rel()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("release removed a lock it no longer owned: %v", err)
	}
}
