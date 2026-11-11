package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

// fakeTrackPlane serves one project ZEN whose work items come on two pages
// (shapes measured on the real instance 2026-11-11, ids shortened).
type fakeTrackPlane struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []string // "METHOD path?query"
	keys     []string
	status   int // non-zero: answer every request with it
}

const (
	trackPlanePage1 = `{"results":[
{"id":"wi-288","sequence_id":288,"name":"Plane backlog item","state":"st-backlog","assignees":["u-dev"],"created_at":"2026-11-09T19:53:02.803317Z","project":"p-zen"},
{"id":"wi-300","sequence_id":300,"name":"Someone else's","state":"st-todo","assignees":["u-other"],"created_at":"2026-11-09T19:53:02.803317Z","project":"p-zen"}],
"next_page_results":true,"next_cursor":"100:1:0"}`
	trackPlanePage2 = `{"results":[
{"id":"wi-256","sequence_id":256,"name":"Plane started item","state":"st-inprogress","assignees":["u-other","u-dev"],"created_at":"2026-11-01T08:00:00.5Z","project":"p-zen"},
{"id":"wi-227","sequence_id":227,"name":"Plane done item","state":"st-done","assignees":["u-dev"],"created_at":"2026-10-01T08:00:00Z","project":"p-zen"}],
"next_page_results":false,"next_cursor":"100:2:0"}`
)

