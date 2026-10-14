package cli

import (
	"bytes"
	"strings"
	"testing"

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
