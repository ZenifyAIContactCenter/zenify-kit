package dbperf

import "testing"

const sampleDiff = "diff --git a/svc.js b/svc.js\n" +
	"--- a/svc.js\n+++ b/svc.js\n" +
	"@@ -10,3 +10,4 @@\n" +
	" ctx()\n" +
	"+db.getCollection('tickets').find({_id:x})\n" +
	" other()\n" +
	"+Model.find({s:1}).sort({t:-1})\n"

func TestAddedLinesFileAndLineNumbers(t *testing.T) {
	got := AddedLines(sampleDiff)
	if len(got) != 2 {
		t.Fatalf("want 2 added lines, got %d: %+v", len(got), got)
	}
	if got[0].File != "svc.js" || got[0].Line != 11 {
		t.Fatalf("first added line wrong: %+v", got[0])
	}
	if got[1].Line != 13 || got[1].Text != "Model.find({s:1}).sort({t:-1})" {
		t.Fatalf("second added line wrong: %+v", got[1])
	}
}

func TestAddedLinesIgnoresHeaderPlusLines(t *testing.T) {
	// The "+++ b/file" header starts with '+' but is not content.
	for _, a := range AddedLines(sampleDiff) {
		if a.Text == "+ b/svc.js" || a.File == "" {
			t.Fatalf("header leaked as content: %+v", a)
		}
	}
}