func newFakeTrackPlane(t *testing.T) *fakeTrackPlane {
	t.Helper()
	f := &fakeTrackPlane{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		f.keys = append(f.keys, r.Header.Get("X-API-Key"))
		status := f.status
		f.mu.Unlock()
		if status != 0 {
			http.Error(w, `{"detail":"no"}`, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		const proj = "/api/v1/workspaces/zenify/projects/"
		switch {
		case r.URL.Path == "/api/v1/users/me/":
			_, _ = io.WriteString(w, `{"id":"u-dev","display_name":"dev"}`)
		case r.URL.Path == proj:
			_, _ = io.WriteString(w, `{"results":[{"id":"p-zen","identifier":"ZEN","name":"Zenify","is_member":true},{"id":"p-kb","identifier":"KB","name":"Knowledge","is_member":false}],"next_page_results":false,"next_cursor":"100:0:0"}`)
		case strings.HasPrefix(r.URL.Path, proj+"p-kb/"):
			// A public project the developer is not a member of (measured: 403 on states).
			http.Error(w, `{"detail":"forbidden"}`, http.StatusForbidden)
		case r.URL.Path == proj+"p-zen/states/":
			_, _ = io.WriteString(w, zenStatesJSON)
		case r.URL.Path == proj+"p-zen/work-items/" && r.URL.Query().Get("cursor") == "":
			_, _ = io.WriteString(w, trackPlanePage1)
		case r.URL.Path == proj+"p-zen/work-items/" && r.URL.Query().Get("cursor") == "100:1:0":
			_, _ = io.WriteString(w, trackPlanePage2)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	saved, savedClient := trustedBrokerManifest, trackPlaneClient
	yaml := "org: o\nrepos:\n  - name: r\n    url: u\nplane:\n  url: " + f.srv.URL + "\n  workspace: zenify\n"
	trustedBrokerManifest = func() ([]byte, string) { return []byte(yaml), "test-plane" }
	trackPlaneClient = f.srv.Client()
	t.Cleanup(func() { trustedBrokerManifest, trackPlaneClient = saved, savedClient })
	return f
}

func (f *fakeTrackPlane) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// planeTrackWS is a tracked workspace; pat != "" goes to its .zenify/.env.
func planeTrackWS(t *testing.T, pat string) string {
	t.Helper()
	t.Setenv(planePATKey, "")
	ws := trackWS(t, true)
	if pat != "" {
		if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ws, ".zenify", ".env"), []byte(planePATKey+"="+pat+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

type planeStats struct {
	Backlog struct {
		Items []struct {
			Source, ID, Code, Repo, Est, Ref, Title string
		}
		GhError    string
		PlaneError string
	}
}

func planeTrackStats(t *testing.T, ws string, extra ...string) planeStats {
	t.Helper()
	out, err := runTrack(t, ws, append([]string{"stats", "--json"}, extra...)...)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	var s planeStats
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("stats json: %v\n%s", err, out)
	}
	return s
}

func planeCodes(s planeStats) []string {
	var out []string
	for _, it := range s.Backlog.Items {
		if it.Source == "plane" {
			out = append(out, it.Code)
		}
	}
	return out
}

// SC-9: two pages, three items assigned to me, one of them completed.
func TestPlaneMine(t *testing.T) {
	noGh(t)
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "pat-secret")
	s := planeTrackStats(t, ws, "--force")
	if s.Backlog.PlaneError != "" {
		t.Fatalf("planeError = %q", s.Backlog.PlaneError)
	}
	if got := strings.Join(planeCodes(s), ","); got != "ZEN-288,ZEN-256" && got != "ZEN-256,ZEN-288" {
		t.Fatalf("plane codes = %q, items %+v", got, s.Backlog.Items)
	}
	for _, it := range s.Backlog.Items {
		if it.Code == "ZEN-288" {
			u := f.srv.URL + "/zenify/browse/ZEN-288/"
			if it.ID != u || it.Ref != u || it.Repo != "-" || it.Est != "-" || it.Title != "Plane backlog item" {
				t.Fatalf("item = %+v", it)
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range f.keys {
		if k != "pat-secret" {
			t.Fatalf("X-API-Key = %q", k)
		}
	}
	var pages int
	for _, r := range f.requests {
		if strings.Contains(r, "/work-items/") {
			pages++
			if !strings.Contains(r, "per_page=100") {
				t.Fatalf("work-items request without per_page=100: %s", r)
			}
		}
	}
	if pages != 2 {
		t.Fatalf("work-item pages = %d, requests %v", pages, f.requests)
	}
}

// SC-9: a second run within 5 minutes reads the cache.
func TestPlaneCache(t *testing.T) {
	noGh(t)
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "pat-secret")
	planeTrackStats(t, ws)
	n := f.count()
	if n == 0 {
		t.Fatal("first run made no Plane request")
	}
	s := planeTrackStats(t, ws)
	if f.count() != n {
		t.Fatalf("second run within 5 min made %d Plane requests", f.count()-n)
	}
	if len(planeCodes(s)) != 2 {
		t.Fatalf("cached plane items = %+v", s.Backlog.Items)
	}
	planeTrackStats(t, ws, "--force")
	if f.count() == n {
		t.Fatal("--force must bypass the cache")
	}
}

// SC-10: a Plane 401 leaves GitHub items and keeps the cache unwritten.
func TestPlaneErrorIsolated(t *testing.T) {
	f := newFakeTrackPlane(t)
	f.status = http.StatusUnauthorized
	ws := planeTrackWS(t, "pat-bad")
	gitRepoWithOrigin(t, ws, "a", "git@github.com:Z/a.git")
	fakeGh(t, func(string, string) ([]trackstats.Issue, error) {
		return []trackstats.Issue{{URL: "https://github.com/Z/a/issues/9", Repo: "a", Title: "gh item", CreatedAt: time.Now()}}, nil
	})
	s := planeTrackStats(t, ws)
	if !strings.Contains(s.Backlog.PlaneError, "401") || s.Backlog.GhError != "" {
		t.Fatalf("planeError = %q ghError = %q", s.Backlog.PlaneError, s.Backlog.GhError)
	}
	if strings.Contains(s.Backlog.PlaneError, "pat-bad") {
		t.Fatalf("planeError leaks the PAT: %q", s.Backlog.PlaneError)
	}
	if len(s.Backlog.Items) != 1 || s.Backlog.Items[0].Source != "github" {
		t.Fatalf("items = %+v", s.Backlog.Items)
	}
	if _, err := os.Stat(filepath.Join(ws, ".zenify", "track", backlogCache)); !os.IsNotExist(err) {
		t.Fatalf("%s written after a Plane error: %v", backlogCache, err)
	}
}

// SC-11: no PAT, or a manifest without Plane, makes no request and no error.
func TestPlaneNoPAT(t *testing.T) {
	noGh(t)
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "")
	s := planeTrackStats(t, ws, "--force")
	if f.count() != 0 || s.Backlog.PlaneError != "" {
		t.Fatalf("requests %d planeError %q", f.count(), s.Backlog.PlaneError)
	}

	// TestMain's manifest has no plane block.
	trustedBrokerManifest = func() ([]byte, string) { return []byte(noBrokerYAML), "test-no-broker" }
	ws = planeTrackWS(t, "pat-secret")
	s = planeTrackStats(t, ws, "--force")
	if f.count() != 0 || s.Backlog.PlaneError != "" {
		t.Fatalf("no plane in manifest: requests %d planeError %q", f.count(), s.Backlog.PlaneError)
	}
}

// SC-12: start --from <Plane key> records the browse URL and the item leaves the backlog.
func TestStartFromPlane(t *testing.T) {
	noGh(t)
	f := newFakeTrackPlane(t)
	ws := planeTrackWS(t, "pat-secret")
	planeTrackStats(t, ws) // fills the backlog cache
	n := f.count()
	if _, err := runTrack(t, ws, "start", "--kind", "fix", "--repo", "be", "--from", "ZEN-288"); err != nil {
		t.Fatal(err)
	}
	u := f.srv.URL + "/zenify/browse/ZEN-288/"
	lines := strings.Split(strings.TrimSpace(trackLog(t, ws)), "\n")
	last := strings.Split(lines[len(lines)-1], trackSep)
	if len(last) != 7 || last[1] != "start" || last[6] != u {
		t.Fatalf("start row = %q, want from %s", lines[len(lines)-1], u)
	}
	s := planeTrackStats(t, ws)
	if got := strings.Join(planeCodes(s), ","); got != "ZEN-256" {
		t.Fatalf("plane codes after start = %q", got)
	}
	if f.count() != n {
		t.Fatalf("start and stats within the TTL called Plane %d times", f.count()-n)
	}
}

// A 3xx from Plane is an error, never followed: the PAT must not reach the redirect target.
func TestPlaneClientNoRedirect(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			leaked.Store(true)
		}
	}))
	defer other.Close()
	plane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusFound)
	}))
	defer plane.Close()
	c := *trackPlaneClient
	p := planeAPI{ctx: context.Background(), client: &c, base: plane.URL + "/api/v1", pat: "secret"}
	if err := p.do(http.MethodGet, "/users/me/", nil, &struct{}{}); err == nil {
		t.Fatal("a redirect must surface as an error")
	}
	if leaked.Load() {
		t.Fatal("X-API-Key reached the redirect target")
	}
}

