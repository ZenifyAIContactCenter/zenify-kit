package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	audit *syncBuffer
	calls *int32
	ts    *httptest.Server
}

// newFixture serves h behind auth.Require, so the login comes from a real JWT.
func newFixture(t *testing.T, run func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error) *fixture {
	t.Helper()
	var calls int32
	buf := &syncBuffer{}
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

// syncBuffer is a bytes.Buffer safe to read while the handler may still write.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b.Bytes()...)
}

func (s *syncBuffer) String() string { return string(s.Bytes()) }

func (f *fixture) auditLen() int      { return len(f.audit.Bytes()) }
func (f *fixture) auditBytes() []byte { return f.audit.Bytes() }

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

func TestQuery_FailedStderrMasksURLParts(t *testing.T) {
	f := newFixture(t, func(_ context.Context, _ string, _ []string, _ []string, _, errw io.Writer) error {
		_, _ = io.WriteString(errw, "auth failed for readonly_user (pw s3cret%2Fpw = s3cret/pw) at db-stg.example.test\n")
		return errors.New("exit status 1")
	})
	f.h.Creds.MySQLStg = "mysql://readonly_user:s3cret%2Fpw@db-stg.example.test:3306/db"
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mysql", Op: "sql", SQL: "SELECT 1"})
	if st != http.StatusBadGateway || e.Error != api.CodeQueryFailed {
		t.Fatalf("got %d %+v", st, e)
	}
	for _, leak := range []string{"readonly_user", "s3cret%2Fpw", "s3cret/pw", "db-stg.example.test"} {
		if strings.Contains(e.Message, leak) {
			t.Errorf("message leaks %q: %q", leak, e.Message)
		}
	}
	if !strings.Contains(e.Message, "auth failed for ***") {
		t.Errorf("message = %q, want the masked stderr", e.Message)
	}
}

func TestQuery_TruncatedMapped(t *testing.T) {
	t.Run("sql row cap", func(t *testing.T) {
		f := newFixture(t, func(_ context.Context, _ string, _ []string, _ []string, out, _ io.Writer) error {
			for i := 0; i < 10; i++ {
				if _, err := io.WriteString(out, "row\n"); err != nil {
					return err
				}
			}
			return nil
		})
		st, resp, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mysql", Op: "sql", SQL: "SELECT 1", Limit: 3})
		if st != 200 || !resp.Truncated || resp.Rows != 3 {
			t.Fatalf("got %d %+v %+v", st, resp, e)
		}
	})
	t.Run("mongo notice", func(t *testing.T) {
		f := newFixture(t, func(_ context.Context, _ string, _ []string, _ []string, out, errw io.Writer) error {
			_, _ = io.WriteString(out, "{}\n")
			// dbread's truncNotice for --limit 5, printed by the generated script.
			_, _ = fmt.Fprintf(errw, "… đã cắt ở %d dòng (--limit, tối đa %d)\n", 5, 200) //znf:allow-lang
			return nil
		})
		st, resp, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "find", Coll: "users", Limit: 5})
		if st != 200 {
			t.Fatalf("got %d %+v", st, e)
		}
		if !resp.Truncated {
			t.Fatalf("Truncated=%v with stderr %q", resp.Truncated, resp.Stderr)
		}
	})
}

func TestQuery_OversizedBody400(t *testing.T) {
	f := newFixture(t, okRun)
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "find", Coll: "users",
		Filter: `{"a":"` + strings.Repeat("x", maxBody) + `"}`})
	if st != http.StatusBadRequest || e.Error != api.CodeInvalidQuery {
		t.Fatalf("got %d %+v", st, e)
	}
	if atomic.LoadInt32(f.calls) != 0 {
		t.Fatal("seam must not run")
	}
}

func TestQuery_StgUsesStgURL(t *testing.T) {
	var gotEnv []string
	f := newFixture(t, func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error {
		gotEnv = env
		return okRun(ctx, name, args, env, out, errw)
	})
	if st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "collections"}); st != 200 {
		t.Fatalf("got %d %+v", st, e)
	}
	if got := strings.Join(gotEnv, "\n"); !strings.Contains(got, "ZNF_MONGO_URL=mongodb://fake-stg") || strings.Contains(got, fakePrdURL) {
		t.Fatalf("env = %v, want the stg URL only", gotEnv)
	}
}

