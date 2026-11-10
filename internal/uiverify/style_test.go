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
