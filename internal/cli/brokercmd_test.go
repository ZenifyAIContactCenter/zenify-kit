package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// brokerFixture serves /token plus the handlers given, points the CLI's
// broker client at it, and returns a manifest path that declares a broker.
func brokerFixture(t *testing.T, handlers map[string]http.HandlerFunc) (manifestPath string, hits *int32) {
	t.Helper()
	hits = new(int32)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.TokenResponse{Token: "test-jwt", ExpiresAt: time.Now().Add(15 * time.Minute).Format(time.RFC3339), Login: "octo"})
	})
	for path, h := range handlers {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(hits, 1)
			h(w, r)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	home := t.TempDir()
	tuneBrokerClient = func(c *brokerclient.Client) {
		c.URL, c.Home, c.GHToken = srv.URL, home, func() (string, error) { return "test-gh", nil }
	}
	t.Cleanup(func() { tuneBrokerClient = nil })
	manifestPath = filepath.Join(t.TempDir(), "repos.yaml")
	y := "org: o\nrepos:\n  - name: r\n    url: u\nbroker:\n  url: https://broker.example\n"
	if err := os.WriteFile(manifestPath, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, hits
}

func noBrokerManifest(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "repos.yaml")
	if err := os.WriteFile(p, []byte("org: o\nrepos:\n  - name: r\n    url: u\n"), 0o600); err != nil {
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
	if out != "{\"a\":1}\n" || errOut != "note\n" {
		t.Errorf("stdout=%q stderr=%q", out, errOut)
	}
}

func TestDBReadCmd_BrokerSQLMapsToMySQL(t *testing.T) {
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

func TestDBReadCmd_BrokerValidatesLocally(t *testing.T) {
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
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/query": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeInvalidQuery, Message: "nope"})
	}})
	_, _, err := execCmd(newDBReadCmd(), "--manifest", mp, "count", "users")
	if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

// No broker: db-read takes the v0.38.0 local path. A real run would open the
// workspace's databases, so the test checks the routing decision and that a
// local validation error still surfaces unchanged.
func TestDBReadCmd_NoBrokerLocal(t *testing.T) {
	called := false
	tuneBrokerClient = func(*brokerclient.Client) { called = true }
	t.Cleanup(func() { tuneBrokerClient = nil })
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

func TestDBReadCmd_MissingManifestRunsLocal(t *testing.T) {
	_, _, err := execCmd(newDBReadCmd(), "--manifest", filepath.Join(t.TempDir(), "nope.yaml"), "find", "users", "--limit", "500")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v, want the local BadArgs validation error", err)
	}
}

func TestRedisRead_NoBroker(t *testing.T) {
	_, _, err := execCmd(newRedisReadCmd(), "--manifest", noBrokerManifest(t), "GET", "k")
	if err == nil || !strings.Contains(err.Error(), "redis-read cần broker") { //znf:allow-lang
		t.Fatalf("err = %v", err)
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code = %d, want BadArgs", exitcode.Code(err))
	}
}

func TestRedisRead_DelRejectedLocally(t *testing.T) {
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
