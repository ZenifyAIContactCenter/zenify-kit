package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

const dataMongoURL = "mongodb://user1:secret9@host7.example/db"

// dataFixture is a temp workspace (cwd), a stub broker whose POST /creds
// answers with values (or a status/code when refuse is set), and the requests
// the broker saw.
type dataFixture struct {
	ws   string
	reqs *[]api.CredsRequest
	hits *int32
}

func newDataFixture(t *testing.T, values map[string]string, refuseCode string) dataFixture {
	t.Helper()
	hermeticDBRead(t)
	t.Setenv("PATH", "/bin:/usr/bin") // the children are sh, env and true
	ws, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(ws)
	var mu sync.Mutex
	reqs := &[]api.CredsRequest{}
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/creds": func(w http.ResponseWriter, r *http.Request) {
		var req api.CredsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		*reqs = append(*reqs, req)
		mu.Unlock()
		if refuseCode != "" {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(api.Error{Error: refuseCode, Message: "no"})
			return
		}
		_ = json.NewEncoder(w).Encode(api.CredsResponse{Values: values})
	}})
	useTrustedManifest(t, mp)
	return dataFixture{ws: ws, reqs: reqs, hits: hits}
}

func runData(args ...string) (string, string, error) {
	return execCmd(newDataCmd(), append([]string{"run"}, args...)...)
}

func TestDataRun_NeedsEnv(t *testing.T) {
	newDataFixture(t, nil, "")
	for _, args := range [][]string{{"--", "true"}, {"--stg", "--prd", "--", "true"}} {
		_, _, err := runData(args...)
		if got := exitcode.Code(err); got != exitcode.BadArgs {
			t.Errorf("args %v: exit %d (err %v), want %d", args, got, err, exitcode.BadArgs)
		}
	}
}

func TestDataRun_NoBroker(t *testing.T) {
	hermeticDBRead(t) // trusted manifest from TestMain declares no broker
	_, _, err := runData("--stg", "--", "true")
	if exitcode.Code(err) != exitcode.BadArgs || err == nil || !strings.HasPrefix(err.Error(), "data run: ") {
		t.Fatalf("err = %v (exit %d), want BadArgs with prefix", err, exitcode.Code(err))
	}
}

func TestDataRun_MasksValues(t *testing.T) {
	newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	out, errOut, err := runData("--stg", "--", "sh", "-c", `echo $MONGO_URL; printf %s "$MONGO_URL"; echo "$MONGO_URL" >&2`)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"secret9", "host7.example"} {
		if strings.Contains(out, s) || strings.Contains(errOut, s) {
			t.Errorf("%q leaked: stdout %q stderr %q", s, out, errOut)
		}
	}
	// The final printf has no newline: only the flush on Close emits it.
	if want := "***\n***"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestDataRun_ScrubsInheritedEnv(t *testing.T) {
	newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	t.Setenv("MYSQL_URL", "x1")
	t.Setenv("MONGO_URL_PRD", "y1")
	t.Setenv("KEEP_ME", "kept1")
	out, _, err := runData("--stg", "--", "sh", "-c", "env")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"MYSQL_URL=", "MONGO_URL_PRD="} {
		if strings.Contains(out, bad) {
			t.Errorf("child env still has %s", bad)
		}
	}
	if !strings.Contains(out, "KEEP_ME=kept1") {
		t.Error("unrelated env var was dropped")
	}
	if !strings.Contains(out, "MONGO_URL=") {
		t.Error("broker value missing in child env")
	}
}

func TestDataRun_ExitPassthrough(t *testing.T) {
	newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	_, _, err := runData("--stg", "--", "sh", "-c", "exit 3")
	if got := exitcode.Code(err); got != 3 {
		t.Fatalf("exit %d (err %v), want 3", got, err)
	}
}

func TestDataRun_WriteWithoutDryRun(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	_, _, err := runData("--stg", "--write", "--", "sh", "-c", "true")
	if exitcode.Code(err) != exitcode.NeedsDryRun || err == nil || !strings.HasPrefix(err.Error(), "data run: ") {
		t.Fatalf("err = %v (exit %d)", err, exitcode.Code(err))
	}
	if want := "run: zenify data run --stg -- sh -c true --dry-run"; !strings.HasSuffix(err.Error(), want) {
		t.Errorf("error = %q, want suffix %q", err, want)
	}
	_, _, err = runData("--stg", "--write", "--lumi", "--", "sh", "-c", "echo $X", "it's")
	if want := `run: zenify data run --stg --lumi -- sh -c 'echo $X' 'it'\''s' --dry-run`; err == nil || !strings.HasSuffix(err.Error(), want) {
		t.Errorf("lumi error = %v, want suffix %q", err, want)
	}
	if len(*f.reqs) != 0 {
		t.Errorf("broker got %d /creds requests, want 0", len(*f.reqs))
	}
}

