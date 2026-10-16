package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/spf13/cobra"
)

// brokerFixture serves /token plus the handlers given, points the CLI's
// broker client at it, and returns a manifest path that declares a broker.
// Commands reach it only through an explicit --manifest, or through
// useTrustedManifest for the ones without that flag (e2e, visual).
func brokerFixture(t *testing.T, handlers map[string]http.HandlerFunc) (manifestPath string, hits *int32) {
	t.Helper()
	hits = new(int32)
	mux := http.NewServeMux()
	if _, own := handlers["/token"]; !own {
		mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(api.TokenResponse{Token: "test-jwt", ExpiresAt: time.Now().Add(15 * time.Minute).Format(time.RFC3339), Login: "octo", Role: "dev"})
		})
	}
	for path, h := range handlers {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(hits, 1)
			h(w, r)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	home := t.TempDir()
	saved := tuneBrokerClient
	tuneBrokerClient = func(c *brokerclient.Client) {
		c.URL, c.Home, c.GHToken = srv.URL, home, stubGHToken
	}
	t.Cleanup(func() { tuneBrokerClient = saved })
	manifestPath = filepath.Join(t.TempDir(), "repos.yaml")
	y := "org: o\nrepos:\n  - name: r\n    url: u\nbroker:\n  url: https://broker.example\n"
	if err := os.WriteFile(manifestPath, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, hits
}

type panicRunner struct{}

func (panicRunner) Run(...string) ([]byte, error) {
	panic("test hermeticity: a cli test reached the real gh runner; inject a ghx.Runner fake")
}

// useGH swaps the package gh runner for r until the test ends.
func useGH(t *testing.T, r ghx.Runner) {
	t.Helper()
	saved := ghRunner
	ghRunner = r
	t.Cleanup(func() { ghRunner = saved })
}

func stubGHToken(context.Context) (string, error) { return "test-gh", nil }

const noBrokerYAML = "org: o\nrepos:\n  - name: r\n    url: u\n"

// credentialEnv is every DB/E2E credential key a command could read locally.
var credentialEnv = []string{"MONGO_URL", "MYSQL_URL", "MONGO_URL_PRD", "MYSQL_URL_PRD", "AUDIT_MONGO_URL_PRD", "REDIS_URL_PRD",
	"E2E_DOMAIN", "E2E_EMAIL", "E2E_PASSWORD", "POSTHOG_API_KEY"}

// TestMain makes the whole package hermetic before any test runs:
//   - the GitHub token source errors and the gh runner panics;
//   - the trusted broker manifest declares no broker, and tuneBrokerClient
//     panics, so only a test that installs brokerFixture reaches a broker;
//   - HOME and ZENIFY_HOME are empty temp dirs (no token cache, no workspace
//     pointer), with the Go caches pinned so `go build` children still work;
//   - DB/E2E credential env vars are unset;
//   - PATH starts with failing shims for mongosh, mysql, gh and claude, and
//     the run fails if any test executed one;
//   - the workspace marker walk-up refuses the real workspace above this
//     package, so no test loads its .zenify/.env or .zenify-overlay.yaml.
func TestMain(m *testing.M) {
	os.Exit(runHermetic(m))
}

func runHermetic(m *testing.M) int {
	ghToken = func(context.Context) (string, error) {
		return "", fmt.Errorf("test hermeticity: a cli test reached the real gh token source; use brokerFixture")
	}
	ghRunner = panicRunner{}
	trustedBrokerManifest = func() ([]byte, string) { return []byte(noBrokerYAML), "test-no-broker" }
	tuneBrokerClient = func(*brokerclient.Client) {
		panic("test hermeticity: a cli test built a broker client without brokerFixture")
	}

	root, err := os.MkdirTemp("", "zenify-cli-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	if home, err := os.UserHomeDir(); err == nil {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			gopath = filepath.Join(home, "go")
		}
		setDefault("GOPATH", gopath)
		setDefault("GOMODCACHE", filepath.Join(gopath, "pkg", "mod"))
		if cache, err := os.UserCacheDir(); err == nil {
			setDefault("GOCACHE", filepath.Join(cache, "go-build"))
		}
	}
	for _, k := range []string{"HOME", "ZENIFY_HOME"} {
		d := filepath.Join(root, k)
		mustMkdir(d)
		_ = os.Setenv(k, d)
	}
	for _, k := range credentialEnv {
		_ = os.Unsetenv(k)
	}
	shims := filepath.Join(root, "shims")
	mustMkdir(shims)
	hitLog := filepath.Join(root, "shim-hits")
	for _, n := range []string{"mongosh", "mysql", "gh", "claude"} {
		script := "#!/bin/sh\necho " + n + " \"$*\" >> '" + hitLog + "'\necho 'test hermeticity: a cli test ran the real " + n + "' >&2\nexit 97\n"
		if err := os.WriteFile(filepath.Join(shims, n), []byte(script), 0o700); err != nil { //nolint:gosec // shim must be executable
			panic(err)
		}
	}
	_ = os.Setenv("PATH", shims+string(os.PathListSeparator)+os.Getenv("PATH"))
	wd, err := os.Getwd()
	if err != nil {
		panic("test hermeticity: cannot resolve the package dir, so the real workspace cannot be fenced off: " + err.Error())
	}
	if real, ok := findWorkspaceRoot(wd); ok {
		orig := findWorkspaceRoot
		findWorkspaceRoot = func(start string) (string, bool) {
			if r, ok := orig(start); ok && r != real {
				return r, true
			}
			return "", false
		}
	}
	code := m.Run()
	if hits, err := os.ReadFile(hitLog); err == nil && len(hits) > 0 { //nolint:gosec // test-owned temp file
		fmt.Fprintf(os.Stderr, "test hermeticity: tests ran shimmed binaries:\n%s", hits)
		return 1
	}
	return code
}

