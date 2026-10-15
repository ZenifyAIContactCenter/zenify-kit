package dbread

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func writeEnvFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCreds_FromEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeEnvFile(t, path, "MONGO_URL='mongodb://from-file'\nMYSQL_HOST=file-host\n")

	creds, err := LoadCreds(func(string) string { return "" }, path)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if creds["MONGO_URL"] != "mongodb://from-file" {
		t.Errorf("MONGO_URL = %q, want value from the env file", creds["MONGO_URL"])
	}
	if creds["MYSQL_HOST"] != "file-host" {
		t.Errorf("MYSQL_HOST = %q, want value from the env file", creds["MYSQL_HOST"])
	}
}

func TestLoadCreds_EnvWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeEnvFile(t, path, "MONGO_URL=mongodb://from-file\nMYSQL_HOST=file-host\n")

	env := map[string]string{"MONGO_URL": "mongodb://from-env"}
	creds, err := LoadCreds(func(k string) string { return env[k] }, path)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if creds["MONGO_URL"] != "mongodb://from-env" {
		t.Errorf("MONGO_URL = %q, want live env to win", creds["MONGO_URL"])
	}
	if creds["MYSQL_HOST"] != "file-host" {
		t.Errorf("MYSQL_HOST = %q, want file value for a key env lacks", creds["MYSQL_HOST"])
	}
}

func TestLoadCreds_MissingFileNoError(t *testing.T) {
	creds, err := LoadCreds(func(string) string { return "" }, filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("creds = %v, want empty", creds)
	}
}

// baseOptions returns Options wired with a fake settings path (empty dir, so
// LoadCreds falls through to env), fresh buffers, and env carrying a fake
// MONGO_URL/MYSQL_* set so Run doesn't hit the "not set" fatal path.
func baseOptions(t *testing.T, env map[string]string) (*Options, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	o := &Options{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     func(k string) string { return env[k] },
		EnvPath: filepath.Join(t.TempDir(), "nope"),
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

// call is one invocation recorded by the fake run seam.
type call struct {
	name     string
	args     []string
	extraEnv []string
}

// recordRun installs a seam that records every call and writes out to o.Stdout.
func recordRun(o *Options, out string) *[]call {
	var calls []call
	o.SetRun(func(name string, args []string, extraEnv []string, _ string) error {
		calls = append(calls, call{name, args, extraEnv})
		_, _ = fmt.Fprint(o.Stdout, out)
		return nil
	})
	return &calls
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestRun_Mongo_URLNotOnArgv(t *testing.T) {
	env := fakeEnv()
	env["MONGO_URL"] = "mongodb://u:SECRETPW@h"
	for _, cmd := range []string{"collections", "doc", "count", "find"} {
		o, _, _ := baseOptions(t, env)
		calls := recordRun(o, "")
		o.Cmd, o.Arg = cmd, "users"
		if err := Run(o); err != nil {
			t.Fatalf("Run(%s): %v", cmd, err)
		}
		if len(*calls) != 1 {
			t.Fatalf("Run(%s): %d calls, want 1", cmd, len(*calls))
		}
		c := (*calls)[0]
		if c.name != "mongosh" {
			t.Errorf("Run(%s): name = %q, want mongosh", cmd, c.name)
		}
		if c.args[0] != "--nodb" {
			t.Errorf("Run(%s): args[0] = %q, want --nodb", cmd, c.args[0])
		}
		for _, a := range c.args {
			if strings.Contains(a, "SECRETPW") {
				t.Errorf("Run(%s): URL secret on argv: %q", cmd, a)
			}
		}
		if !contains(c.extraEnv, "ZNF_MONGO_URL=mongodb://u:SECRETPW@h") {
			t.Errorf("Run(%s): extraEnv = %v, want ZNF_MONGO_URL", cmd, c.extraEnv)
		}
	}
}

func TestRun_Find_ScriptCarriesFlags(t *testing.T) {
	o, _, _ := baseOptions(t, fakeEnv())
	calls := recordRun(o, "")
	o.Cmd, o.Arg = "find", "users"
	o.Filter, o.Projection, o.Limit = `{"role":"agent"}`, `{"_id":1}`, 2
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	args := (*calls)[0].args
	wantPrefix := []string{"--nodb", "--quiet", "--eval"}
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4 elements", args)
	}
	for i, w := range wantPrefix {
		if args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, args[i], w)
		}
	}
	script := args[3]
	for _, frag := range []string{`d.getCollection("users").find(`, `{\"role\":\"agent\"}`, `{\"_id\":1}`, ".limit(3)", "r.length>2"} {
		if !strings.Contains(script, frag) {
			t.Errorf("script = %q, missing %q", script, frag)
		}
	}
}

