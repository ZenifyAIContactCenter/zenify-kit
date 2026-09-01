package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestRunChecksAggregatesHealth(t *testing.T) {
	saved := checks
	defer func() { checks = saved }()
	checks = []Check{
		{Name: "a", Run: func() (bool, string) { return true, "ok" }},
		{Name: "b", Run: func() (bool, string) { return false, "bad" }},
	}
	results, healthy := runChecks()
	if healthy {
		t.Fatal("healthy should be false when any check fails")
	}
	if len(results) != 2 || results[0].Name != "a" || results[1].OK {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestDoctorJSONEnvelope(t *testing.T) {
	saved := checks
	defer func() { checks = saved }()
	checks = []Check{{Name: "x", Run: func() (bool, string) { return true, "1.2.3" }}}

	cmd := newDoctorCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var env doctorEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not valid json: %v\n%s", err, out.String())
	}
	if env.SchemaVersion != 1 || !env.Data.Healthy || len(env.Data.Checks) != 1 || env.Data.Checks[0].Name != "x" {
		t.Fatalf("bad envelope: %+v", env)
	}
}

func TestDoctorExitOnFail(t *testing.T) {
	saved := checks
	defer func() { checks = saved }()
	checks = []Check{{Name: "bad", Run: func() (bool, string) { return false, "nope" }}}

	cmd := newDoctorCmd()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--exit-on-fail"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when a check fails under --exit-on-fail")
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("want exit code %d, got %d", exitcode.Fail, exitcode.Code(err))
	}
}

func TestDoctorNoExitByDefault(t *testing.T) {
	saved := checks
	defer func() { checks = saved }()
	checks = []Check{{Name: "bad", Run: func() (bool, string) { return false, "nope" }}}
	cmd := newDoctorCmd()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("default run must not error on unhealthy: %v", err)
	}
}
