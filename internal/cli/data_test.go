package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
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
	if !strings.Contains(out, "***") {
		t.Errorf("stdout %q shows no mask", out)
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
	if !strings.Contains(err.Error(), "zenify data run --stg --") {
		t.Errorf("error does not name the dry-run command: %v", err)
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
	// A dry-run recorded for stg must stay usable after a refused prd call.
	if _, _, err := runData("--stg", "--", "true"); err != nil {
		t.Fatal(err)
	}
	// Re-point the broker at a refusing stub.
	g := newDataFixtureAt(t, f.ws, nil, api.CodeLeadOnly)
	_, _, err := runData("--prd", "--", "true")
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
