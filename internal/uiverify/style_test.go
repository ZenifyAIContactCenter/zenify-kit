package uiverify

import (
	"reflect"
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	all := []string{StyleHex, StylePalette, StyleArbitrary}
	cases := []struct {
		line  string
		rules []string
		want  []string // "rule match"
	}{
		{"color: #fff", []string{StyleHex}, []string{"hex #fff"}},
		{"color: #ffff", []string{StyleHex}, []string{"hex #ffff"}},
		{"color: #ff0000", []string{StyleHex}, []string{"hex #ff0000"}},
		{"color: #ff0000aa", []string{StyleHex}, []string{"hex #ff0000aa"}},
		{`className="text-[#ff0000]"`, []string{StyleHex}, []string{"hex #ff0000"}},
		{"#content { }", []string{StyleHex}, nil},
		{`href="#section"`, []string{StyleHex}, nil},
		{"#abc123z", []string{StyleHex}, nil},
		{"#ff000g", []string{StyleHex}, nil},
		{"bg-red-500", []string{StylePalette}, []string{"palette bg-red-500"}},
		{"hover:text-slate-900/50", []string{StylePalette}, []string{"palette text-slate-900/50"}},
		{"border-zinc-950", []string{StylePalette}, []string{"palette border-zinc-950"}},
		{`"bg-red-500 text-blue-50"`, []string{StylePalette}, []string{"palette bg-red-500", "palette text-blue-50"}},
		{"text-primary bg-background", []string{StylePalette}, nil},
		{"bg-red-5000", []string{StylePalette}, nil},
		{"mybg-red-500", []string{StylePalette}, nil},
		{"w-[437px]", []string{StyleArbitrary}, []string{"arbitrary w-[437px]"}},
		{"mt-[1.5rem]", []string{StyleArbitrary}, []string{"arbitrary mt-[1.5rem]"}},
		{"w-full grid-cols-[1fr_auto] w-[50%]", []string{StyleArbitrary}, nil},
		{"#fff bg-red-500", all, []string{"hex #fff", "palette bg-red-500"}},
		{"#fff bg-red-500 w-[4px]", []string{StylePalette}, []string{"palette bg-red-500"}},
	}
	for _, tc := range cases {
		var got []string
		for _, v := range matchRules(tc.line, tc.rules) {
			got = append(got, v.Rule+" "+v.Match)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.line, got, tc.want)
		}
	}
}

const twoHunks = `diff --git a/src/A.tsx b/src/A.tsx
index 1..2 100644
--- a/src/A.tsx
+++ b/src/A.tsx
@@ -3,4 +10,5 @@ ctx
 context
-removed #000
+added #111
 context2
+added #222
@@ -30,2 +40,3 @@
 c
+added #333
-gone
 c
`

func TestScanAdded_LineNumbers(t *testing.T) {
	got, checked := scanAdded(twoHunks, []string{"src/A.tsx"}, []string{StyleHex})
	want := []Violation{
		{File: "src/A.tsx", Line: 11, Rule: "hex", Match: "#111"},
		{File: "src/A.tsx", Line: 13, Rule: "hex", Match: "#222"},
		{File: "src/A.tsx", Line: 41, Rule: "hex", Match: "#333"},
	}
	if !reflect.DeepEqual(got, want) || checked != 3 {
		t.Fatalf("got %+v checked=%d, want %+v checked=3", got, checked, want)
	}
}

func styleDiff(body string) string {
	return "diff --git a/src/A.tsx b/src/A.tsx\n--- a/src/A.tsx\n+++ b/src/A.tsx\n@@ -1,3 +1,4 @@\n" + body
}

func TestScanAdded_Waiver(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"marker on same line", "+x #fff // znf:ui-style-ok: brand\n", 0},
		{"marker on previous added line", "+// znf:ui-style-ok: brand\n+x #fff\n", 0},
		{"marker covers only the next line", "+// znf:ui-style-ok: brand\n+ok\n+x #fff\n", 1},
		{"marker without reason", "+// znf:ui-style-ok:\n+x #fff\n", 1},
		{"marker in context line", " // znf:ui-style-ok: brand\n+x #fff\n", 1},
		{"p0 marker does not waive", "+x #fff // znf:ui-verify-ok: copy only\n", 1},
		{"context line breaks the chain", "+// znf:ui-style-ok: brand\n ctx\n+x #fff\n", 1},
	}
	for _, tc := range cases {
		got, _ := scanAdded(styleDiff(tc.body), []string{"src/A.tsx"}, []string{StyleHex})
		if len(got) != tc.want {
			t.Errorf("%s: %d violations, want %d (%+v)", tc.name, len(got), tc.want, got)
		}
	}
}