func setDefault(k, v string) {
	if os.Getenv(k) == "" {
		_ = os.Setenv(k, v)
	}
}

func mustMkdir(d string) {
	if err := os.MkdirAll(d, 0o700); err != nil {
		panic(err)
	}
}

// fakeClaudeNoMCP puts a `claude` first on PATH that knows no MCP server, for
// tests that run the read-only playwright check (`claude mcp get playwright`).
func fakeClaudeNoMCP(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho 'No MCP server found' >&2\nexit 1\n"), 0o700); err != nil { //nolint:gosec // fake must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestMain's guard: from the package dir (inside the real workspace) no test
// resolves a workspace, so none can load its .zenify/.env or overlay.
func TestHermetic_PackageDirResolvesNoWorkspace(t *testing.T) {
	if p := workspaceEnvPath(); p != "" {
		t.Fatalf("workspaceEnvPath() = %q, want \"\"", p)
	}
	if m := workspaceManifest(); m != nil {
		t.Fatalf("workspaceManifest() = %+v, want nil", m)
	}
}

// useTrustedManifest makes the manifest at path the trusted broker source (the
// embedded one in production) until the test ends.
func useTrustedManifest(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test-owned temp file
	if err != nil {
		t.Fatal(err)
	}
	saved := trustedBrokerManifest
	trustedBrokerManifest = func() ([]byte, string) { return b, path }
	t.Cleanup(func() { trustedBrokerManifest = saved })
}

// hermeticDBRead keeps a db-read command test away from real databases on top
// of TestMain: cwd is an empty temp dir, ZENIFY_HOME and HOME are fresh, PATH
// has no gh/mongosh/mysql at all, and every credential key is blank, so a test
// that wrongly takes the local path fails on "no workspace"/"not set".
func hermeticDBRead(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	for _, k := range credentialEnv {
		t.Setenv(k, "")
	}
}

func noBrokerManifest(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "repos.yaml")
	if err := os.WriteFile(p, []byte(noBrokerYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func execCmd(cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	var o, e bytes.Buffer
	cmd.SetOut(&o)
	cmd.SetErr(&e)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return o.String(), e.String(), err
}

func TestDBReadCmd_BrokerPrd(t *testing.T) {
	hermeticDBRead(t)
	var got api.QueryRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-jwt" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(api.QueryResponse{Stdout: "{\"a\":1}\n", Stderr: "note\n", Rows: 1})
	}})
	out, errOut, err := execCmd(newDBReadCmd(), "--manifest", mp, "--prd", "find", "users", "--limit", "1")
	if err != nil {
		t.Fatal(err)
	}
	want := api.QueryRequest{Env: "prd", Engine: "mongo", Op: "find", Coll: "users", Limit: 1}
	if got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
	if out != "{\"a\":1}\n" || errOut != "zenify db-read: production (via broker)\nnote\n" {
		t.Errorf("stdout=%q stderr=%q", out, errOut)
	}
}

