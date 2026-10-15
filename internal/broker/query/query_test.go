package query

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

var testKey = []byte("test-key-0123456789abcdef0123456")

const (
	fakeAuditURL = "mongodb://fake-audit-prd"
	fakePrdURL   = "mongodb://fake-prd"
)

func testCreds() Creds {
	return Creds{
		MongoStg: "mongodb://fake-stg", MongoPrd: fakePrdURL,
		MySQLStg: "mysql://u:p@fake-stg:3306/db", MySQLPrd: "mysql://u:p@fake-prd:3306/db",
		AuditMongoPrd: fakeAuditURL, MainDB: "3csoft", AuditDB: "audit_logs",
	}
}

type fixture struct {
	h     *Handler
	audit *bytes.Buffer
	calls *int32
	ts    *httptest.Server
}

// newFixture serves h behind auth.Require, so the login comes from a real JWT.
func newFixture(t *testing.T, run func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error) *fixture {
	t.Helper()
	var calls int32
	buf := &bytes.Buffer{}
	f := &fixture{audit: buf, calls: &calls}
	f.h = &Handler{Creds: testCreds(), Audit: &audit.Logger{W: buf}, MaxConcurrent: 2,
		Run: func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error {
			atomic.AddInt32(&calls, 1)
			return run(ctx, name, args, env, out, errw)
		}}
	f.ts = httptest.NewServer(auth.Require(testKey, f.h))
	t.Cleanup(f.ts.Close)
	return f
}

func okRun(_ context.Context, _ string, _ []string, _ []string, out, _ io.Writer) error {
	_, _ = io.WriteString(out, "{\"a\":1}\n{\"b\":2}\n")
	return nil
}

func (f *fixture) post(t *testing.T, login string, req api.QueryRequest) (int, api.QueryResponse, api.Error) {
	t.Helper()
	tok, _, _ := auth.Issue(testKey, login, time.Now())
	b, _ := json.Marshal(req)
	r, _ := http.NewRequest(http.MethodPost, f.ts.URL, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var ok api.QueryResponse
	var bad api.Error
	if resp.StatusCode == http.StatusOK {
		_ = json.Unmarshal(raw, &ok)
	} else {
		_ = json.Unmarshal(raw, &bad)
	}
	return resp.StatusCode, ok, bad
}

func TestQuery_SQLDeleteRejectedNoSpawn(t *testing.T) { // SC-3
	f := newFixture(t, okRun)
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mysql", Op: "sql", SQL: "DELETE FROM t"})
	if st != 400 || e.Error != api.CodeInvalidQuery || e.Message == "" {
		t.Fatalf("got %d %+v", st, e)
	}
	if n := atomic.LoadInt32(f.calls); n != 0 {
		t.Fatalf("seam called %d times, want 0", n)
	}
}

func TestQuery_MergeDepth2Rejected(t *testing.T) { // SC-4
	f := newFixture(t, okRun)
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "aggregate", Coll: "tickets",
		Pipeline: `[{"$facet":{"a":[{"$merge":"x"}]}}]`})
	if st != 400 || e.Error != api.CodeInvalidQuery {
		t.Fatalf("got %d %+v", st, e)
	}
	if atomic.LoadInt32(f.calls) != 0 {
		t.Fatal("seam must not run")
	}
}

func TestQuery_TargetAuditInvalid(t *testing.T) { // SC-14
	f := newFixture(t, okRun)
	for _, r := range []api.QueryRequest{
		{Env: "prd", Engine: "mysql", Target: "audit", Op: "tables"},
		{Env: "stg", Engine: "mongo", Target: "audit", Op: "collections"},
		{Env: "prd", Engine: "mongo", Target: "other", Op: "collections"},
	} {
		st, _, e := f.post(t, "octocat", r)
		if st != 400 || e.Error != api.CodeInvalidTarget {
			t.Fatalf("%+v: got %d %+v", r, st, e)
		}
	}
	if atomic.LoadInt32(f.calls) != 0 {
		t.Fatal("seam must not run")
	}
}

func TestQuery_TargetAuditUsesAuditDB(t *testing.T) {
	var gotEnv []string
	var gotArgs []string
	f := newFixture(t, func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error {
		gotArgs, gotEnv = args, env
		return okRun(ctx, name, args, env, out, errw)
	})
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mongo", Target: "audit", Op: "collections"})
	if st != 200 {
		t.Fatalf("got %d %+v", st, e)
	}
	if !strings.Contains(strings.Join(gotEnv, "\n"), "ZNF_MONGO_URL="+fakeAuditURL) {
		t.Fatalf("env lacks audit url: %v", gotEnv)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), `getSiblingDB("audit_logs")`) {
		t.Fatalf("script lacks audit db: %v", gotArgs)
	}
}

