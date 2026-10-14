package devenv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".zenify", ".env")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPath(t *testing.T) {
	if got := Path("/ws"); got != "/ws/.zenify/.env" {
		t.Fatalf("Path = %q", got)
	}
}

func TestLoad_MissingFileIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nope", ".env"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(f.Keys()) != 0 {
		t.Fatalf("keys = %v", f.Keys())
	}
}

func TestRoundTrip(t *testing.T) {
	vals := map[string]string{
		"HASH":   "a#b",
		"QUOTE":  "it's",
		"SPACE":  "p@ss w0rd",
		"EQ":     "x=y&z",
		"DOLLAR": "$HOME",
		"EMPTY":  "",
		"MIXED":  `it's "a" $x \ end`,
	}
	f := &File{}
	for k, v := range vals {
		f.Set(k, v)
	}
	p := filepath.Join(t.TempDir(), "sub", ".env")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range vals {
		got, ok := g.Get(k)
		if !ok || got != want {
			t.Errorf("%s = %q,%v want %q", k, got, ok, want)
		}
	}
	raw, _ := os.ReadFile(p)
	s := string(raw)
	if !strings.Contains(s, "HASH='a#b'\n") {
		t.Errorf("want single-quoted HASH, file:\n%s", s)
	}
	if !strings.Contains(s, `QUOTE="it's"`+"\n") {
		t.Errorf("want double-quoted QUOTE, file:\n%s", s)
	}
}

func TestParse_Forms(t *testing.T) {
	p := writeFile(t, "A=v\nB='v # x'\nC=\"v\"\nD=v # cmt\n\n# just a comment\nE=\n")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "v", "B": "v # x", "C": "v", "D": "v", "E": ""}
	if got := f.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values = %v want %v", got, want)
	}
	if !reflect.DeepEqual(f.Keys(), []string{"A", "B", "C", "D", "E"}) {
		t.Fatalf("Keys = %v", f.Keys())
	}
}

func TestDesc(t *testing.T) {
	p := writeFile(t, "# mô tả\nA=1\n# orphan\n\nB=2\nC=3\n") //znf:allow-lang
	f, _ := Load(p)
	if got := f.Desc("A"); got != "mô tả" { //znf:allow-lang
		t.Errorf("Desc(A) = %q", got)
	}
	if got := f.Desc("B"); got != "" {
		t.Errorf("Desc(B) = %q want empty", got)
	}
	f.SetDesc("A", "mới") //znf:allow-lang
	f.SetDesc("C", "cho C")
	if f.Desc("A") != "mới" || f.Desc("C") != "cho C" { //znf:allow-lang
		t.Errorf("SetDesc failed: A=%q C=%q", f.Desc("A"), f.Desc("C"))
	}
	out := filepath.Join(t.TempDir(), ".env")
	if err := f.Save(out); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(out)
	if strings.Contains(string(raw), "# mô tả") { //znf:allow-lang
		t.Errorf("old desc line not replaced:\n%s", raw)
	}
	if strings.Count(string(raw), "# mới") != 1 { //znf:allow-lang
		t.Errorf("want exactly one new desc:\n%s", raw)
	}
}

func TestSave_PreservesOrderAndComments(t *testing.T) {
	p := writeFile(t, "# head\nA=1\n\n# about B\nB=2\nC=3\n")
	f, _ := Load(p)
	f.Set("B", "two")
	f.Set("D", "4")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	// untouched lines stay byte-identical; only Set lines are rewritten
	want := "# head\nA=1\n\n# about B\nB='two'\nC=3\nD='4'\n"
	if string(raw) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", raw, want)
	}
}

func TestSave_Mode0600(t *testing.T) {
	p := writeFile(t, "A=1\n") // created 0644
	f, _ := Load(p)
	f.Set("A", "2")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", st.Mode().Perm())
	}
	// fresh file in a missing dir
	q := filepath.Join(t.TempDir(), "x", ".env")
	if err := (&File{}).Save(q); err != nil {
		t.Fatal(err)
	}
	st, _ = os.Stat(q)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("fresh mode = %v", st.Mode().Perm())
	}
}

func TestLookup_EmptyEnvFallsBack(t *testing.T) {
	f := &File{}
	f.Set("K", "fromfile")
	f.Set("J", "jfile")
	env := map[string]string{"K": "", "J": "fromenv"}
	get := Lookup(func(k string) string { return env[k] }, f)
	if got := get("K"); got != "fromfile" {
		t.Errorf("K = %q", got)
	}
	if got := get("J"); got != "fromenv" {
		t.Errorf("J = %q", got)
	}
	if got := get("MISSING"); got != "" {
		t.Errorf("MISSING = %q", got)
	}
}

func TestChildEnv(t *testing.T) {
	look := func(k string) string { return map[string]string{"A": "1", "B": ""}[k] }
	got := ChildEnv([]string{"X=y"}, []string{"A", "B", "C"}, look)
	want := []string{"X=y", "A=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSetDesc_NewlineCannotInjectKey(t *testing.T) {
	f := &File{}
	f.Set("K", "v12345")
	f.SetDesc("K", "x\nEVIL=1")
	if _, ok := f.Get("EVIL"); ok {
		t.Error("newline in desc created a key")
	}
	if got := f.Keys(); len(got) != 1 || got[0] != "K" {
		t.Errorf("keys = %v, want [K]", got)
	}
}