func TestDBReadCmd_BrokerSQLMapsToMySQL(t *testing.T) {
	hermeticDBRead(t)
	var got api.QueryRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(api.QueryResponse{})
	}})
	if _, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "sql", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	want := api.QueryRequest{Env: "stg", Engine: "mysql", Op: "sql", SQL: "SELECT 1", Limit: 50}
	if got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
}

// tables keeps its substring filter in Coll on the broker path.
func TestDBReadCmd_BrokerTablesKeepsFilter(t *testing.T) {
	hermeticDBRead(t)
	var got api.QueryRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(api.QueryResponse{})
	}})
	out, errOut, err := execCmd(newDBReadCmd(), "--manifest", mp, "tables", "foo")
	if err != nil {
		t.Fatal(err)
	}
	want := api.QueryRequest{Env: "stg", Engine: "mysql", Op: "tables", Coll: "foo", Limit: 50}
	if got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
	if out != "" || errOut != "" { // no production notice on staging
		t.Errorf("stdout=%q stderr=%q", out, errOut)
	}
}

func TestDBReadCmd_BrokerValidatesLocally(t *testing.T) {
	hermeticDBRead(t)
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(http.ResponseWriter, *http.Request) {}})
	_, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "find", "users", "--limit", "500")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v (code %d), want BadArgs", err, exitcode.Code(err))
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Errorf("broker got %d requests, want 0", n)
	}
}

func TestDBReadCmd_Broker400IsBadArgs(t *testing.T) {
	hermeticDBRead(t)
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeInvalidQuery, Message: "nope"})
	}})
	_, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "count", "users")
	if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

// A dev's lead-only request prints FR-14.6's message.
func TestDBReadCmd_BrokerLeadOnly(t *testing.T) {
	hermeticDBRead(t)
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeLeadOnly, Message: "production data reads are lead-only"})
	}})
	_, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "--prd", "find", "users")
	if err == nil || !strings.Contains(err.Error(), "lệnh này chỉ dành cho lead (team leaders); dev dùng explain/indexes/stats/count") || exitcode.Code(err) == exitcode.BadArgs { //znf:allow-lang
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

// indexes and stats are Mongo ops sent with the collection, no flags.
func TestDBReadCmd_BrokerIndexesAndStats(t *testing.T) {
	hermeticDBRead(t)
	var got api.QueryRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(api.QueryResponse{})
	}})
	for _, op := range []string{"indexes", "stats"} {
		if _, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "--prd", op, "users"); err != nil {
			t.Fatal(err)
		}
		want := api.QueryRequest{Env: "prd", Engine: "mongo", Op: op, Coll: "users", Limit: 50}
		if got != want {
			t.Errorf("request = %+v, want %+v", got, want)
		}
	}
	if _, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "stats", "users", "--filter", `{"a":1}`); exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("stats --filter: err = %v, want BadArgs", err)
	}
}

// A misconfigured broker (500 broker_misconfigured) surfaces its own message:
// not BadArgs, and no WARP hint.
func TestDBReadCmd_BrokerMisconfigured(t *testing.T) {
	hermeticDBRead(t)
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeBrokerMisconfigured, Message: "no credential for target audit"})
	}})
	_, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "count", "users")
	if err == nil || exitcode.Code(err) == exitcode.BadArgs || !strings.Contains(err.Error(), "no credential for target audit") || strings.Contains(err.Error(), "WARP") {
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

// No broker: db-read takes the v0.38.0 local path. A real run would open the
// workspace's databases, so the test checks the routing decision and that a
// local validation error still surfaces unchanged.
func TestDBReadCmd_NoBrokerLocal(t *testing.T) {
	hermeticDBRead(t)
	called := false
	saved := tuneBrokerClient
	tuneBrokerClient = func(*brokerclient.Client) { called = true }
	t.Cleanup(func() { tuneBrokerClient = saved })
	mp := noBrokerManifest(t)
	if c, ok, err := brokerFromFlags(mp, ""); ok || c != nil || err != nil {
		t.Fatalf("brokerFromFlags = %v, %v, %v; want nil, false, nil", c, ok, err)
	}
	_, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "find", "users", "--limit", "500")
	if exitcode.Code(err) != exitcode.BadArgs || called {
		t.Errorf("err = %v (code %d), broker client built = %v", err, exitcode.Code(err), called)
	}
}

