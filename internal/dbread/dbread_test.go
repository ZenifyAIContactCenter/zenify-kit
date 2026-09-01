package dbread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefuseWrites(t *testing.T) {
	cases := []struct {
		stmt string
		want bool
	}{
		{"INSERT", true},
		{"Db.X.updateOne", true},
		{"SET x", true},
		{"SET a=1", true},
		{"db.coll.find({})", false},
		{"SELECT * FROM t", false},
		{"db.coll.aggregate([...])", false},
		{"estimatedDocumentCount()", false},
		{"reset()", false},
		{"db.coll.deleteOne({})", true},
		{"db.coll.remove({})", true},
		{"db.coll.replaceOne({},{})", true},
		{"db.coll.drop()", true},
		{"db.coll.renameCollection('x')", true},
		{"db.coll.save({})", true},
		{"db.coll.bulkWrite([])", true},
		{"db.coll.findAndModify({})", true},
		{"db.runCommand({collMod: 'x'})", true},
		{"TRUNCATE TABLE t", true},
		{"ALTER TABLE t", true},
		{"GRANT ALL ON x TO y", true},
		{"REVOKE ALL ON x FROM y", true},
		{"CREATE TABLE t (id int)", true},
	}
	for _, c := range cases {
		if got := RefuseWrites(c.stmt); got != c.want {
			t.Errorf("RefuseWrites(%q) = %v, want %v", c.stmt, got, c.want)
		}
	}
}

func TestLoadCreds_EnvWinsOverSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	writeSettings(t, path, map[string]string{"MONGO_URL": "mongodb://from-settings", "MYSQL_HOST": "settings-host"})

	env := map[string]string{"MONGO_URL": "mongodb://from-env"}
	creds, err := LoadCreds(func(k string) string { return env[k] }, path)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if creds["MONGO_URL"] != "mongodb://from-env" {
		t.Errorf("MONGO_URL = %q, want live env to win", creds["MONGO_URL"])
	}
}

func TestLoadCreds_SettingsFallbackWhenEnvEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	writeSettings(t, path, map[string]string{"MONGO_URL": "mongodb://from-settings", "MYSQL_HOST": "settings-host"})

	creds, err := LoadCreds(func(string) string { return "" }, path)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if creds["MONGO_URL"] != "mongodb://from-settings" {
		t.Errorf("MONGO_URL = %q, want settings fallback", creds["MONGO_URL"])
	}
	if creds["MYSQL_HOST"] != "settings-host" {
		t.Errorf("MYSQL_HOST = %q, want settings fallback", creds["MYSQL_HOST"])
	}
}

func TestLoadCreds_MissingFileNoError(t *testing.T) {
	creds, err := LoadCreds(func(string) string { return "" }, filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("creds = %v, want empty", creds)
	}
}

func TestLoadCreds_MalformedJSONNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := LoadCreds(func(string) string { return "" }, path)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("creds = %v, want empty", creds)
	}
}

func writeSettings(t *testing.T, path string, env map[string]string) {
	t.Helper()
	doc := map[string]any{"env": env}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// baseOptions returns Options wired with a fake settings path (empty dir, so
// LoadCreds falls through to env), fresh buffers, and env carrying a fake
// MONGO_URL/MYSQL_* set so Run doesn't hit the "not set" fatal path.
func baseOptions(t *testing.T, env map[string]string) (*Options, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	o := &Options{
		Stdout:       &stdout,
		Stderr:       &stderr,
		Env:          func(k string) string { return env[k] },
		SettingsPath: filepath.Join(t.TempDir(), "nope.json"),
	}
	return o, &stdout, &stderr
}

const fakeMongoURL = "mongodb://u:p@1.2.3.4:27017/?authSource=3csoft" //nolint:gosec // G101 -- test fixture literal, not a real credential
const fakeMysqlPassword = "s3cr3t-pw"

func fakeEnv() map[string]string {
	return map[string]string{
		"MONGO_URL":      fakeMongoURL,
		"MYSQL_HOST":     "mysql.internal",
		"MYSQL_PORT":     "3306",
		"MYSQL_USER":     "clusteradmin",
		"MYSQL_PASSWORD": fakeMysqlPassword,
		"MYSQL_DATABASE": "db_acd",
	}
}

func TestRun_Collections_ExactEvalAndArgs(t *testing.T) {
	o, stdout, _ := baseOptions(t, fakeEnv())
	var capturedName string
	var capturedArgs []string
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		capturedName = name
		capturedArgs = args
		_, _ = fmt.Fprint(o.Stdout, "chat_rooms\nusers\ntickets\n")
		return nil
	})
	o.Cmd = "collections"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedName != "mongosh" {
		t.Errorf("name = %q, want mongosh", capturedName)
	}
	wantArgs := []string{fakeMongoURL, "--quiet", "--eval",
		`db.getSiblingDB("3csoft").getCollectionInfos({type:"collection"}).map(c=>c.name).sort().forEach(n=>print(n))`}
	if len(capturedArgs) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", capturedArgs, wantArgs)
	}
	for i := range wantArgs {
		if capturedArgs[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, capturedArgs[i], wantArgs[i])
		}
	}
	if stdout.String() != "chat_rooms\nusers\ntickets\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRun_Collections_ArgFilters(t *testing.T) {
	o, stdout, _ := baseOptions(t, fakeEnv())
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		_, _ = fmt.Fprint(o.Stdout, "chat_rooms\nusers\ntickets\nchat_messages\n")
		return nil
	})
	o.Cmd = "collections"
	o.Arg = "chat"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != "chat_rooms\nchat_messages" {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRun_Eval_WriteRefused(t *testing.T) {
	o, _, stderr := baseOptions(t, fakeEnv())
	called := false
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		called = true
		return nil
	})
	o.Cmd = "eval"
	o.Arg = "db.tickets.insertOne({})"
	if err := Run(o); err == nil {
		t.Fatal("Run: want error for a write expression")
	}
	if called {
		t.Error("run seam was invoked for a refused write")
	}
	if !strings.Contains(stderr.String(), "refusing") {
		t.Errorf("stderr = %q, want refusal message", stderr.String())
	}
}