func TestRun_Collections_SubstringInScript(t *testing.T) {
	o, stdout, _ := baseOptions(t, fakeEnv())
	calls := recordRun(o, "chat_messages\nchat_rooms\n")
	o.Cmd, o.Arg = "collections", "Chat"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains((*calls)[0].args[3], `includes("chat")`) {
		t.Errorf("script = %q, want lower-cased substring filter", (*calls)[0].args[3])
	}
	if stdout.String() != "chat_messages\nchat_rooms\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRun_Tables_FilteredNotCapped(t *testing.T) {
	o, stdout, _ := baseOptions(t, fakeEnv())
	var b strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&b, "t%d\n", i)
	}
	b.WriteString("agents\n")
	calls := recordRun(o, b.String())
	o.Cmd, o.Arg = "tables", "AGENT"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != "agents" {
		t.Errorf("stdout = %q, want the 61st line to survive the filter", stdout.String())
	}
	if (*calls)[0].name != "mysql" {
		t.Errorf("name = %q, want mysql", (*calls)[0].name)
	}
	if stmt := strings.Join((*calls)[0].args, " "); strings.Contains(stmt, "SQL_SELECT_LIMIT") {
		t.Errorf("tables statement carries SQL_SELECT_LIMIT: %q", stmt)
	}
}

func TestRun_EvalRemoved(t *testing.T) {
	o, _, _ := baseOptions(t, fakeEnv())
	calls := recordRun(o, "")
	o.Cmd, o.Arg = "eval", "db.users.find()"
	err := Run(o)
	if err == nil || !strings.Contains(err.Error(), "find/aggregate") {
		t.Fatalf("err = %v, want removal message naming find/aggregate", err)
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("exit code = %d, want BadArgs", exitcode.Code(err))
	}
	if len(*calls) != 0 {
		t.Error("run seam invoked for removed eval")
	}
}

func TestRun_SQL_Refused(t *testing.T) {
	o, _, _ := baseOptions(t, fakeEnv())
	calls := recordRun(o, "")
	o.Cmd, o.Arg = "sql", "DROP TABLE t"
	err := Run(o)
	if err == nil {
		t.Fatal("Run: want error for DROP TABLE")
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("exit code = %d, want BadArgs", exitcode.Code(err))
	}
	if len(*calls) != 0 {
		t.Error("run seam invoked for a refused statement")
	}
}

func TestRun_SQL_Args(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	calls := recordRun(o, "1\n")
	o.Cmd, o.Arg = "sql", "SELECT id FROM agents"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := (*calls)[0]
	if c.name != "mysql" {
		t.Errorf("name = %q, want mysql", c.name)
	}
	for _, w := range []string{"--binary-mode", "--quick"} {
		if !contains(c.args, w) {
			t.Errorf("args = %v, want %s", c.args, w)
		}
	}
	for _, w := range []string{"-h", "mysql.internal", "-P", "3306", "-u", "clusteradmin", "-N", "-B"} {
		if !contains(c.args, w) {
			t.Errorf("args = %v, missing %q", c.args, w)
		}
	}
	if c.args[len(c.args)-1] != "db_acd" {
		t.Errorf("last arg = %q, want database db_acd", c.args[len(c.args)-1])
	}
	e := -1
	for i, a := range c.args {
		if a == "-e" {
			e = i
		}
	}
	if e < 0 || e+1 >= len(c.args) {
		t.Fatalf("args = %v, want -e <stmt>", c.args)
	}
	stmt := c.args[e+1]
	if !strings.HasPrefix(stmt, "START TRANSACTION READ ONLY;") {
		t.Errorf("-e = %q, want read-only txn prefix", stmt)
	}
	if !strings.Contains(stmt, "SELECT id FROM agents;") || !strings.HasSuffix(stmt, "ROLLBACK;") {
		t.Errorf("-e = %q, want user stmt wrapped and rolled back", stmt)
	}
	if !contains(c.extraEnv, "MYSQL_PWD="+fakeMysqlPassword) {
		t.Errorf("extraEnv = %v, want MYSQL_PWD", c.extraEnv)
	}
	for _, a := range c.args {
		if strings.Contains(a, fakeMysqlPassword) {
			t.Errorf("password on argv: %q", a)
		}
	}
	if stdout.String() != "1\n" || stderr.String() != "" {
		t.Errorf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRun_SQL_Cap(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	var in, want strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&in, "row%d\n", i)
		if i <= 50 {
			fmt.Fprintf(&want, "row%d\n", i)
		}
	}
	recordRun(o, in.String())
	o.Cmd, o.Arg = "sql", "SELECT id FROM agents"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != want.String() {
		t.Errorf("stdout =\n%q\nwant\n%q", stdout.String(), want.String())
	}
	if got := stderr.String(); got != "… đã cắt ở 50 dòng (--limit, tối đa 200)\n" { //znf:allow-lang
		t.Errorf("stderr = %q, want the truncation notice", got)
	}
}

