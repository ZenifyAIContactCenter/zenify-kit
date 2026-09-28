package cli

import (
	"bytes"
	"encoding/json"
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

func TestRunDbPerfJSONEmitsBlockingTier(t *testing.T) {
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

const pyOnlyDiff = "diff --git a/app/x.py b/app/x.py\n+++ b/app/x.py\n@@ -1 +1,2 @@\n+rows = session.execute(q).all()\n"

func TestRunDbPerfZeroSitesJSONIsJSON(t *testing.T) {
	var out, errb bytes.Buffer
	_ = runDbPerf(pyOnlyDiff, dbperf.Defaults(), true, &out, &errb)
	var res struct {
		SitesScanned   int      `json:"sites_scanned"`
		UnscannedFiles []string `json:"unscanned_files"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("--json with 0 sites must emit JSON: %v (%q)", err, out.String())
	}
	if res.SitesScanned != 0 || len(res.UnscannedFiles) != 1 || res.UnscannedFiles[0] != "app/x.py" {
		t.Fatalf("got %+v", res)
	}
}

func TestRunDbPerfZeroSitesPlainNamesUnsupportedStack(t *testing.T) {
	var out, errb bytes.Buffer
	_ = runDbPerf(pyOnlyDiff, dbperf.Defaults(), false, &out, &errb)
	if !strings.Contains(out.String(), "coverage: unsupported stack") {
		t.Fatalf("want coverage line, got %q", out.String())
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("0-site plain output must carry no ANSI: %q", out.String())
	}
}

func TestRunDbPerfZeroSitesJSOnlyHasNoCoverageLine(t *testing.T) {
	var out, errb bytes.Buffer
	_ = runDbPerf("+++ b/x.js\n@@ -1 +1,2 @@\n+const y = 1\n", dbperf.Defaults(), false, &out, &errb)
	if strings.Contains(out.String(), "coverage:") {
		t.Fatalf("a JS-only diff must not print a coverage line: %q", out.String())
	}
}