func TestDataRun_DryRunThenWrite(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	args := []string{"--", "sh", "-c", "true"}
	if _, _, err := runData(append([]string{"--stg"}, args...)...); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runData(append([]string{"--stg", "--write"}, args...)...); err != nil {
		t.Fatalf("write after dry-run: %v", err)
	}
	if len(*f.reqs) != 2 || (*f.reqs)[0].Mode != api.CredsModeRead || (*f.reqs)[1].Mode != api.CredsModeWrite || (*f.reqs)[1].Env != "stg" {
		t.Fatalf("requests = %+v", *f.reqs)
	}
	_, _, err := runData(append([]string{"--stg", "--write"}, args...)...)
	if exitcode.Code(err) != exitcode.NeedsDryRun {
		t.Fatalf("second write: exit %d (err %v), want 7", exitcode.Code(err), err)
	}
}

func TestDataRun_LumiFlagSent(t *testing.T) {
	f := newDataFixture(t, map[string]string{"LUMI_MYSQL_URL": "mysql://u:pw123456@h.example/db"}, "")
	if _, _, err := runData("--stg", "--lumi", "--", "true"); err != nil {
		t.Fatal(err)
	}
	if len(*f.reqs) != 1 || !(*f.reqs)[0].Lumi {
		t.Fatalf("requests = %+v", *f.reqs)
	}
}

func TestDataRun_FailedDryRun(t *testing.T) {
	newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	if _, _, err := runData("--stg", "--", "sh", "-c", "exit 1"); exitcode.Code(err) != 1 {
		t.Fatalf("dry-run exit %d, want 1", exitcode.Code(err))
	}
	_, _, err := runData("--stg", "--write", "--", "sh", "-c", "exit 1")
	if exitcode.Code(err) != exitcode.NeedsDryRun {
		t.Fatalf("write after failed dry-run: exit %d (err %v), want 7", exitcode.Code(err), err)
	}
}

func TestDataRun_WriteLog(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	inside := filepath.Join(f.ws, "docs", "scripts", "r1", "x.sh")
	outside := filepath.Join(f.ws, "elsewhere.sh")
	for _, p := range []string{inside, outside} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("echo $MONGO_URL\necho oops >&2\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{inside, outside} {
		for _, extra := range [][]string{{}, {"--write"}} {
			args := append(append([]string{"--stg"}, extra...), "--", "sh", p)
			if _, _, err := runData(args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	in, _ := filepath.Glob(filepath.Join(f.ws, "docs", "scripts", "r1", "x.sh.*.stg.write.log"))
	if len(in) != 1 {
		t.Fatalf("logs under docs/scripts/r1: %v", in)
	}
	out, _ := filepath.Glob(filepath.Join(f.ws, ".zenify", "data-runs", "elsewhere.sh.*.stg.write.log"))
	if len(out) != 1 {
		t.Fatalf("logs under .zenify/data-runs: %v", out)
	}
	for _, p := range append(in, out...) {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, "secret9") || strings.Contains(s, "host7.example") {
			t.Errorf("%s leaks a broker value: %q", p, s)
		}
		if !strings.Contains(s, "***") || !strings.Contains(s, "oops") {
			t.Errorf("%s should hold masked stdout and stderr, got %q", p, s)
		}
	}
}

func TestDataRun_LeadOnly(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	if _, _, err := runData("--stg", "--", "true"); err != nil {
		t.Fatal(err)
	}
	// A refused --write must leave the dry-run record unclaimed (MarkUsed only after the broker accepts).
	g := newDataFixtureAt(t, f.ws, nil, api.CodeLeadOnly)
	_, _, err := runData("--stg", "--write", "--", "true")
	if exitcode.Code(err) != exitcode.Fail || err == nil || !strings.HasPrefix(err.Error(), "data run: ") || !strings.Contains(err.Error(), "prd needs role lead") {
		t.Fatalf("err = %v (exit %d)", err, exitcode.Code(err))
	}
	if len(*g.reqs) != 1 {
		t.Fatalf("refusing broker saw %d requests", len(*g.reqs))
	}
	newDataFixtureAt(t, f.ws, map[string]string{"MONGO_URL": dataMongoURL}, "")
	if _, _, err := runData("--stg", "--write", "--", "true"); err != nil {
		t.Fatalf("dry-run record lost after refusal: %v", err)
	}
}

// newDataFixtureAt swaps the stub broker while keeping workspace ws as cwd.
func newDataFixtureAt(t *testing.T, ws string, values map[string]string, refuseCode string) dataFixture {
	t.Helper()
	var mu sync.Mutex
	reqs := &[]api.CredsRequest{}
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/creds": func(w http.ResponseWriter, r *http.Request) {
		var req api.CredsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		*reqs = append(*reqs, req)
		mu.Unlock()
		if refuseCode != "" {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(api.Error{Error: refuseCode, Message: "no"})
			return
		}
		_ = json.NewEncoder(w).Encode(api.CredsResponse{Values: values})
	}})
	useTrustedManifest(t, mp)
	return dataFixture{ws: ws, reqs: reqs, hits: hits}
}