// A manifest that declares an invalid broker must not silently send db-read
// down the local (credential-holding) path.
func TestDBReadCmd_InvalidBrokerManifestErrors(t *testing.T) {
	hermeticDBRead(t)
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(http.ResponseWriter, *http.Request) {}})
	if err := os.WriteFile(mp, []byte("org: o\nrepos:\n  - name: r\n    url: u\nbroker:\n  url: http://x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"find", "users"}, {"--prd", "find", "users"}} {
		_, errOut, err := execCmd(newDBReadCmd(), append([]string{"--manifest", mp}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("err = %v, want the manifest https error", err)
		}
		// the local path prints a MONGO_URL / workspace complaint on stderr
		if strings.Contains(errOut, "MONGO_URL") || strings.Contains(errOut, "workspace") {
			t.Errorf("local path was taken: stderr = %q", errOut)
		}
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Errorf("broker got %d requests, want 0", n)
	}
}

// A mistyped --manifest is an error, never a silent fallback to local
// (possibly production) credentials.
func TestDBReadCmd_MissingManifestErrors(t *testing.T) {
	hermeticDBRead(t)
	for _, args := range [][]string{{"find", "users"}, {"--prd", "find", "users"}} {
		_, errOut, err := execCmd(newDBReadCmd(), append([]string{"--manifest", filepath.Join(t.TempDir(), "nope.yaml")}, args...)...)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%v: err = %v, want a not-exist error", args, err)
		}
		if strings.Contains(errOut, "MONGO_URL") || strings.Contains(errOut, "workspace") {
			t.Errorf("local path was taken: stderr = %q", errOut)
		}
	}
}

// Without --manifest the broker comes from the trusted source, never from a
// manifest/repos.yaml in cwd.
func TestBrokerFromFlags_IgnoresCwdManifest(t *testing.T) {
	hermeticDBRead(t)
	mp, _ := brokerFixture(t, nil)
	if err := os.MkdirAll("manifest", 0o750); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(mp) //nolint:gosec // test-owned temp file
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("manifest", "repos.yaml"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if c, ok, err := brokerFromFlags("", ""); ok || c != nil || err != nil {
		t.Fatalf("brokerFromFlags = %v, %v, %v; want the trusted no-broker manifest", c, ok, err)
	}
	useTrustedManifest(t, mp)
	if _, ok, err := brokerFromFlags("", ""); !ok || err != nil {
		t.Fatalf("trusted broker manifest: ok=%v err=%v", ok, err)
	}
}

func TestRedisRead_NoBroker(t *testing.T) {
	hermeticDBRead(t)
	_, _, err := execCmd(newRedisReadCmd(), "--manifest", noBrokerManifest(t), "GET", "k")
	if err == nil || !strings.Contains(err.Error(), "redis-read cần broker") { //znf:allow-lang
		t.Fatalf("err = %v", err)
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code = %d, want BadArgs", exitcode.Code(err))
	}
}

func TestRedisRead_DelRejectedLocally(t *testing.T) {
	hermeticDBRead(t)
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/redis": func(http.ResponseWriter, *http.Request) {}})
	_, _, err := execCmd(newRedisReadCmd(), "--manifest", mp, "DEL", "k")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v (code %d), want BadArgs", err, exitcode.Code(err))
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Errorf("broker got %d requests, want 0", n)
	}
}

func TestRedisRead_SendsAndIndents(t *testing.T) {
	hermeticDBRead(t)
	var got api.RedisRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/redis": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(api.RedisResponse{Result: []string{"a", "b"}})
	}})
	out, _, err := execCmd(newRedisReadCmd(), "--manifest", mp, "--prd", "lrange", "q", "0", "-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env != "prd" || got.Cmd != "LRANGE" || strings.Join(got.Args, " ") != "q 0 -1" {
		t.Errorf("request = %+v", got)
	}
	if want := "[\n  \"a\",\n  \"b\"\n]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestBrokerHeaders_PrintsJSON(t *testing.T) {
	hermeticDBRead(t)
	mp, _ := brokerFixture(t, nil)
	out, errOut, err := execCmd(newBrokerCmd(), "headers", "--manifest", mp)
	if err != nil {
		t.Fatalf("err=%v stderr=%s", err, errOut)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout not one JSON object: %q", out)
	}
	if !reflect.DeepEqual(got, map[string]string{"Authorization": "Bearer test-jwt"}) {
		t.Fatalf("got=%v", got)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("extra lines: %q", out)
	}
}

