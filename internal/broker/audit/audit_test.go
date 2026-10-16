package audit

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLog_OneLineContractKeys(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{W: &buf}
	l.Log(Entry{TS: time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC), Login: "octocat", Endpoint: "/query",
		Env: "stg", Op: "find", Args: map[string]any{"coll": "tickets"}, Rows: 3, Bytes: 120, MS: 42, Status: 200})
	l.Log(Entry{Login: "hubot", Endpoint: "/token", Status: 403})

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), buf.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"args", "bytes", "endpoint", "env", "login", "ms", "op", "role", "rows", "status", "ts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if m["ts"] != "2026-10-15T08:00:00Z" || m["login"] != "octocat" || m["rows"] != float64(3) || m["ms"] != float64(42) {
		t.Errorf("line = %s", lines[0])
	}
}
