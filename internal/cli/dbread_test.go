package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// Every case here fails validation before credentials are read, so no test
// reaches a real database even though cwd sits inside a workspace.
func runDBRead(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newDBReadCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestDBReadCmd_UseAndFlags(t *testing.T) {
	cmd := newDBReadCmd()
	if !strings.HasPrefix(cmd.Use, "db-read <collections|tables|doc|count|find|aggregate|explain|sql>") {
		t.Errorf("Use = %q", cmd.Use)
	}
	if strings.Contains(cmd.Use, "eval") {
		t.Errorf("Use still lists eval: %q", cmd.Use)
	}
	for _, f := range []string{"filter", "projection", "sort", "pipeline", "limit"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("flag --%s missing", f)
		}
	}
	if !strings.Contains(cmd.Long, `aggregate <collection> --pipeline '[{"$group":{"_id":"$field"}}]'`) {
		t.Errorf("Long lacks the distinct replacement example:\n%s", cmd.Long)
	}
}

func TestDBReadCmd_EvalRemoved_BadArgs(t *testing.T) {
	_, err := runDBRead(t, "eval", "db.users.find()")
	if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "find/aggregate") {
		t.Errorf("err = %v (code %d), want BadArgs removal message", err, exitcode.Code(err))
	}
}

func TestDBReadCmd_LimitTooHigh_BadArgs(t *testing.T) {
	_, err := runDBRead(t, "find", "users", "--limit", "500")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v (code %d), want BadArgs", err, exitcode.Code(err))
	}
}

func TestDBReadCmd_SQLWrite_BadArgs(t *testing.T) {
	_, err := runDBRead(t, "sql", "DROP TABLE t")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v (code %d), want BadArgs", err, exitcode.Code(err))
	}
}

// failAfter accepts n bytes, then fails every write, like the dbread cap writer.
type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, errors.New("cap reached")
	}
	k := min(len(p), f.n)
	f.n -= k
	if k < len(p) {
		return k, errors.New("cap reached")
	}
	return k, nil
}

// A refused stdout write must stop the child at once instead of letting it
// stream until the 60 s timeout.
func TestRunDBChild_StopsOnWriteError(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	// SIGPIPE ignored: closing the pipe alone would not stop this child.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	err := runDBChild(ctx, "sh", []string{"-c", `trap "" PIPE; while :; do echo y; done 2>/dev/null`}, nil, &failAfter{n: 1024}, io.Discard)
	if err == nil {
		t.Error("want an error from the killed child")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("child ran %v after the write error, want it stopped", d)
	}
}

// FR-10.1: --prd is wired through cobra into production mode. A stub mongosh on
// PATH records the URL variable it receives, so no real database is reached.
func TestDBReadCmd_PrdFlagSelectsProduction(t *testing.T) {
	isolateHome(t)
	t.Chdir(t.TempDir()) // no workspace marker above cwd: no real .zenify/.env is loaded
	if f := newDBReadCmd().Flags().Lookup("prd"); f == nil || f.Value.Type() != "bool" || f.DefValue != "false" {
		t.Fatalf("--prd flag = %+v, want bool default false", f)
	}
	bin := t.TempDir()
	rec := filepath.Join(t.TempDir(), "rec")
	stub := "#!/bin/sh\necho \"$" + dbread.MongoURLEnv + "\" > \"" + rec + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "mongosh"), []byte(stub), 0o700); err != nil { //nolint:gosec // G306 -- test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MONGO_URL", "fake-dev-url")
	t.Setenv("MONGO_URL_PRD", "fake-prd-url")
	mp := noBrokerManifest(t) // the embedded manifest declares a broker; this test covers the direct path

	for _, tc := range []struct {
		args []string
		url  string
		prod bool
	}{
		{[]string{"--prd", "count", "users"}, "fake-prd-url", true},
		{[]string{"count", "users"}, "fake-dev-url", false},
	} {
		out, err := runDBRead(t, append([]string{"--manifest", mp}, tc.args...)...)
		if err != nil {
			t.Fatalf("%v: err = %v\n%s", tc.args, err, out)
		}
		if got := strings.Contains(out, "production (MONGO_URL_PRD)"); got != tc.prod {
			t.Errorf("%v: production notice = %v, want %v:\n%s", tc.args, got, tc.prod, out)
		}
		b, _ := os.ReadFile(rec) //nolint:gosec // G304 -- test temp path
		if strings.TrimSpace(string(b)) != tc.url {
			t.Errorf("%v: child got %q, want %q", tc.args, b, tc.url)
		}
	}
}

// S6: with --prd, the child's stderr is masked — a driver error naming the
// production URL or its host must not reach the agent.
func TestDBReadCmd_PrdMasksChildStderr(t *testing.T) {
	isolateHome(t)
	t.Chdir(t.TempDir())
	bin := t.TempDir()
	stub := "#!/bin/sh\necho \"MongoServerSelectionError: connect ECONNREFUSED prodhost-a.example:27017 via $" + dbread.MongoURLEnv + "\" >&2\nprintf 'tail prodhost-b.example' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "mongosh"), []byte(stub), 0o700); err != nil { //nolint:gosec // G306 -- test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MONGO_URL_PRD", "mongodb://fakeuser:fakepass1@prodhost-a.example:27017,prodhost-b.example/db")

	out, err := runDBRead(t, "--manifest", noBrokerManifest(t), "--prd", "count", "users")
	if err == nil {
		t.Fatalf("want the stub's failure, got nil:\n%s", out)
	}
	for _, leak := range []string{"prodhost-a.example", "prodhost-b.example", "fakepass1", "mongodb://"} {
		if strings.Contains(out, leak) {
			t.Errorf("output leaks %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "MongoServerSelectionError") || !strings.Contains(out, "tail") {
		t.Errorf("masked stderr lost the rest of the message:\n%s", out)
	}
}