func TestBrokerHeaders_NoBrokerFailsWithoutStdout(t *testing.T) {
	hermeticDBRead(t)
	for _, sub := range []string{"headers", "token"} {
		out, _, err := execCmd(newBrokerCmd(), sub, "--manifest", noBrokerManifest(t))
		if exitcode.Code(err) != exitcode.BadArgs || out != "" {
			t.Fatalf("%s: err=%v (code %d) out=%q", sub, err, exitcode.Code(err), out)
		}
	}
}

// `broker headers` gives up at brokerHeadersTimeout (8s in production) when
// the broker hangs, printing nothing.
func TestBrokerHeaders_Bounded(t *testing.T) {
	hermeticDBRead(t)
	saved := brokerHeadersTimeout
	brokerHeadersTimeout = 200 * time.Millisecond
	t.Cleanup(func() { brokerHeadersTimeout = saved })
	release := make(chan struct{})
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/token": func(http.ResponseWriter, *http.Request) { <-release }})
	t.Cleanup(func() { close(release) })
	start := time.Now()
	out, _, err := execCmd(newBrokerCmd(), "headers", "--manifest", mp)
	if err == nil || out != "" {
		t.Fatalf("err=%v out=%q", err, out)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v, want the injected bound", d)
	}
}

// `broker token --refresh` ignores a cached JWT and caches the new one.
func TestBrokerToken_Refresh(t *testing.T) {
	hermeticDBRead(t)
	var n int32
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/token": func(w http.ResponseWriter, _ *http.Request) {
		i := atomic.AddInt32(&n, 1)
		_ = json.NewEncoder(w).Encode(api.TokenResponse{Token: fmt.Sprintf("jwt-%d", i), ExpiresAt: time.Now().Add(15 * time.Minute).Format(time.RFC3339), Login: "octo", Role: "dev"})
	}})
	for i, args := range [][]string{{"token"}, {"token"}, {"token", "--refresh"}, {"token"}} {
		out, errOut, err := execCmd(newBrokerCmd(), append(args, "--manifest", mp)...)
		want := []string{"jwt-1", "jwt-1", "jwt-2", "jwt-2"}[i]
		if err != nil || strings.TrimSpace(out) != want {
			t.Fatalf("%v: out=%q err=%v, want %s", args, out, err, want)
		}
		if errOut != "login=octo, role=dev\n" { // stderr, so $(zenify broker token) stays the JWT
			t.Fatalf("%v: stderr=%q, want login and role", args, errOut)
		}
	}
	if *hits != 2 {
		t.Fatalf("/token hits = %d, want 2", *hits)
	}
}

func TestRegisterBrokerMCP(t *testing.T) {
	m := &manifest.Manifest{Broker: &manifest.Broker{URL: "https://broker.example/"}}
	savedLook := lookPath
	t.Cleanup(func() { lookPath = savedLook })
	lookPath = func(string) (string, error) { return "/fake/claude", nil }
	var adds []string
	run := func(_ string, args ...string) ([]byte, error) {
		if args[1] == "get" {
			return []byte("No MCP server found with name: " + args[2]), errors.New("exit status 1")
		}
		adds = append(adds, args[2]+" "+args[3])
		return nil, nil
	}
	var out, errOut bytes.Buffer
	registerBrokerMCP(&out, &errOut, m, run)
	if len(adds) != 2 || !strings.HasPrefix(adds[0], "zenify-grafana ") || !strings.Contains(adds[0], "https://broker.example/mcp/grafana") || !strings.HasPrefix(adds[1], "zenify-posthog ") || !strings.Contains(adds[1], "https://broker.example/mcp/posthog") {
		t.Fatalf("adds=%v", adds)
	}
	adds = nil
	registerBrokerMCP(&out, &errOut, &manifest.Manifest{}, run)
	registerBrokerMCP(&out, &errOut, nil, run)
	if len(adds) != 0 || errOut.Len() != 0 {
		t.Fatalf("no-broker must be a no-op: adds=%v err=%q", adds, errOut.String())
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	registerBrokerMCP(&out, &errOut, m, run)
	if len(adds) != 0 || errOut.Len() != 0 {
		t.Fatalf("without claude it must be a no-op: adds=%v err=%q", adds, errOut.String())
	}
}

// runClaudeBounded kills a hung child at claudeMCPTimeout.
func TestRunClaudeBounded_TimesOut(t *testing.T) {
	saved := claudeMCPTimeout
	claudeMCPTimeout = 200 * time.Millisecond
	t.Cleanup(func() { claudeMCPTimeout = saved })
	bin := filepath.Join(t.TempDir(), "hang")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 30\n"), 0o700); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := runClaudeBounded(bin); err == nil {
		t.Fatal("want an error from the killed child")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v, want the bound to stop it", d)
	}
}