// The real runner kills the child once the cap writer refuses a write, so the
// child returns an error; truncation must still be a success.
func TestRun_SQL_CapStopsChild(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	written := 0
	o.SetRun(func(string, []string, []string, string) error {
		for i := 1; i <= 100000; i++ {
			if _, err := fmt.Fprintf(o.Stdout, "row%d\n", i); err != nil {
				return errors.New("signal: killed")
			}
			written++
		}
		return nil
	})
	o.Cmd, o.Arg, o.Limit = "sql", "SELECT id FROM t LIMIT 10000000", 5
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v, want nil on truncation", err)
	}
	if written != 5 {
		t.Errorf("child wrote %d rows after the cap, want it stopped at 5", written)
	}
	if stdout.String() != "row1\nrow2\nrow3\nrow4\nrow5\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "đã cắt ở 5 dòng") { //znf:allow-lang
		t.Errorf("stderr = %q, want notice", stderr.String())
	}
}

func TestRun_FlagCombinationsRefused(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Options)
	}{
		{"find+pipeline", func(o *Options) { o.Cmd = "find"; o.Pipeline = `[{"$match":{}}]` }},
		{"count+pipeline", func(o *Options) { o.Cmd = "count"; o.Pipeline = `[{"$match":{}}]` }},
		{"aggregate+filter", func(o *Options) { o.Cmd = "aggregate"; o.Pipeline = `[{"$match":{}}]`; o.Filter = `{"a":1}` }},
		{"aggregate+projection", func(o *Options) { o.Cmd = "aggregate"; o.Pipeline = `[{"$match":{}}]`; o.Projection = `{"a":1}` }},
		{"aggregate+sort", func(o *Options) { o.Cmd = "aggregate"; o.Pipeline = `[{"$match":{}}]`; o.Sort = `{"a":1}` }},
		{"explain+pipeline+filter", func(o *Options) { o.Cmd = "explain"; o.Pipeline = `[{"$match":{}}]`; o.Filter = `{"a":1}` }},
		{"explain+pipeline+sort", func(o *Options) { o.Cmd = "explain"; o.Pipeline = `[{"$match":{}}]`; o.Sort = `{"a":1}` }},
	}
	for _, c := range cases {
		o, _, _ := baseOptions(t, fakeEnv())
		calls := recordRun(o, "")
		o.Arg = "users"
		c.set(o)
		if err := Run(o); exitcode.Code(err) != exitcode.BadArgs {
			t.Errorf("%s: err = %v, want BadArgs", c.name, err)
		}
		if len(*calls) != 0 {
			t.Errorf("%s: seam invoked", c.name)
		}
	}
	o, _, _ := baseOptions(t, fakeEnv())
	recordRun(o, "")
	o.Cmd, o.Arg, o.Filter, o.Sort = "explain", "users", `{"a":1}`, `{"b":1}`
	if err := Run(o); err != nil {
		t.Errorf("explain --filter --sort: %v, want nil", err)
	}
}

