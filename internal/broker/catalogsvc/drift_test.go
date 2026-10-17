package catalogsvc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/config"
)

// driftYAML: prd lists a (all sources) and d (loki only); b is ignored on prd
// loki; e is only ignored on prd tempo. stg lists a on loki only.
const driftYAML = `version: 1
datasources:
  prd: {prometheus: uid-prd-prom, loki: uid-prd-loki, tempo: uid-prd-tempo}
  stg: {prometheus: uid-stg-prom, loki: uid-stg-loki, tempo: uid-stg-tempo}
labels: {loki_app: app}
services:
  - id: svc-a
    repo: org/svc-a
    prd: {loki: [a], tempo: [a], prometheus_job: [a]}
    stg: {loki: [a]}
datastores:
  - id: svc-d
    repo: org/svc-d
    prd: {loki: [d]}
ignore:
  prd: {loki: [b], tempo: [e]}
`

const (
	lokiPrd  = "/api/datasources/proxy/uid/uid-prd-loki/loki/api/v1/label/app/values"
	tempoPrd = "/api/datasources/proxy/uid/uid-prd-tempo/api/v2/search/tag/resource.service.name/values"
	promPrd  = "/api/datasources/proxy/uid/uid-prd-prom/api/v1/label/job/values"
	lokiStg  = "/api/datasources/proxy/uid/uid-stg-loki/loki/api/v1/label/app/values"
	tempoStg = "/api/datasources/proxy/uid/uid-stg-tempo/api/v2/search/tag/resource.service.name/values"
	promStg  = "/api/datasources/proxy/uid/uid-stg-prom/api/v1/label/job/values"
)

// fakeGrafana answers by path; an unknown path is 404. It records every request.
type fakeGrafana struct {
	mu     sync.Mutex
	routes map[string]struct {
		status int
		body   string
	}
	reqs []*http.Request
}

func (g *fakeGrafana) set(path string, status int, body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.routes[path] = struct {
		status int
		body   string
	}{status, body}
}

func (g *fakeGrafana) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	rt, ok := g.routes[r.URL.Path]
	g.reqs = append(g.reqs, r)
	g.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(rt.status)
	_, _ = w.Write([]byte(rt.body))
}

func (g *fakeGrafana) request(path string) *http.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, r := range g.reqs {
		if r.URL.Path == path {
			return r
		}
	}
	return nil
}

func labelValues(v ...string) string {
	b, _ := json.Marshal(map[string]any{"status": "success", "data": v})
	return string(b)
}

func tempoValues(v ...string) string {
	tv := []map[string]string{}
	for _, s := range v {
		tv = append(tv, map[string]string{"type": "string", "value": s})
	}
	b, _ := json.Marshal(map[string]any{"tagValues": tv, "metrics": map[string]any{"inspectedBytes": "1"}})
	return string(b)
}

// driftNow is the fake clock: 2026-10-18T00:00:00Z.
var driftNow = time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)

func driftSetup(t *testing.T, opts ...Option) (*Drift, *fakeGrafana) {
	t.Helper()
	return driftSetupYAML(t, driftYAML, opts...)
}

func driftSetupYAML(t *testing.T, yaml string, opts ...Option) (*Drift, *fakeGrafana) {
	t.Helper()
	gh := httptest.NewServer(&stub{status: 200, body: yaml})
	t.Cleanup(gh.Close)
	g := &fakeGrafana{routes: map[string]struct {
		status int
		body   string
	}{}}
	for _, p := range []string{lokiPrd, promPrd, lokiStg, promStg} {
		g.set(p, 200, labelValues())
	}
	for _, p := range []string{tempoPrd, tempoStg} {
		g.set(p, 200, tempoValues())
	}
	gs := httptest.NewServer(g)
	t.Cleanup(gs.Close)
	now := func() time.Time { return driftNow }
	h := New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token",
		WithBaseURL(gh.URL), WithClock(now))
	return NewDrift(h, gs.URL+"/", "glsa_secret_token", append([]Option{WithClock(now)}, opts...)...), g
}

func postDrift(h http.Handler, body string) (int, api.DriftResponse, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/catalog/drift", strings.NewReader(body)))
	var resp api.DriftResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp, rec.Body.String()
}