// `broker token --refresh` removes the cached JWT even when /token then fails,
// so `broker headers` stops serving a token the broker refuses.
func TestBrokerToken_RefreshDropsCacheOnFailure(t *testing.T) {
	hermeticDBRead(t)
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/token": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeNotOrgMember, Message: "left the org"})
	}})
	var cache string
	saved := tuneBrokerClient
	tuneBrokerClient = func(c *brokerclient.Client) {
		saved(c)
		cache = filepath.Join(c.Home, ".zenify", "broker", "token.json")
		if err := os.MkdirAll(filepath.Dir(cache), 0o700); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf(`{"url":%q,"token":"rejected-jwt","expires_at":"2099-01-01T00:00:00Z","login":"octo","role":"dev"}`, c.URL)
		if err := os.WriteFile(cache, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { tuneBrokerClient = saved })
	out, _, err := execCmd(newBrokerCmd(), "token", "--refresh", "--manifest", mp)
	if err == nil || out != "" {
		t.Fatalf("out=%q err=%v, want the 403 and no token", out, err)
	}
	if _, serr := os.Stat(cache); !errors.Is(serr, fs.ErrNotExist) {
		t.Fatalf("cache still present after --refresh: %v", serr)
	}
}

// With a cached dev role, a prd read outside the dev allowlist fails before
// any broker call; allowed ops, stg, and a missing cache still reach it.
func TestDevPrdPrecheck(t *testing.T) {
	hermeticDBRead(t)
	ok := func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(map[string]any{}) }
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/query": ok, "/redis": ok})

	// No cache yet: the broker decides, so the request is sent.
	if _, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "--prd", "find", "users"); err != nil || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("no cache: err=%v hits=%d, want sent", err, *hits)
	}
	// That call cached role=dev.
	for _, args := range [][]string{
		{"--prd", "find", "users"},
		{"--prd", "doc", "users"},
		{"--prd", "sql", "SELECT * FROM accounts"},
		{"--prd", "sql", "SHOW PROCESSLIST"},
	} {
		before := atomic.LoadInt32(hits)
		_, _, err := execCmd(newDBReadCmd(), append([]string{"--manifest", mp}, args...)...)
		if err == nil || !strings.Contains(err.Error(), leadOnlyMsg) || atomic.LoadInt32(hits) != before {
			t.Errorf("db-read %v: err=%v hits %d→%d, want lead_only and no call", args, err, before, *hits)
		}
	}
	for _, args := range [][]string{
		{"--prd", "indexes", "users"},
		{"--prd", "stats", "users"},
		{"--prd", "sql", "EXPLAIN SELECT * FROM accounts WHERE id=1"},
		{"find", "users"}, // stg
	} {
		before := atomic.LoadInt32(hits)
		if _, _, err := execCmd(newDBReadCmd(), append([]string{"--manifest", mp}, args...)...); err != nil || atomic.LoadInt32(hits) != before+1 {
			t.Errorf("db-read %v: err=%v, want sent", args, err)
		}
	}
	before := atomic.LoadInt32(hits)
	if _, _, err := execCmd(newRedisReadCmd(), "--manifest", mp, "--prd", "GET", "k"); err == nil || !strings.Contains(err.Error(), leadOnlyMsg) || atomic.LoadInt32(hits) != before {
		t.Errorf("redis-read --prd GET: err=%v, want lead_only and no call", err)
	}
	for _, args := range [][]string{{"--prd", "TTL", "k"}, {"GET", "k"}} {
		before := atomic.LoadInt32(hits)
		if _, _, err := execCmd(newRedisReadCmd(), append([]string{"--manifest", mp}, args...)...); err != nil || atomic.LoadInt32(hits) != before+1 {
			t.Errorf("redis-read %v: err=%v, want sent", args, err)
		}
	}
}