func TestQuery_MainUsesMainDBAndProdURL(t *testing.T) {
	var gotEnv, gotArgs []string
	f := newFixture(t, func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error {
		gotArgs, gotEnv = args, env
		return okRun(ctx, name, args, env, out, errw)
	})
	st, _, _ := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mongo", Op: "collections"})
	if st != 200 || !strings.Contains(strings.Join(gotEnv, "\n"), "ZNF_MONGO_URL="+fakePrdURL) ||
		!strings.Contains(strings.Join(gotArgs, " "), `getSiblingDB("3csoft")`) {
		t.Fatalf("got %d env=%v args=%v", st, gotEnv, gotArgs)
	}
}

func TestQuery_ConcurrentLimit(t *testing.T) { // SC-12
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	f := newFixture(t, func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error {
		started <- struct{}{}
		<-release
		return okRun(ctx, name, args, env, out, errw)
	})
	req := api.QueryRequest{Env: "stg", Engine: "mongo", Op: "collections"}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); st, _, _ := f.post(t, "octocat", req); codes <- st }()
	}
	<-started
	<-started
	st, _, e := f.post(t, "octocat", req)
	if st != 429 || e.Error != api.CodeTooManyConcurrent {
		t.Fatalf("3rd request: got %d %+v", st, e)
	}
	// A different login is unaffected.
	other := make(chan int, 1)
	go func() { st, _, _ := f.post(t, "hubot", req); other <- st }()
	<-started
	close(release)
	wg.Wait()
	if st := <-other; st != 200 {
		t.Fatalf("other login: got %d", st)
	}
	for i := 0; i < 2; i++ {
		if st := <-codes; st != 200 {
			t.Fatalf("held request: got %d", st)
		}
	}
	// Slots are released afterwards.
	if st, _, _ := f.post(t, "octocat", req); st != 200 {
		t.Fatalf("after release: got %d", st)
	}
}

func TestQuery_Timeout(t *testing.T) {
	f := newFixture(t, func(ctx context.Context, _ string, _ []string, _ []string, _, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	})
	f.h.Timeout = 50 * time.Millisecond
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "collections"})
	if st != http.StatusGatewayTimeout || e.Error != api.CodeTimeout {
		t.Fatalf("got %d %+v", st, e)
	}
}

func TestQuery_AuditLine(t *testing.T) {
	f := newFixture(t, okRun)
	st, resp, _ := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mongo", Target: "audit", Op: "collections"})
	if st != 200 || resp.Rows != 2 || resp.Truncated || resp.Stdout != "{\"a\":1}\n{\"b\":2}\n" {
		t.Fatalf("got %d %+v", st, resp)
	}
	lines := strings.Split(strings.TrimSpace(f.audit.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 audit line, got %d: %q", len(lines), f.audit.String())
	}
	var e audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Login != "octocat" || e.Endpoint != "/query" || e.Env != "prd" || e.Op != "collections" ||
		e.Rows != 2 || e.Status != 200 || e.Bytes != len(resp.Stdout) {
		t.Fatalf("entry: %+v", e)
	}
	for _, bad := range []string{"fake-audit", "fake-prd", `"a":1`} {
		if strings.Contains(lines[0], bad) {
			t.Fatalf("audit line leaks %q: %s", bad, lines[0])
		}
	}
}

func TestQuery_FailureAlsoAudited(t *testing.T) {
	f := newFixture(t, okRun)
	f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mysql", Op: "sql", SQL: "DELETE FROM t"})
	var e audit.Entry
	if err := json.Unmarshal(bytes.TrimSpace(f.audit.Bytes()), &e); err != nil || e.Status != 400 || e.Login != "octocat" {
		t.Fatalf("entry %+v err %v raw %q", e, err, f.audit.String())
	}
}

func TestChildEnv_OnlyPathHomeAndExtra(t *testing.T) {
	getenv := func(k string) string {
		return map[string]string{"PATH": "/bin", "HOME": "/h", "BROKER_JWT_KEY": "secret", "MONGO_URL_PRD": "x"}[k]
	}
	got := childEnv(getenv, []string{"ZNF_MONGO_URL=mongodb://fake"})
	want := map[string]bool{"PATH=/bin": true, "HOME=/h": true, "ZNF_MONGO_URL=mongodb://fake": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, kv := range got {
		if !want[kv] {
			t.Fatalf("unexpected env entry %q", kv)
		}
	}
}