func TestQuery_TimeoutAudited504(t *testing.T) {
	f := newFixture(t, func(ctx context.Context, _ string, _ []string, _ []string, _, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	})
	f.h.Timeout = 50 * time.Millisecond
	if st, _, _ := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "collections"}); st != http.StatusGatewayTimeout {
		t.Fatalf("got %d", st)
	}
	var e audit.Entry
	if err := json.Unmarshal(bytes.TrimSpace(f.audit.Bytes()), &e); err != nil || e.Status != 504 || e.Endpoint != "/query" || e.Login != "octocat" {
		t.Fatalf("entry %+v err %v", e, err)
	}
}

func TestQuery_SuccessNearDeadlineIs200(t *testing.T) {
	f := newFixture(t, func(ctx context.Context, name string, args, env []string, out, errw io.Writer) error {
		<-ctx.Done() // finishes only after the deadline, but without an error
		return okRun(ctx, name, args, env, out, errw)
	})
	f.h.Timeout = 30 * time.Millisecond
	if st, resp, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mongo", Op: "collections"}); st != 200 || resp.Rows != 2 {
		t.Fatalf("got %d %+v %+v", st, resp, e)
	}
}

func TestQuery_ClientCancelAudited499(t *testing.T) {
	started := make(chan struct{})
	f := newFixture(t, func(ctx context.Context, _ string, _ []string, _ []string, _, _ io.Writer) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	tok, _, _ := auth.Issue(testKey, "octocat", time.Now())
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, f.ts.URL, strings.NewReader(`{"env":"stg","engine":"mongo","op":"collections"}`))
	r.Header.Set("Authorization", "Bearer "+tok)
	go func() { <-started; cancel() }()
	if resp, err := http.DefaultClient.Do(r); err == nil {
		_ = resp.Body.Close()
		t.Fatalf("got a response %d, want the client to give up", resp.StatusCode)
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.auditLen() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	var e audit.Entry
	if err := json.Unmarshal(bytes.TrimSpace(f.auditBytes()), &e); err != nil || e.Status != statusClientClosed {
		t.Fatalf("entry %+v err %v, want status 499", e, err)
	}
}

func TestQuery_MissingCredential500(t *testing.T) {
	f := newFixture(t, okRun)
	f.h.Creds.MySQLPrd = ""
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mysql", Op: "tables"})
	if st != http.StatusInternalServerError || e.Error != api.CodeBrokerMisconfigured || strings.Contains(e.Message, "zenify up") {
		t.Fatalf("got %d %+v", st, e)
	}
	if atomic.LoadInt32(f.calls) != 0 {
		t.Fatal("seam must not run")
	}
}

func TestHandler_Validate(t *testing.T) {
	ok := &Handler{Creds: testCreds(), Audit: &audit.Logger{W: io.Discard}, Run: okRun, MaxConcurrent: 2}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid handler: %v", err)
	}
	bad := &Handler{Creds: testCreds()}
	bad.Creds.AuditMongoPrd = ""
	err := bad.Validate()
	for _, want := range []string{"Audit is nil", "Run is nil", "MaxConcurrent", "Creds.AuditMongoPrd"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() = %v, want it to name %q", err, want)
		}
	}
	if err != nil && strings.Contains(err.Error(), fakePrdURL) {
		t.Error("Validate leaks a credential")
	}
}

func TestExecRun_GrandchildCannotOutliveDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var out, errw bytes.Buffer
	start := time.Now()
	// The background sleep inherits stdout and keeps the pipe open after sh is killed.
	err := ExecRun(ctx, "sh", []string{"-c", "sleep 10 & sleep 10"}, nil, &out, &errw)
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("ExecRun returned after %v, want about deadline+waitDelay", el)
	}
	if err == nil {
		t.Fatal("want an error from the killed child")
	}
}