func TestDrift_UnlistedAndMissing(t *testing.T) {
	d, g := driftSetup(t)
	g.set(lokiPrd, 200, labelValues("a", "b", "c"))
	code, resp, body := postDrift(d, `{"env":"prd"}`)
	if code != 200 {
		t.Fatalf("code %d: %s", code, body)
	}
	got := resp.Envs["prd"]["loki"]
	want := api.DriftResult{Unlisted: []string{"c"}, Missing: []string{"d"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prd loki = %+v, want %+v", got, want)
	}
}

func TestDrift_SortedAcrossSources(t *testing.T) {
	d, g := driftSetup(t)
	g.set(promPrd, 200, labelValues("z", "a", "m"))
	g.set(tempoPrd, 200, tempoValues("y", "x"))
	_, resp, body := postDrift(d, `{"env":"prd"}`)
	if got, want := resp.Envs["prd"]["prometheus"], (api.DriftResult{Unlisted: []string{"m", "z"}, Missing: []string{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("prometheus = %+v, want %+v (%s)", got, want, body)
	}
	if got, want := resp.Envs["prd"]["tempo"], (api.DriftResult{Unlisted: []string{"x", "y"}, Missing: []string{"a"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("tempo = %+v, want %+v (%s)", got, want, body)
	}
}

func TestDrift_SourceErrorIsolated(t *testing.T) {
	d, g := driftSetup(t)
	g.set(tempoPrd, 500, `{"message":"Authorization: Bearer glsa_secret_token"}`)
	g.set(lokiPrd, 200, labelValues("a", "d"))
	g.set(promPrd, 200, labelValues("a"))
	code, resp, body := postDrift(d, `{"env":"prd"}`)
	if code != 200 {
		t.Fatalf("code %d: %s", code, body)
	}
	tempo := resp.Envs["prd"]["tempo"]
	if tempo.Error == "" || !strings.Contains(tempo.Error, "500") {
		t.Errorf("tempo error = %q, want one naming status 500", tempo.Error)
	}
	if strings.Contains(body, "glsa_secret_token") || strings.Contains(body, "Authorization") {
		t.Errorf("response echoes the upstream body: %s", body)
	}
	empty := api.DriftResult{Unlisted: []string{}, Missing: []string{}}
	if got := resp.Envs["prd"]["loki"]; !reflect.DeepEqual(got, empty) {
		t.Errorf("loki = %+v, want %+v", got, empty)
	}
	if got := resp.Envs["prd"]["prometheus"]; !reflect.DeepEqual(got, empty) {
		t.Errorf("prometheus = %+v, want %+v", got, empty)
	}
}

// TestDrift_ConfigGapIsPerSource: no labels.loki_app and no prd tempo uid
// fail only those sources, before any HTTP call, for both envs at once.
func TestDrift_ConfigGapIsPerSource(t *testing.T) {
	y := strings.Replace(driftYAML, "labels: {loki_app: app}\n", "", 1)
	y = strings.Replace(y, ", tempo: uid-prd-tempo}", "}", 1)
	d, g := driftSetupYAML(t, y)
	g.set(promPrd, 200, labelValues("a"))
	code, resp, body := postDrift(d, `{}`)
	if code != 200 {
		t.Fatalf("code %d: %s", code, body)
	}
	for _, env := range []string{"prd", "stg"} {
		if e := resp.Envs[env]["loki"].Error; !strings.Contains(e, "loki_app") {
			t.Errorf("%s loki error = %q, want one naming loki_app", env, e)
		}
	}
	if e := resp.Envs["prd"]["tempo"].Error; !strings.Contains(e, "uid") {
		t.Errorf("prd tempo error = %q, want one naming the missing uid", e)
	}
	if got, want := resp.Envs["prd"]["prometheus"], (api.DriftResult{Unlisted: []string{}, Missing: []string{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("prd prometheus = %+v, want %+v", got, want)
	}
	if r := resp.Envs["stg"]["tempo"]; r.Error != "" {
		t.Errorf("stg tempo error = %q, want none", r.Error)
	}
	if strings.Contains(body, "glsa_secret_token") {
		t.Error("response leaks the token")
	}
}

func TestDrift_IgnoreOnly(t *testing.T) {
	d, g := driftSetup(t)
	// e is in ignore (prd tempo) only and is live; a is listed and live.
	g.set(tempoPrd, 200, tempoValues("a", "e"))
	_, resp, body := postDrift(d, `{"env":"prd"}`)
	want := api.DriftResult{Unlisted: []string{}, Missing: []string{}}
	if got := resp.Envs["prd"]["tempo"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("tempo = %+v, want %+v (%s)", got, want, body)
	}
	// e is not live: still not missing, since ignore is not catalog.
	g.set(tempoPrd, 200, tempoValues("a"))
	_, resp, body = postDrift(d, `{"env":"prd"}`)
	if got := resp.Envs["prd"]["tempo"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("tempo (e not live) = %+v, want %+v (%s)", got, want, body)
	}
}

func TestDrift_EnvFilter(t *testing.T) {
	d, g := driftSetup(t)
	g.set(lokiStg, 200, labelValues("a", "q"))
	_, resp, body := postDrift(d, `{"env":"stg"}`)
	if len(resp.Envs) != 1 || resp.Envs["stg"] == nil {
		t.Fatalf("envs = %v, want only stg (%s)", resp.Envs, body)
	}
	if got, want := resp.Envs["stg"]["loki"], (api.DriftResult{Unlisted: []string{"q"}, Missing: []string{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("stg loki = %+v, want %+v", got, want)
	}
	if g.request(lokiPrd) != nil {
		t.Error("prd was queried for env stg")
	}
	// Empty env = both.
	_, resp, _ = postDrift(d, `{}`)
	if len(resp.Envs) != 2 || resp.Envs["prd"] == nil || resp.Envs["stg"] == nil {
		t.Fatalf("empty env: envs = %v, want prd and stg", resp.Envs)
	}
	for _, env := range []string{"prd", "stg"} {
		if len(resp.Envs[env]) != 3 {
			t.Errorf("%s sources = %v, want loki, tempo, prometheus", env, resp.Envs[env])
		}
	}
}

func TestDrift_BadEnv(t *testing.T) {
	d, _ := driftSetup(t)
	for _, body := range []string{`{"env":"dev"}`, `not json`} {
		code, _, raw := postDrift(d, body)
		var e api.Error
		_ = json.Unmarshal([]byte(raw), &e)
		if code != 400 || e.Error != api.CodeInvalidBody {
			t.Errorf("body %q: got %d %s, want 400 %s", body, code, raw, api.CodeInvalidBody)
		}
	}
}

// TestDrift_RequestShape checks the auth header and the 24-hour window units
// each source receives: Loki ns, Tempo s, Prometheus unix seconds.
func TestDrift_RequestShape(t *testing.T) {
	d, g := driftSetup(t)
	postDrift(d, `{"env":"prd"}`)
	// driftNow = 2026-10-18T00:00:00Z = 1792281600; minus 24h = 1792195200.
	const end, start = "1792281600", "1792195200"
	for _, c := range []struct{ path, start, end string }{
		{lokiPrd, start + "000000000", end + "000000000"},
		{tempoPrd, start, end},
		{promPrd, start, end},
	} {
		r := g.request(c.path)
		if r == nil {
			t.Errorf("%s not requested", c.path)
			continue
		}
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer glsa_secret_token" {
			t.Errorf("%s: %s auth=%q", c.path, r.Method, r.Header.Get("Authorization"))
		}
		if q := r.URL.Query(); q.Get("start") != c.start || q.Get("end") != c.end {
			t.Errorf("%s: start=%s end=%s, want %s %s", c.path, q.Get("start"), q.Get("end"), c.start, c.end)
		}
	}
}

func TestDrift_CatalogUnavailable(t *testing.T) {
	gh := httptest.NewServer(&stub{status: 500, body: "boom"})
	t.Cleanup(gh.Close)
	h := New(config.Catalog{Repo: "org/monitoring", Path: "catalog.yaml", Ref: "main"}, "ghp_secret_token", WithBaseURL(gh.URL))
	code, _, raw := postDrift(NewDrift(h, "http://127.0.0.1:1", "glsa_secret_token"), `{}`)
	var e api.Error
	_ = json.Unmarshal([]byte(raw), &e)
	if code != 503 || e.Error != api.CodeCatalogUnavailable {
		t.Fatalf("got %d %s", code, raw)
	}
}

func TestDrift_AuditsEveryRequest(t *testing.T) {
	var buf bytes.Buffer
	log := &audit.Logger{W: &buf}
	d, _ := driftSetup(t, WithAudit(log))
	key := []byte("test-key-0123456789abcdef0123456")
	tok, _, err := auth.Issue(key, "octocat", "dev", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	call := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/catalog/drift", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		auth.RequireAudited(key, log, func(*http.Request) string { return "/catalog/drift" }, d).ServeHTTP(rec, req)
		return rec.Code
	}
	if c := call(`{"env":"prd"}`); c != 200 {
		t.Fatalf("first call = %d", c)
	}
	if c := call(`{"env":"dev"}`); c != 400 {
		t.Fatalf("second call = %d", c)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("audit rows = %d, want 2: %v", len(lines), lines)
	}
	for i, want := range []int{200, 400} {
		var e map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			t.Fatal(err)
		}
		if e["endpoint"] != "/catalog/drift" || e["login"] != "octocat" || e["role"] != "dev" || e["status"] != float64(want) {
			t.Errorf("row %d = %s, want status %d", i, lines[i], want)
		}
	}
	if strings.Contains(buf.String(), "glsa_secret_token") || strings.Contains(buf.String(), "ghp_secret_token") {
		t.Error("audit leaks a token")
	}
}
