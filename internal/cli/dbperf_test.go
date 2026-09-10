package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbperf"
)

func TestRunDbPerfNoQueryIsNoop(t *testing.T) {
	var out, errb bytes.Buffer
	err := runDbPerf("diff --git a/x.js b/x.js\n+++ b/x.js\n@@ -1 +1,2 @@\n+const y = 1\n", dbperf.Defaults(), false, &out, &errb)
	if err != nil {
		t.Fatalf("fail-open: want nil, got %v", err)
	}
	if !strings.Contains(out.String(), "không có query") { //znf:allow-lang
		t.Fatalf("want no-op message, got %q", out.String())
	}
}

func TestRunDbPerfBlockingExitsNonZeroViaFlag(t *testing.T) {
	var out, errb bytes.Buffer
	diff := "+++ b/s.js\n@@ -1 +1,2 @@\n+Model.find({s:1}).sort({t:-1})\n"
	_ = runDbPerf(diff, dbperf.Defaults(), true, &out, &errb)
	// JSON mode emits the Result; a BLOCKING finding must be present so the
	// caller (ship wiring) can detect it.
	if !strings.Contains(out.String(), `"tier":"BLOCKING"`) {
		t.Fatalf("want BLOCKING in json: %q", out.String())
	}
}

func TestRunDbPerfMalformedDiffFailOpen(t *testing.T) {
	var out, errb bytes.Buffer
	if err := runDbPerf("\x00not a diff", dbperf.Defaults(), false, &out, &errb); err != nil {
		t.Fatalf("must never error: %v", err)
	}
}
