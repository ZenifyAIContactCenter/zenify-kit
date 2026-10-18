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
	return f.postAs(t, login, api.RoleLead, req)
}

func (f *fixture) postAs(t *testing.T, login, role string, req api.QueryRequest) (int, api.QueryResponse, api.Error) {
	t.Helper()
	tok, _, _ := auth.Issue(testKey, login, role, time.Now())
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
	tok, _, _ := auth.Issue(testKey, "octocat", "lead", time.Now())
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
	optional := &Handler{Creds: testCreds(), Audit: &audit.Logger{W: io.Discard}, Run: okRun, MaxConcurrent: 2}
	optional.Creds.LumiMySQLStg, optional.Creds.LumiMySQLPrd = "", ""
	if err := optional.Validate(); err != nil { // SC-3: lumi creds optional
		t.Fatalf("empty lumi creds must validate: %v", err)
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

func TestQuery_DevPrdDataReadLeadOnly(t *testing.T) { // SC-20
	for name, req := range map[string]api.QueryRequest{
		"mongo find":           {Env: "prd", Engine: "mongo", Op: "find", Coll: "accounts"},
		"mongo doc":            {Env: "prd", Engine: "mongo", Op: "doc", Coll: "accounts"},
		"mongo aggregate":      {Env: "prd", Engine: "mongo", Op: "aggregate", Coll: "accounts", Pipeline: `[{"$match":{}}]`},
		"mysql select":         {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SELECT * FROM accounts"},
		"show processlist":     {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW PROCESSLIST"},
		"comment then select":  {Env: "prd", Engine: "mysql", Op: "sql", SQL: "/* x */ SELECT * FROM accounts"},
		"explain of a write":   {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN DELETE FROM accounts"},
		"explain no select":    {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN FORMAT=JSON accounts"},
		"audit target":         {Env: "prd", Engine: "mongo", Target: "audit", Op: "count", Coll: "audit_logs"},
		"desc for connection":  {Env: "prd", Engine: "mysql", Op: "sql", SQL: "DESC FOR CONNECTION 1"},
		"describe format conn": {Env: "prd", Engine: "mysql", Op: "sql", SQL: "DESCRIBE FORMAT=JSON FOR CONNECTION 1"},
		"explain for conn":     {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN FOR CONNECTION 1"},
		// Row channels a dev could open through plan/schema statements.
		"explain analyze":         {Env: "prd", Engine: "mysql", Op: "sql", SQL: "explain analyze select 1"},
		"explain tree analyze":    {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN FORMAT=TREE ANALYZE SELECT * FROM users"},
		"explain cte":             {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN FORMAT = JSON WITH t AS (SELECT 1) SELECT * FROM t"},
		"explain cte delete":      {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN WITH t AS (SELECT 1) DELETE FROM accounts"},
		"explain const subquery":  {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN SELECT * FROM users WHERE id = (SELECT 1 FROM users WHERE ASCII(password)>77)"},
		"show tables where bool":  {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW TABLES WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77"},
		"show index where bool":   {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW INDEX FROM users WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77"},
		"show columns where bool": {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW COLUMNS FROM users WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77"},
		"show status where bool":  {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW TABLE STATUS WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77"},
		"show tables where error": {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW TABLES WHERE EXTRACTVALUE(1,CONCAT(0x7e,(SELECT password FROM users LIMIT 1)))"},
		"desc subquery":           {Env: "prd", Engine: "mysql", Op: "sql", SQL: "DESC (SELECT password FROM users)"},
	} {
		f := newFixture(t, okRun)
		st, _, e := f.postAs(t, "devgal", api.RoleDev, req)
		if st != http.StatusForbidden || e.Error != api.CodeLeadOnly {
			t.Errorf("%s: got %d %+v, want 403 lead_only", name, st, e)
		}
		if req.Op != "count" && !strings.Contains(e.Message, api.DevPrdSummary()) { // count has its own filter message
			t.Errorf("%s: message %q lacks DevPrdSummary", name, e.Message)
		}
		if n := atomic.LoadInt32(f.calls); n != 0 {
			t.Errorf("%s: %d processes started, want 0", name, n)
		}
		var line audit.Entry
		if err := json.Unmarshal(f.auditBytes(), &line); err != nil || line.Role != api.RoleDev || line.Status != http.StatusForbidden || line.Login != "devgal" {
			t.Errorf("%s: audit = %s", name, f.auditBytes())
		}
	}
}

func TestQuery_DevPrdPlanAndSchemaRunAsLead(t *testing.T) { // SC-21
	for name, req := range map[string]api.QueryRequest{
		"mysql explain":        {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN SELECT * FROM accounts WHERE id=1"},
		"explain format":       {Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN FORMAT = JSON SELECT * FROM accounts WHERE email = 'x'"},
		"describe":             {Env: "prd", Engine: "mysql", Op: "sql", SQL: "DESCRIBE accounts"},
		"desc":                 {Env: "prd", Engine: "mysql", Op: "sql", SQL: "  desc accounts"},
		"desc select":          {Env: "prd", Engine: "mysql", Op: "sql", SQL: "DESC SELECT 1"},
		"describe backticked":  {Env: "prd", Engine: "mysql", Op: "sql", SQL: "DESCRIBE `accounts`"},
		"show index":           {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW INDEX FROM accounts"},
		"show keys":            {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW KEYS FROM accounts"},
		"show create table":    {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW CREATE TABLE accounts"},
		"show table status":    {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW TABLE STATUS"},
		"show columns":         {Env: "prd", Engine: "mysql", Op: "sql", SQL: "SHOW COLUMNS FROM accounts"},
		"tables":               {Env: "prd", Engine: "mysql", Op: "tables"},
		"mongo indexes":        {Env: "prd", Engine: "mongo", Op: "indexes", Coll: "accounts"},
		"mongo stats":          {Env: "prd", Engine: "mongo", Op: "stats", Coll: "accounts"},
		"mongo count":          {Env: "prd", Engine: "mongo", Op: "count", Coll: "accounts"},
		"mongo explain":        {Env: "prd", Engine: "mongo", Op: "explain", Coll: "accounts", Filter: `{"a":1}`},
		"mongo collections":    {Env: "prd", Engine: "mongo", Op: "collections"},
		"stg find (full read)": {Env: "stg", Engine: "mongo", Op: "find", Coll: "accounts"},
		"stg select":           {Env: "stg", Engine: "mysql", Op: "sql", SQL: "SELECT * FROM accounts"},
	} {
		f := newFixture(t, okRun)
		st, _, e := f.postAs(t, "devgal", api.RoleDev, req)
		if st != http.StatusOK {
			t.Errorf("%s: got %d %+v, want 200", name, st, e)
		}
		if n := atomic.LoadInt32(f.calls); n != 1 {
			t.Errorf("%s: %d processes started, want 1", name, n)
		}
	}
}

// The dev allowlist only narrows: the read-only guard still refuses what it
// refuses for a lead.
func TestQuery_DevGuardStillRuns(t *testing.T) {
	for name, sql := range map[string]string{
		"explain select into":  "EXPLAIN SELECT * FROM accounts INTO OUTFILE '/tmp/x'",
		"comment then explain": "/* x */ EXPLAIN SELECT 1",
	} {
		f := newFixture(t, okRun)
		st, _, e := f.postAs(t, "devgal", api.RoleDev, api.QueryRequest{Env: "prd", Engine: "mysql", Op: "sql", SQL: sql})
		if st != http.StatusBadRequest || e.Error != api.CodeInvalidQuery || atomic.LoadInt32(f.calls) != 0 {
			t.Errorf("%s: got %d %+v calls=%d, want 400 invalid_query and no process", name, st, e, atomic.LoadInt32(f.calls))
		}
	}
}

func TestQuery_IndexesAndStatsScripts(t *testing.T) {
	var script string
	f := newFixture(t, func(_ context.Context, _ string, args, _ []string, out, _ io.Writer) error {
		script = args[len(args)-1]
		return okRun(context.Background(), "", nil, nil, out, nil)
	})
	if st, _, e := f.post(t, "boss", api.QueryRequest{Env: "prd", Engine: "mongo", Op: "indexes", Coll: "accounts"}); st != http.StatusOK {
		t.Fatalf("indexes: %d %+v", st, e)
	}
	if !strings.Contains(script, `d.getCollection("accounts").getIndexes()`) {
		t.Errorf("indexes script = %s", script)
	}
	if st, _, e := f.post(t, "boss", api.QueryRequest{Env: "prd", Engine: "mongo", Op: "stats", Coll: "accounts"}); st != http.StatusOK {
		t.Fatalf("stats: %d %+v", st, e)
	}
	if !strings.Contains(script, `d.getCollection("accounts").stats()`) || !strings.Contains(script, "totalIndexSize:s.totalIndexSize") {
		t.Errorf("stats script = %s", script)
	}
}

// Exploits from the security review: a filtered count is an oracle, and an
// explain that executes surfaces documents through its error text.
func TestQuery_DevPrdMongoNoRowChannel(t *testing.T) {
	for name, filter := range map[string]string{
		"count regex oracle": `{"email":{"$regex":"^a"}}`,
		"count toInt error":  `{"$expr":{"$eq":[{"$toInt":"$email"},0]}}`,
		"count eq oracle":    `{"email":"a@b.c"}`,
	} {
		f := newFixture(t, okRun)
		st, _, e := f.postAs(t, "devgal", api.RoleDev, api.QueryRequest{Env: "prd", Engine: "mongo", Op: "count", Coll: "users", Filter: filter})
		if st != http.StatusForbidden || e.Error != api.CodeLeadOnly || !strings.Contains(e.Message, "estimatedDocumentCount") {
			t.Errorf("%s: got %d %+v, want 403 lead_only naming estimatedDocumentCount", name, st, e)
		}
		if n := atomic.LoadInt32(f.calls); n != 0 {
			t.Errorf("%s: %d processes started, want 0", name, n)
		}
	}

	script := func(role string, req api.QueryRequest) string {
		t.Helper()
		var s string
		f := newFixture(t, func(_ context.Context, _ string, args, _ []string, out, _ io.Writer) error {
			s = args[len(args)-1]
			return okRun(context.Background(), "", nil, nil, out, nil)
		})
		if st, _, e := f.postAs(t, "someone", role, req); st != http.StatusOK {
			t.Fatalf("%s %+v: %d %+v", role, req, st, e)
		}
		return s
	}
	where := api.QueryRequest{Env: "prd", Engine: "mongo", Op: "explain", Coll: "users", Filter: `{"$where":"throw new Error(tojson(this))"}`}
	pipe := api.QueryRequest{Env: "prd", Engine: "mongo", Op: "explain", Coll: "users",
		Pipeline: `[{"$addFields":{"x":{"$toInt":"$email"}}},{"$addFields":{"y":{"$function":{"body":"function(e){throw new Error(e)}","args":["$email"],"lang":"js"}}}}]`}
	for name, req := range map[string]api.QueryRequest{"explain $where": where, "explain pipeline": pipe} {
		if s := script(api.RoleDev, req); !strings.Contains(s, `explain("queryPlanner")`) || strings.Contains(s, "executionStats") {
			t.Errorf("dev %s: script = %s, want queryPlanner only", name, s)
		}
		if s := script(api.RoleLead, req); !strings.Contains(s, `explain("executionStats")`) {
			t.Errorf("lead %s: script = %s, want executionStats", name, s)
		}
	}
	stg := where
	stg.Env = "stg"
	if s := script(api.RoleDev, stg); !strings.Contains(s, `explain("executionStats")`) {
		t.Errorf("dev stg explain: script = %s, want executionStats", s)
	}
	if s := script(api.RoleDev, api.QueryRequest{Env: "prd", Engine: "mongo", Op: "count", Coll: "users"}); !strings.Contains(s, `d.getCollection("users").estimatedDocumentCount()`) {
		t.Errorf("dev count: script = %s, want estimatedDocumentCount", s)
	}
}

// A successful child can still write row content to stderr, so dev on prd
// gets none of it; lead and stg keep it.
func TestQuery_DevPrdSuccessDropsChildStderr(t *testing.T) {
	okRun := func(_ context.Context, _ string, _ []string, _ []string, out, errw io.Writer) error {
		_, _ = io.WriteString(out, "{}\n")
		_, _ = io.WriteString(errw, "warning: SECRET-ROW-CONTENT\n")
		return nil
	}
	for _, c := range []struct {
		role string
		env  string
		leak bool
	}{
		{api.RoleDev, "prd", false},
		{api.RoleLead, "prd", true},
		{api.RoleDev, "stg", true},
	} {
		f := newFixture(t, okRun)
		st, resp, e := f.postAs(t, "someone", c.role, api.QueryRequest{Env: c.env, Engine: "mongo", Op: "explain", Coll: "users"})
		if st != http.StatusOK {
			t.Fatalf("%s %s: got %d %+v", c.role, c.env, st, e)
		}
		if got := strings.Contains(resp.Stderr, "SECRET-ROW-CONTENT"); got != c.leak {
			t.Errorf("%s %s: stderr = %q, want child stderr shown=%v", c.role, c.env, resp.Stderr, c.leak)
		}
	}
}

// A server error can quote the row it failed on, so dev on prd gets a
// generic message; lead and stg keep the child's stderr.
func TestQuery_DevPrdFailureWithholdsStderr(t *testing.T) {
	failRun := func(_ context.Context, _ string, _ []string, _ []string, _, errw io.Writer) error {
		_, _ = io.WriteString(errw, "MongoServerError: SECRET-ROW-CONTENT\n")
		return errors.New("exit status 1")
	}
	for _, c := range []struct {
		role string
		req  api.QueryRequest
		leak bool
	}{
		{api.RoleDev, api.QueryRequest{Env: "prd", Engine: "mongo", Op: "explain", Coll: "users"}, false},
		{api.RoleDev, api.QueryRequest{Env: "prd", Engine: "mysql", Op: "sql", SQL: "EXPLAIN SELECT * FROM users"}, false},
		{api.RoleLead, api.QueryRequest{Env: "prd", Engine: "mongo", Op: "explain", Coll: "users"}, true},
		{api.RoleDev, api.QueryRequest{Env: "stg", Engine: "mongo", Op: "find", Coll: "users"}, true},
	} {
		f := newFixture(t, failRun)
		st, _, e := f.postAs(t, "someone", c.role, c.req)
		if st != http.StatusBadGateway || e.Error != api.CodeQueryFailed {
			t.Fatalf("%s %+v: got %d %+v", c.role, c.req, st, e)
		}
		if got := strings.Contains(e.Message, "SECRET-ROW-CONTENT"); got != c.leak {
			t.Errorf("%s %s %s: message = %q, want stderr shown=%v", c.role, c.req.Env, c.req.Op, e.Message, c.leak)
		}
		if !c.leak && e.Message != api.DevPrdErrorWithheld {
			t.Errorf("%s %s: message = %q, want the withheld notice", c.role, c.req.Op, e.Message)
		}
		if !strings.Contains(f.audit.String(), `"status":502`) {
			t.Errorf("%s %s: failure not audited: %s", c.role, c.req.Op, f.audit.String())
		}
	}
}

func TestQuery_PrdStderrHasNoEnvNames(t *testing.T) {
	f := newFixture(t, func(_ context.Context, _ string, _ []string, _ []string, out, _ io.Writer) error {
		_, _ = io.WriteString(out, "{\"a\":1}\n")
		return nil
	})
	st, resp, e := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mongo", Op: "count", Coll: "users"})
	if st != 200 {
		t.Fatalf("got %d %+v", st, e)
	}
	if strings.Contains(resp.Stderr, "MONGO_URL_PRD") {
		t.Errorf("stderr = %q, want no env var name", resp.Stderr)
	}
}

func TestQuery_TargetLumiUsesLumiURL(t *testing.T) {
	for _, tc := range []struct{ env, host, port, db string }{
		{"stg", "lumi-stg", "3307", "mass_chatbot_v2"},
		{"prd", "lumi-prd", "3306", "db_chatbot"},
	} {
		t.Run(tc.env, func(t *testing.T) {
			var gotArgs, gotEnv []string
			f := newFixture(t, func(_ context.Context, _ string, args, env []string, out, _ io.Writer) error {
				gotArgs, gotEnv = args, env
				return okRun(nil, "", nil, nil, out, nil)
			})
			f.h.Creds.LumiMySQLStg = "mysql+pymysql://lu:lp@lumi-stg:3307/mass_chatbot_v2"
			f.h.Creds.LumiMySQLPrd = "mysql+pymysql://lu:lp@lumi-prd:3306/db_chatbot"
			st, _, e := f.post(t, "octocat", api.QueryRequest{Env: tc.env, Engine: "mysql", Target: "lumi", Op: "sql", SQL: "SELECT 1"})
			if st != http.StatusOK {
				t.Fatalf("got %d %+v", st, e)
			}
			args := strings.Join(gotArgs, " ")
			if !strings.Contains(args, "-h "+tc.host+" -P "+tc.port+" -u lu") || !strings.HasSuffix(strings.TrimSpace(args), tc.db) {
				t.Errorf("argv = %q", args)
			}
			if !strings.Contains(strings.Join(gotEnv, "\n"), "MYSQL_PWD=lp") {
				t.Errorf("env lacks MYSQL_PWD=lp: %v", gotEnv)
			}
			if strings.Contains(args, "fake-stg") || strings.Contains(args, "fake-prd") {
				t.Errorf("argv used the main URL: %q", args)
			}
		})
	}
}

func TestQuery_TargetLumiTables(t *testing.T) {
	var gotArgs []string
	f := newFixture(t, func(_ context.Context, _ string, args, _ []string, out, _ io.Writer) error {
		gotArgs = args
		return okRun(nil, "", nil, nil, out, nil)
	})
	f.h.Creds.LumiMySQLPrd = "mysql+pymysql://lu:lp@lumi-prd:3306/db_chatbot"
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mysql", Target: "lumi", Op: "tables"})
	if st != http.StatusOK || !strings.Contains(strings.Join(gotArgs, " "), "lumi-prd") {
		t.Fatalf("got %d %+v args=%v", st, e, gotArgs)
	}
}

func TestQuery_TargetLumiMongoInvalid(t *testing.T) {
	f := newFixture(t, okRun)
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "prd", Engine: "mongo", Target: "lumi", Op: "collections"})
	if st != http.StatusBadRequest || e.Error != api.CodeInvalidTarget || atomic.LoadInt32(f.calls) != 0 {
		t.Fatalf("got %d %+v calls=%d", st, e, atomic.LoadInt32(f.calls))
	}
}

func TestQuery_TargetLumiMissingCredential500(t *testing.T) {
	f := newFixture(t, okRun)
	st, _, e := f.post(t, "octocat", api.QueryRequest{Env: "stg", Engine: "mysql", Target: "lumi", Op: "tables"})
	if st != http.StatusInternalServerError || e.Error != api.CodeBrokerMisconfigured || atomic.LoadInt32(f.calls) != 0 {
		t.Fatalf("got %d %+v calls=%d", st, e, atomic.LoadInt32(f.calls))
	}
}

func TestQuery_TargetLumiStderrMasked(t *testing.T) {
	for _, env := range []string{"stg", "prd"} {
		t.Run(env, func(t *testing.T) {
			f := newFixture(t, func(_ context.Context, _ string, _ []string, _ []string, _, errw io.Writer) error {
				_, _ = io.WriteString(errw, "auth failed for lumi_user (pw lumi%2Fpass = lumi/pass) at lumi-"+env+".example.test\n")
				return errors.New("exit status 1")
			})
			f.h.Creds.LumiMySQLStg = "mysql+pymysql://lumi_user:lumi%2Fpass@lumi-stg.example.test:3307/mass_chatbot_v2"
			f.h.Creds.LumiMySQLPrd = "mysql+pymysql://lumi_user:lumi%2Fpass@lumi-prd.example.test:3306/db_chatbot"
			st, _, e := f.post(t, "octocat", api.QueryRequest{Env: env, Engine: "mysql", Target: "lumi", Op: "sql", SQL: "SELECT 1"})
			if st != http.StatusBadGateway || e.Error != api.CodeQueryFailed {
				t.Fatalf("got %d %+v", st, e)
			}
			for _, leak := range []string{"lumi_user", "lumi%2Fpass", "lumi/pass", "lumi-" + env + ".example.test"} {
				if strings.Contains(e.Message, leak) {
					t.Errorf("message leaks %q: %q", leak, e.Message)
				}
			}
		})
	}
}

func TestQuery_TargetLumiDevPrdLeadOnly(t *testing.T) {
	f := newFixture(t, okRun)
	st, _, e := f.postAs(t, "octocat", api.RoleDev, api.QueryRequest{Env: "prd", Engine: "mysql", Target: "lumi", Op: "sql", SQL: "SELECT 1"})
	if st != http.StatusForbidden || e.Error != api.CodeLeadOnly {
		t.Fatalf("got %d %+v", st, e)
	}
}

func TestMySQLScheme(t *testing.T) {
	for in, want := range map[string]string{
		"mysql+pymysql://u:p@h:1/d": "mysql://u:p@h:1/d",
		"mysql://u:p@h:1/d":         "mysql://u:p@h:1/d",
		"":                          "",
	} {
		if got := mysqlScheme(in); got != want {
			t.Errorf("mysqlScheme(%q) = %q, want %q", in, got, want)
		}
	}
}