func TestDataRun_EditedScriptNeedsNewDryRun(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	script := filepath.Join(f.ws, "job.sh")
	if err := os.WriteFile(script, []byte("true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runData("--stg", "--", "sh", script); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("true # edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runData("--stg", "--write", "--", "sh", script)
	if exitcode.Code(err) != exitcode.NeedsDryRun {
		t.Fatalf("exit %d (err %v), want 7", exitcode.Code(err), err)
	}
	if len(*f.reqs) != 1 || (*f.reqs)[0].Mode != api.CredsModeRead {
		t.Fatalf("requests = %+v, want only the dry-run read", *f.reqs)
	}
}

func TestDataRun_DryRunEnvMismatch(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	if _, _, err := runData("--stg", "--", "sh", "-c", "true"); err != nil {
		t.Fatal(err)
	}
	_, _, err := runData("--prd", "--write", "--", "sh", "-c", "true")
	if exitcode.Code(err) != exitcode.NeedsDryRun {
		t.Fatalf("exit %d (err %v), want 7", exitcode.Code(err), err)
	}
	if len(*f.reqs) != 1 {
		t.Fatalf("broker saw %d requests, want 1 (the stg dry-run)", len(*f.reqs))
	}
}

func TestDataRun_PrdLeadOnlyNamesRole(t *testing.T) {
	f := newDataFixture(t, nil, api.CodeLeadOnly)
	_, _, err := runData("--prd", "--", "true")
	if exitcode.Code(err) != exitcode.Fail || err == nil || !strings.Contains(err.Error(), "prd needs role lead (yours: dev)") {
		t.Fatalf("err = %v (exit %d)", err, exitcode.Code(err))
	}
	if len(*f.reqs) != 1 || (*f.reqs)[0].Env != "prd" {
		t.Fatalf("requests = %+v", *f.reqs)
	}
}

// A dry-run binds --lumi: one without it does not unlock `--write --lumi`, and vice versa.
func TestDataRun_DryRunLumiMismatch(t *testing.T) {
	for _, tc := range []struct{ name, dry, write string }{
		{"dry plain, write lumi", "", "--lumi"},
		{"dry lumi, write plain", "--lumi", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
			flags := func(extra ...string) []string {
				var out []string
				for _, a := range append([]string{"--stg"}, extra...) {
					if a != "" {
						out = append(out, a)
					}
				}
				return append(out, "--", "sh", "-c", "true")
			}
			if _, _, err := runData(flags(tc.dry)...); err != nil {
				t.Fatal(err)
			}
			_, _, err := runData(flags("--write", tc.write)...)
			if exitcode.Code(err) != exitcode.NeedsDryRun {
				t.Fatalf("exit %d (err %v), want 7", exitcode.Code(err), err)
			}
			if len(*f.reqs) != 1 {
				t.Fatalf("broker saw %d requests, want 1 (the dry-run)", len(*f.reqs))
			}
		})
	}
}

// A broker built before /creds answers a plain-text 404: data run names the upgrade.
func TestDataRun_BrokerWithoutCreds(t *testing.T) {
	f := newDataFixture(t, nil, "")
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{})
	useTrustedManifest(t, mp)
	_, _, err := runData("--stg", "--", "true")
	if exitcode.Code(err) != exitcode.Fail || err == nil || !strings.Contains(err.Error(), "broker chưa hỗ trợ /creds — cần nâng cấp broker") { //znf:allow-lang
		t.Fatalf("err = %v (exit %d)", err, exitcode.Code(err))
	}
	if recs, _ := filepath.Glob(filepath.Join(f.ws, ".zenify", "data-runs", "*.json")); len(recs) != 0 {
		t.Errorf("a refused run wrote dry-run records: %v", recs)
	}
}

