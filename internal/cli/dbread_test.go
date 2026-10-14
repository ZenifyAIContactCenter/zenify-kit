package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

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