func TestScanAdded_Scope(t *testing.T) {
	var b strings.Builder
	for _, f := range []string{"src/A.tsx", "src/B.ts", "src/C.css", "src/Other.tsx"} {
		b.WriteString("diff --git a/" + f + " b/" + f + "\n--- a/" + f + "\n+++ b/" + f + "\n@@ -0,0 +1 @@\n+x #fff\n")
	}
	files := []string{"src/A.tsx", "src/B.ts", "src/C.css"}
	got, checked := scanAdded(b.String(), files, []string{StyleHex})
	if len(got) != 1 || got[0].File != "src/A.tsx" || checked != 1 {
		t.Fatalf("got %+v checked=%d, want only src/A.tsx", got, checked)
	}
}

func TestNewFileDiff_AllLinesAdded(t *testing.T) {
	d := newFileDiff("src/A.tsx", "a\n// znf:ui-style-ok: brand\nb #fff\nc #000\n")
	vs, checked := scanAdded(d, []string{"src/A.tsx"}, []string{StyleHex})
	if checked != 4 || len(vs) != 1 || vs[0].Line != 4 || vs[0].Match != "#000" {
		t.Fatalf("checked=%d vs=%+v, want 4 lines and only #000 on line 4", checked, vs)
	}
	if newFileDiff("src/A.tsx", "") != "" {
		t.Fatal("empty file must yield no diff")
	}
}

// A comment closer is not a reason: "{/* znf:ui-style-ok: */}" must not waive.
func TestScanAdded_WaiverNeedsRealReason(t *testing.T) {
	for _, tc := range []struct {
		marker string
		want   int
	}{
		{"{/* znf:ui-style-ok: */}", 1},
		{"<!-- znf:ui-style-ok: -->", 1},
		{"// znf:ui-style-ok: ...", 1},
		{"{/* znf:ui-style-ok: partner brand */}", 0},
	} {
		diff := "diff --git a/src/A.tsx b/src/A.tsx\n--- a/src/A.tsx\n+++ b/src/A.tsx\n@@ -1,0 +1,2 @@\n+" +
			tc.marker + "\n+<b className=\"text-[#fff]\" />\n"
		got, _ := scanAdded(diff, []string{"src/A.tsx"}, []string{StyleHex})
		if len(got) != tc.want {
			t.Errorf("%q: violations=%v, want %d", tc.marker, got, tc.want)
		}
	}
}

// git quotes a path holding a quote, backslash or tab even with
// core.quotePath=false; the header must still map to the render-set name.
func TestScanAdded_QuotedPath(t *testing.T) {
	diff := "diff --git \"a/src/q\\\"t.tsx\" \"b/src/q\\\"t.tsx\"\n--- /dev/null\n+++ \"b/src/q\\\"t.tsx\"\n@@ -0,0 +1 @@\n+c #123456\n"
	got, _ := scanAdded(diff, []string{`src/q"t.tsx`}, []string{StyleHex})
	if len(got) != 1 || got[0].File != `src/q"t.tsx` {
		t.Fatalf("violations=%v, want 1 in src/q\"t.tsx", got)
	}
}

func TestRules_Widened(t *testing.T) {
	for _, tc := range []struct {
		line, rule string
		want       int
	}{
		{"border-t-red-500", StylePalette, 1},
		{"border-x-blue-500", StylePalette, 1},
		{"ring-offset-red-500", StylePalette, 1},
		{"divide-y-slate-200", StylePalette, 1},
		{"border-t-2", StylePalette, 0},
		{"top-[-13px]", StyleArbitrary, 1},
		{"w-[13PX]", StyleArbitrary, 1},
		{"h-[calc(100%-13px)]", StyleArbitrary, 0},
	} {
		if got := matchRules(tc.line, []string{tc.rule}); len(got) != tc.want {
			t.Errorf("%q: %v, want %d", tc.line, got, tc.want)
		}
	}
}