func TestRun_UnusedFlagsRefused(t *testing.T) {
	flags := map[string]func(*Options){
		"--filter":     func(o *Options) { o.Filter = `{"a":1}` },
		"--projection": func(o *Options) { o.Projection = `{"a":1}` },
		"--sort":       func(o *Options) { o.Sort = `{"a":1}` },
		"--pipeline":   func(o *Options) { o.Pipeline = `[{"$match":{}}]` },
	}
	cases := []struct {
		cmd, arg string
		flags    []string
	}{
		{"collections", "", []string{"--filter", "--projection", "--sort", "--pipeline"}},
		{"tables", "", []string{"--filter", "--projection", "--sort", "--pipeline"}},
		{"doc", "users", []string{"--filter", "--projection", "--sort", "--pipeline"}},
		{"sql", "SELECT 1", []string{"--filter", "--projection", "--sort", "--pipeline"}},
		{"count", "users", []string{"--projection", "--sort", "--pipeline"}},
	}
	for _, c := range cases {
		for _, f := range c.flags {
			o, _, _ := baseOptions(t, fakeEnv())
			calls := recordRun(o, "")
			o.Cmd, o.Arg = c.cmd, c.arg
			flags[f](o)
			err := Run(o)
			if exitcode.Code(err) != exitcode.BadArgs {
				t.Errorf("%s %s: err = %v, want BadArgs", c.cmd, f, err)
				continue
			}
			if !strings.Contains(err.Error(), f) {
				t.Errorf("%s %s: err = %v, want it to name the flag", c.cmd, f, err)
			}
			if len(*calls) != 0 {
				t.Errorf("%s %s: seam invoked", c.cmd, f)
			}
		}
	}
	// Blank flags are as good as absent; count keeps --filter.
	o, _, _ := baseOptions(t, fakeEnv())
	recordRun(o, "")
	o.Cmd, o.Filter = "collections", "  "
	if err := Run(o); err != nil {
		t.Errorf("collections --filter blank: %v, want nil", err)
	}
	o, _, _ = baseOptions(t, fakeEnv())
	recordRun(o, "")
	o.Cmd, o.Arg, o.Filter = "count", "users", `{"a":1}`
	if err := Run(o); err != nil {
		t.Errorf("count --filter: %v, want nil", err)
	}
}

func TestRun_SQL_ExactLimitNotTruncated(t *testing.T) {
	o, stdout, _ := baseOptions(t, fakeEnv())
	recordRun(o, "a\nb\nc\n")
	o.Cmd, o.Arg, o.Limit = "sql", "SELECT 1", 3
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != "a\nb\nc\n" {
		t.Errorf("stdout = %q, want no truncation line", stdout.String())
	}
}

func TestRun_LimitOutOfRange(t *testing.T) {
	for _, tc := range []struct {
		cmd   string
		limit int
	}{{"find", 500}, {"sql", 500}, {"aggregate", -1}} {
		o, _, _ := baseOptions(t, fakeEnv())
		calls := recordRun(o, "")
		o.Cmd, o.Arg, o.Limit = tc.cmd, "users", tc.limit
		if tc.cmd == "aggregate" {
			o.Pipeline = `[{"$match":{}}]`
		}
		if tc.cmd == "sql" {
			o.Arg = "SELECT 1"
		}
		err := Run(o)
		if err == nil {
			t.Errorf("Run(%s, limit %d): want error", tc.cmd, tc.limit)
		}
		if exitcode.Code(err) != exitcode.BadArgs {
			t.Errorf("Run(%s): exit code = %d, want BadArgs", tc.cmd, exitcode.Code(err))
		}
		if len(*calls) != 0 {
			t.Errorf("Run(%s): seam invoked despite bad limit", tc.cmd)
		}
	}
}

func TestRun_FindLimitTooHigh(t *testing.T) {
	o, _, _ := baseOptions(t, fakeEnv())
	calls := recordRun(o, "")
	o.Cmd, o.Arg, o.Limit = "find", "users", 500
	if err := Run(o); err == nil {
		t.Fatal("Run: want error for --limit 500")
	}
	if len(*calls) != 0 {
		t.Error("run seam invoked for --limit 500")
	}
}

func TestRun_Aggregate_PipelineRequiredAndWriteStageRefused(t *testing.T) {
	for _, pl := range []string{"", `[{"$out":"x"}]`} {
		o, _, _ := baseOptions(t, fakeEnv())
		calls := recordRun(o, "")
		o.Cmd, o.Arg, o.Pipeline = "aggregate", "users", pl
		if err := Run(o); exitcode.Code(err) != exitcode.BadArgs {
			t.Errorf("pipeline %q: err = %v, want BadArgs", pl, err)
		}
		if len(*calls) != 0 {
			t.Errorf("pipeline %q: seam invoked", pl)
		}
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	o, _, stderr := baseOptions(t, fakeEnv())
	calls := recordRun(o, "")
	o.Cmd = "distinct"
	err := Run(o)
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v, want BadArgs", err)
	}
	if len(*calls) != 0 {
		t.Error("seam invoked for unknown command")
	}
	if !strings.Contains(stderr.String(), "aggregate") {
		t.Errorf("usage = %q, want new commands listed", stderr.String())
	}
}