// Projects are fetched in parallel, so a workspace with many projects fits the one deadline; items
// keep the project order of the list.
func TestPlaneProjectsInParallel(t *testing.T) {
	var inflight, peak atomic.Int32
	idents := []string{"A", "B", "C", "D", "E", "F"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me/"):
			io.WriteString(w, `{"id":"me"}`)
		case strings.HasSuffix(p, "/workspaces/zenify/projects/"):
			var rs []string
			for _, id := range idents {
				rs = append(rs, `{"id":"`+id+`","identifier":"`+id+`","is_member":true}`)
			}
			io.WriteString(w, `{"results":[`+strings.Join(rs, ",")+`],"next_page_results":false}`)
		case strings.HasSuffix(p, "/states/"):
			n := inflight.Add(1)
			for {
				if old := peak.Load(); n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			deadline := time.Now().Add(300 * time.Millisecond)
			for inflight.Load() < 2 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			inflight.Add(-1)
			io.WriteString(w, `{"results":[{"id":"s","group":"started"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/work-items/"):
			io.WriteString(w, `{"results":[{"sequence_id":1,"name":"x","state":"s","assignees":["me"],"created_at":"2026-11-09T19:53:02.803317Z"}],"next_page_results":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := planeAPI{ctx: context.Background(), client: srv.Client(), base: srv.URL + "/api/v1", pat: "k"}
	items, err := fetchPlaneMine(p, "zenify", "https://plane.example")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	if strings.Join(keys, ",") != "A-1,B-1,C-1,D-1,E-1,F-1" {
		t.Fatalf("keys = %v", keys)
	}
	if peak.Load() < 2 {
		t.Fatalf("projects fetched one at a time (peak in-flight %d)", peak.Load())
	}
}