func TestRun_Eval_ReadPassesThrough(t *testing.T) {
	o, _, _ := baseOptions(t, fakeEnv())
	var capturedArgs []string
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		capturedArgs = args
		return nil
	})
	o.Cmd = "eval"
	o.Arg = "db.tickets.find({}).limit(1)"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantJS := "db = db.getSiblingDB('3csoft'); db.tickets.find({}).limit(1)"
	if len(capturedArgs) < 4 || capturedArgs[3] != wantJS {
		t.Errorf("args = %v, want eval js %q", capturedArgs, wantJS)
	}
}

func TestRun_SQL_WriteRefused(t *testing.T) {
	o, _, _ := baseOptions(t, fakeEnv())
	called := false
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		called = true
		return nil
	})
	o.Cmd = "sql"
	o.Arg = "DELETE FROM agents"
	if err := Run(o); err == nil {
		t.Fatal("Run: want error for a write query")
	}
	if called {
		t.Error("run seam was invoked for a refused write")
	}
}

func TestRun_SQL_ReadPassesThroughWithPassword(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	var capturedName string
	var capturedArgs, capturedExtraEnv []string
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		capturedName = name
		capturedArgs = args
		capturedExtraEnv = extraEnv
		_, _ = fmt.Fprint(o.Stdout, "agents\n")
		return nil
	})
	o.Cmd = "sql"
	o.Arg = "SELECT * FROM agents"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedName != "mysql" {
		t.Errorf("name = %q, want mysql", capturedName)
	}
	found := false
	for _, e := range capturedExtraEnv {
		if e == "MYSQL_PWD="+fakeMysqlPassword {
			found = true
		}
	}
	if !found {
		t.Errorf("extraEnv = %v, want MYSQL_PWD=%s", capturedExtraEnv, fakeMysqlPassword)
	}
	wantArgs := []string{"-h", "mysql.internal", "-P", "3306", "-u", "clusteradmin", "-N", "-B", "-e", "SELECT * FROM agents", "db_acd"}
	if len(capturedArgs) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", capturedArgs, wantArgs)
	}
	for i := range wantArgs {
		if capturedArgs[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, capturedArgs[i], wantArgs[i])
		}
	}
	if strings.Contains(stdout.String(), fakeMysqlPassword) || strings.Contains(stderr.String(), fakeMysqlPassword) {
		t.Error("FR-041 violation: password leaked into stdout/stderr")
	}
}

func TestRun_NoMongoURL_FatalNoRun(t *testing.T) {
	o, _, stderr := baseOptions(t, map[string]string{})
	called := false
	o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
		called = true
		return nil
	})
	o.Cmd = "collections"
	if err := Run(o); err == nil {
		t.Fatal("Run: want error when MONGO_URL is unset")
	}
	if called {
		t.Error("run seam was invoked despite missing MONGO_URL")
	}
	if !strings.Contains(stderr.String(), "MONGO_URL is not set") {
		t.Errorf("stderr = %q, want MONGO_URL message", stderr.String())
	}
}

func TestRun_ArgRequired(t *testing.T) {
	for _, cmd := range []string{"doc", "count", "eval", "sql"} {
		o, _, _ := baseOptions(t, fakeEnv())
		called := false
		o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
			called = true
			return nil
		})
		o.Cmd = cmd
		o.Arg = ""
		if err := Run(o); err == nil {
			t.Errorf("Run(%s): want error for empty arg", cmd)
		}
		if called {
			t.Errorf("Run(%s): run seam invoked despite missing arg", cmd)
		}
	}
}

func TestRun_FR041_NoSecretLeak(t *testing.T) {
	// Exercises collections, eval-read, and sql-read together and asserts
	// the fake MONGO_URL and MySQL password never land in captured output.
	scenarios := []func(*Options){
		func(o *Options) { o.Cmd = "collections" },
		func(o *Options) { o.Cmd = "eval"; o.Arg = "db.tickets.find({})" },
		func(o *Options) { o.Cmd = "sql"; o.Arg = "SELECT 1" },
	}
	for _, setup := range scenarios {
		o, stdout, stderr := baseOptions(t, fakeEnv())
		o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
			_, _ = fmt.Fprint(o.Stdout, "some-output\n")
			return nil
		})
		setup(o)
		if err := Run(o); err != nil {
			t.Fatalf("Run(%s): %v", o.Cmd, err)
		}
		combined := stdout.String() + stderr.String()
		if strings.Contains(combined, fakeMongoURL) {
			t.Errorf("Run(%s): MONGO_URL leaked into output: %q", o.Cmd, combined)
		}
		if strings.Contains(combined, fakeMysqlPassword) {
			t.Errorf("Run(%s): MySQL password leaked into output: %q", o.Cmd, combined)
		}
	}
}

func TestFilterLines(t *testing.T) {
	out := "chat_rooms\nusers\nchat_messages\nTICKETS\n"
	if got := filterLines(out, "chat"); got != "chat_rooms\nchat_messages" {
		t.Errorf("filterLines chat = %q", got)
	}
	if got := filterLines(out, "TICK"); got != "TICKETS" {
		t.Errorf("filterLines TICK (case-insensitive) = %q", got)
	}
	if got := filterLines(out, ""); got != out {
		t.Errorf("filterLines empty substr = %q, want unchanged", got)
	}
}