func TestRun_NoMongoURL_FatalNoRun(t *testing.T) {
	o, _, stderr := baseOptions(t, map[string]string{})
	calls := recordRun(o, "")
	o.Cmd = "collections"
	if err := Run(o); err == nil {
		t.Fatal("Run: want error when MONGO_URL is unset")
	}
	if len(*calls) != 0 {
		t.Error("run seam was invoked despite missing MONGO_URL")
	}
	if !strings.Contains(stderr.String(), "MONGO_URL is not set") {
		t.Errorf("stderr = %q, want MONGO_URL message", stderr.String())
	}
}

func TestRun_ArgRequired(t *testing.T) {
	for _, cmd := range []string{"doc", "count", "find", "aggregate", "explain", "sql"} {
		o, _, _ := baseOptions(t, fakeEnv())
		calls := recordRun(o, "")
		o.Cmd, o.Arg = cmd, ""
		o.Pipeline = `[{"$match":{}}]`
		err := Run(o)
		if exitcode.Code(err) != exitcode.BadArgs {
			t.Errorf("Run(%s): err = %v, want BadArgs for empty arg", cmd, err)
		}
		if len(*calls) != 0 {
			t.Errorf("Run(%s): run seam invoked despite missing arg", cmd)
		}
	}
}

func TestRun_FR041_NoSecretLeak(t *testing.T) {
	// The fake MONGO_URL and MySQL password must never land in captured
	// output, success or failure.
	scenarios := []func(*Options){
		func(o *Options) { o.Cmd = "collections" },
		func(o *Options) { o.Cmd = "find"; o.Arg = "tickets" },
		func(o *Options) { o.Cmd = "sql"; o.Arg = "SELECT 1" },
	}
	for _, fail := range []bool{false, true} {
		for _, setup := range scenarios {
			o, stdout, stderr := baseOptions(t, fakeEnv())
			o.SetRun(func(name string, args []string, extraEnv []string, stdin string) error {
				_, _ = fmt.Fprint(o.Stdout, "some-output\n")
				if fail {
					return errors.New("exit status 1")
				}
				return nil
			})
			setup(o)
			err := Run(o)
			if (err != nil) != fail {
				t.Fatalf("Run(%s) fail=%v: err = %v", o.Cmd, fail, err)
			}
			combined := stdout.String() + stderr.String()
			if err != nil {
				combined += err.Error()
			}
			if strings.Contains(combined, fakeMongoURL) {
				t.Errorf("Run(%s): MONGO_URL leaked into output: %q", o.Cmd, combined)
			}
			if strings.Contains(combined, fakeMysqlPassword) {
				t.Errorf("Run(%s): MySQL password leaked into output: %q", o.Cmd, combined)
			}
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

func TestCapWriter(t *testing.T) {
	const big = 1 << 20
	cases := []struct {
		name             string
		chunks           []string
		limit, maxBytes  int
		want             string
		lineCut, byteCut bool
	}{
		{"split across writes", []string{"a\nb", "\nc\n", "d\n"}, 2, big, "a\nb\n", true, false},
		{"one write over cap", []string{"a\nb\nc\n"}, 1, big, "a\n", true, false},
		{"exactly limit", []string{"a\n", "b\n"}, 2, big, "a\nb\n", false, false},
		{"under limit no newline", []string{"a\nb"}, 2, big, "a\nb", false, false},
		{"bytes at line boundary", []string{"aaa\nbbb\n", "ccc\n"}, -1, 8, "aaa\nbbb\n", false, true},
		{"half line dropped", []string{"aaa\nbb", "bbbb\n"}, -1, 6, "aaa\n", false, true},
		{"single line over cap is cut", []string{"abc", "defghij\n"}, -1, 4, "abcd", false, true},
		{"exact byte cap", []string{"ab\ncd\n"}, -1, 6, "ab\ncd\n", false, false},
		{"byte cap before line cap", []string{"aaa\nbbb\nccc\n"}, 3, 9, "aaa\nbbb\n", false, true},
		{"no line cap", []string{"a\nb\nc\n"}, -1, big, "a\nb\nc\n", false, false},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		w := newCapWriter(&buf, c.limit, c.maxBytes)
		var werr error
		for _, ch := range c.chunks {
			n, err := w.Write([]byte(ch))
			if err != nil {
				if !errors.Is(err, errCapReached) || n >= len(ch) {
					t.Errorf("%s: Write(%q) = %d, %v; want short write + errCapReached", c.name, ch, n, err)
				}
				werr = err
				break
			}
			if n != len(ch) {
				t.Errorf("%s: Write(%q) = %d, want %d", c.name, ch, n, len(ch))
			}
		}
		if err := w.Flush(); err != nil {
			t.Errorf("%s: Flush = %v", c.name, err)
		}
		cut := c.lineCut || c.byteCut
		if (werr != nil) != cut {
			t.Errorf("%s: write error = %v, want error only when truncated", c.name, werr)
		}
		if buf.String() != c.want {
			t.Errorf("%s: out = %q, want %q", c.name, buf.String(), c.want)
		}
		if w.Truncated() != cut || w.BytesCut() != c.byteCut {
			t.Errorf("%s: Truncated = %v, BytesCut = %v; want %v, %v", c.name, w.Truncated(), w.BytesCut(), cut, c.byteCut)
		}
	}
}

// killOnCap is a fake child that writes rows until the writer refuses, then
// fails the way a cancelled mongosh/mysql does.
func killOnCap(o *Options, row string, rows int) {
	o.SetRun(func(string, []string, []string, string) error {
		for i := 0; i < rows; i++ {
			if _, err := fmt.Fprint(o.Stdout, row); err != nil {
				return errors.New("signal: killed")
			}
		}
		return nil
	})
}

const byteNoticeWant = "… đã cắt ở 1048576 byte (giới hạn output)\n" //znf:allow-lang

func TestRun_ByteCap_SQL(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	row := strings.Repeat("x", 200000) + "\n"
	killOnCap(o, row, 10)
	o.Cmd, o.Arg = "sql", "SELECT body FROM t"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v, want nil on truncation", err)
	}
	if stdout.String() != strings.Repeat(row, 5) {
		t.Errorf("stdout = %d bytes, want 5 whole rows (%d bytes)", stdout.Len(), 5*len(row))
	}
	if stderr.String() != byteNoticeWant {
		t.Errorf("stderr = %q, want the byte notice", stderr.String())
	}
}

func TestRun_ByteCap_Mongo(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	row := strings.Repeat("y", 1000) + "\n"
	killOnCap(o, row, 3000)
	o.Cmd, o.Arg = "find", "users"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v, want nil on truncation", err)
	}
	if want := (MaxOutputBytes / len(row)) * len(row); stdout.Len() != want || !strings.HasSuffix(stdout.String(), "\n") {
		t.Errorf("stdout = %d bytes, want %d ending on a line break", stdout.Len(), want)
	}
	if stderr.String() != byteNoticeWant {
		t.Errorf("stderr = %q, want only the byte notice (no mongosh failure hint)", stderr.String())
	}
}

func TestRun_ByteCap_Tables(t *testing.T) {
	o, stdout, stderr := baseOptions(t, fakeEnv())
	row := "agents_" + strings.Repeat("z", 1000) + "\n"
	recordRun(o, strings.Repeat(row, 2000))
	o.Cmd, o.Arg = "tables", "agents"
	if err := Run(o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.Len() > MaxOutputBytes || stdout.Len() == 0 {
		t.Errorf("stdout = %d bytes, want 1..%d", stdout.Len(), MaxOutputBytes)
	}
	if stderr.String() != byteNoticeWant {
		t.Errorf("stderr = %q, want the byte notice", stderr.String())
	}
}

func TestRun_NoWorkspaceNoEnv_FriendlyError(t *testing.T) {
	var out, errb bytes.Buffer
	o := &Options{Cmd: "collections", Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }}
	o.SetRun(func(string, []string, []string, string) error { t.Fatal("must not run mongosh"); return nil })
	err := Run(o)
	if err == nil || !strings.Contains(errb.String(), "chưa có workspace") { //znf:allow-lang
		t.Fatalf("err=%v stderr=%q", err, errb.String())
	}
}