// Only the six data names reach the child; any other key is skipped and named on stderr.
func TestDataRun_SkipsUnknownBrokerKeys(t *testing.T) {
	newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL, "NODE_OPTIONS": "--require=/x/evil.js"}, "")
	out, errOut, err := runData("--stg", "--", "env")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "NODE_OPTIONS=") {
		t.Errorf("unknown broker key reached the child env")
	}
	if !strings.Contains(out, "MONGO_URL=") {
		t.Errorf("known broker key missing from the child env")
	}
	if !strings.Contains(errOut, "NODE_OPTIONS") {
		t.Errorf("stderr does not name the skipped key: %q", errOut)
	}
	if strings.Contains(out+errOut, "evil") {
		t.Errorf("the skipped value was printed: stdout %q stderr %q", out, errOut)
	}
}

// The write log goes by the script, not the interpreter given as an absolute path.
func TestOpenWriteLog_PicksScriptNotInterpreter(t *testing.T) {
	ws := t.TempDir()
	interp := filepath.Join(t.TempDir(), "node")
	inside := filepath.Join(ws, "docs", "scripts", "r1", "fix.js")
	outside := filepath.Join(ws, "job.js")
	for _, p := range []string{interp, inside, outside} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	for i, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{interp, inside, "--tenant", "T"}, filepath.Join(ws, "docs", "scripts", "r1", "fix.js.")},
		{[]string{interp, outside}, filepath.Join(ws, ".zenify", "data-runs", "job.js.")},
		{[]string{outside, inside}, filepath.Join(ws, "docs", "scripts", "r1", "fix.js.")},
		{[]string{interp, "--flag"}, filepath.Join(ws, ".zenify", "data-runs", "node.")},
	} {
		f, err := openWriteLog(ws, ws, "stg", tc.argv, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if !strings.HasPrefix(f.Name(), tc.want) {
			t.Errorf("argv %v: log %s, want prefix %s", tc.argv, f.Name(), tc.want)
		}
	}
}

// seedAgentCache makes the broker client of the fixture start with a valid
// cached JWT of role agent, so no /token call is needed.
func seedAgentCache(t *testing.T) {
	t.Helper()
	saved := tuneBrokerClient
	tuneBrokerClient = func(c *brokerclient.Client) {
		saved(c)
		body := `{"url":"` + c.URL + `","token":"jwt-agent","expires_at":"2099-01-01T00:00:00Z","login":"bot","role":"agent"}`
		writeBrokerCache(t, c.Home, body)
	}
	t.Cleanup(func() { tuneBrokerClient = saved })
}

// A role-agent session never reaches /creds for a prd write: it stops and
// points at `zenify await data-write`; the dry-run check still comes first.
func TestDataRun_AgentPrdWriteStopsBeforeCreds(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	seedAgentCache(t)
	args := []string{"--", "sh", "-c", "true"}
	_, _, err := runData(append([]string{"--prd", "--write"}, args...)...)
	if exitcode.Code(err) != exitcode.NeedsDryRun {
		t.Fatalf("no dry-run: exit %d (err %v), want NeedsDryRun", exitcode.Code(err), err)
	}
	if _, _, err := runData(append([]string{"--prd"}, args...)...); err != nil {
		t.Fatalf("agent prd dry-run: %v", err)
	}
	before := len(*f.reqs)
	_, _, err = runData(append([]string{"--prd", "--write"}, args...)...)
	if exitcode.Code(err) != exitcode.Fail || err == nil || !strings.Contains(err.Error(), "zenify await data-write") {
		t.Fatalf("err = %v (exit %d), want Fail naming zenify await data-write", err, exitcode.Code(err))
	}
	if len(*f.reqs) != before {
		t.Errorf("broker got %d /creds requests after the stop, want 0", len(*f.reqs)-before)
	}
}

// An installation token in GH_TOKEN (ghs_…) stops a prd write even when the
// token cache holds another role: the cache may be a lead's JWT.
func TestDataRun_AgentGHTokenPrdWriteStops(t *testing.T) {
	f := newDataFixture(t, map[string]string{"MONGO_URL": dataMongoURL}, "")
	t.Setenv("GH_TOKEN", "ghs_x")
	args := []string{"--", "sh", "-c", "true"}
	if _, _, err := runData(append([]string{"--prd"}, args...)...); err != nil {
		t.Fatalf("prd dry-run: %v", err)
	}
	before := len(*f.reqs)
	_, _, err := runData(append([]string{"--prd", "--write"}, args...)...)
	if exitcode.Code(err) != exitcode.Fail || err == nil || !strings.Contains(err.Error(), "zenify await data-write") {
		t.Fatalf("err = %v (exit %d), want Fail naming zenify await data-write", err, exitcode.Code(err))
	}
	if len(*f.reqs) != before {
		t.Errorf("broker got %d /creds requests after the stop, want 0", len(*f.reqs)-before)
	}
}
