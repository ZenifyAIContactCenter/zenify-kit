package devenv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// load builds a File from text through the public loader.
func load(t *testing.T, content string) *File {
	t.Helper()
	f, err := Load(writeFile(t, content))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// text returns what Save writes for f.
func text(t *testing.T, f *File) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func key(name, group, desc, howTo, scope string, secret bool, repo string) manifest.PlacedKey {
	return manifest.PlacedKey{
		SecretKey: manifest.SecretKey{Name: name, Group: group, Description: desc, HowTo: howTo, Scope: scope, Secret: secret},
		Repo:      repo,
	}
}

// lines joins ls as file text, each line newline-terminated.
func lines(ls ...string) string {
	return strings.Join(ls, "\n") + "\n"
}

// scaffoldKeys is deliberately not in group order: Render must regroup.
var scaffoldKeys = []manifest.PlacedKey{
	key("MONGO_URL", "", "Mongo staging", "hỏi lead", "shared", true, ""), //znf:allow-lang
	key("MONGO_URL_PRD", manifest.GroupProduction, "Mongo prod", "devops", "shared", true, ""),
	key("MYSQL_URL", manifest.GroupStaging, "MySQL staging", "", "personal", true, "contact-center-be"),
	key("E2E_DOMAIN", manifest.GroupTooling, "Domain E2E", "", "", false, ""),
}

func TestRender_ScaffoldFromValueOnly(t *testing.T) {
	r := Render(load(t, "MONGO_URL='x'\n"), scaffoldKeys, nil)
	want := lines(
		"# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.", //znf:allow-lang
		"",
		"# ══ Staging ══",
		"# Mongo staging",
		"# Lấy: hỏi lead · shared · secret", //znf:allow-lang
		"MONGO_URL='x'",
		"",
		"# MySQL staging",
		"# personal · secret · contact-center-be",
		"MYSQL_URL=",
		"",
		"# ══ Tooling ══",
		"# Domain E2E",
		"# không secret", //znf:allow-lang
		"E2E_DOMAIN=",
		"",
		"# ══ Production ══",
		"# Mongo prod",
		"# Lấy: devops · shared · secret", //znf:allow-lang
		"MONGO_URL_PRD=",
	)
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if want := []string{"MYSQL_URL", "E2E_DOMAIN", "MONGO_URL_PRD"}; !reflect.DeepEqual(r.Added, want) {
		t.Fatalf("Added = %v, want %v", r.Added, want)
	}
	if len(r.Removed) != 0 || len(r.Undescribed) != 0 {
		t.Fatalf("Removed = %v, Undescribed = %v", r.Removed, r.Undescribed)
	}
}

func TestRender_NilOldIsEmptyFile(t *testing.T) {
	r := Render(nil, scaffoldKeys[:1], nil)
	want := "# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.\n\n" + //znf:allow-lang
		"# ══ Staging ══\n# Mongo staging\n# Lấy: hỏi lead · shared · secret\nMONGO_URL=\n" //znf:allow-lang
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestRender_Idempotent(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"manifest only": "MONGO_URL='x'\nE2E_DOMAIN=example.test\n",
		"custom and stray": "# note\n# mô tả\nMY_KEY='v'\n# OLD=1\n\n\nfoo bar\n" + //znf:allow-lang
			"# tự viết\nMONGO_URL=\"a'b\"\nOTHER=1\n# cuối\n", //znf:allow-lang
		"messy": "\n\nMONGO_URL='old'\n\n\nMONGO_URL='new'\nexport K='v'\nZ=1\n\n# x\nZ=2\n  SPACED = 'y'  \n#\nNODESC=1\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			once := text(t, Render(load(t, in), scaffoldKeys, nil).File)
			twice := text(t, Render(load(t, once), scaffoldKeys, nil).File)
			if once != twice {
				t.Fatalf("second render changed the file:\n%s\n---\n%s", once, twice)
			}
		})
	}
}

func TestRender_ValuesRoundTrip(t *testing.T) {
	vals := map[string]string{
		"MONGO_URL":     `a'b`,
		"MYSQL_URL":     `a"b`,
		"E2E_DOMAIN":    `$HOME`,
		"MONGO_URL_PRD": `x #y`,
		"CUSTOM_SP":     `  sp  `,
		"CUSTOM_URL":    `mongodb://u:p@h:1/db?a=b&c=d`,
		"CUSTOM_MIX":    `a'b"c$d` + "`e\\f",
	}
	old := &File{}
	for k, v := range vals {
		old.Set(k, v)
	}
	out := load(t, text(t, Render(old, scaffoldKeys, nil).File))
	for k, v := range vals {
		if got, ok := out.Get(k); !ok || got != v {
			t.Errorf("key %s: value changed or missing after render", k)
		}
	}
}

func TestRender_RemovesDroppedManagedKey(t *testing.T) {
	keys := []manifest.PlacedKey{key("A", "", "a", "", "", true, "")}
	old := load(t, "# b desc\n# Lấy: x · secret\nB='v'\nA='w'\n") //znf:allow-lang
	r := Render(old, keys, []string{"A", "B"})
	want := "# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.\n\n" + //znf:allow-lang
		"# ══ Staging ══\n# a\n# secret\nA='w'\n"
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if !reflect.DeepEqual(r.Removed, []string{"B"}) {
		t.Fatalf("Removed = %v, want [B]", r.Removed)
	}
}

func TestRender_FirstRunRemovesNothing(t *testing.T) {
	keys := []manifest.PlacedKey{key("A", "", "a", "", "", true, "")}
	r := Render(load(t, "OPENROUTER_API_KEY='v'\n"), keys, nil)
	want := "# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.\n\n" + //znf:allow-lang
		"# ══ Staging ══\n# a\n# secret\nA=\n\n" +
		"# ══ Custom (ngoài manifest) ══\nOPENROUTER_API_KEY='v'\n" //znf:allow-lang
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if len(r.Removed) != 0 {
		t.Fatalf("Removed = %v", r.Removed)
	}
	if !reflect.DeepEqual(r.Undescribed, []string{"OPENROUTER_API_KEY"}) {
		t.Fatalf("Undescribed = %v", r.Undescribed)
	}
}

func TestRender_CustomDescAndStrayLines(t *testing.T) {
	keys := []manifest.PlacedKey{key("A", "", "a", "", "", true, "")}
	r := Render(load(t, "# note\n# mô tả\nMY_KEY='v'\n# OLD=1\nA='w'\n"), keys, nil)                               //znf:allow-lang
	want := "# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.\n\n" + //znf:allow-lang
		"# ══ Staging ══\n# a\n# secret\nA='w'\n\n" +
		"# ══ Custom (ngoài manifest) ══\n# mô tả\nMY_KEY='v'\n\n# note\n# OLD=1\n" //znf:allow-lang
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if len(r.Undescribed) != 0 {
		t.Fatalf("Undescribed = %v", r.Undescribed)
	}
}

func TestRender_ManifestDescReplaced(t *testing.T) {
	r := Render(load(t, "# của tôi 1\n# của tôi 2\nMONGO_URL='x'\n"), scaffoldKeys[:1], nil)                       //znf:allow-lang
	want := "# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.\n\n" + //znf:allow-lang
		"# ══ Staging ══\n# Mongo staging\n# Lấy: hỏi lead · shared · secret\nMONGO_URL='x'\n" //znf:allow-lang
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestRender_CustomHeaderOnlyWhenNeeded(t *testing.T) {
	got := text(t, Render(load(t, "\n\nMONGO_URL='x'\n\n"), scaffoldKeys, nil).File)
	if strings.Contains(got, "# ══ Custom") {
		t.Fatalf("custom header printed without custom content:\n%s", got)
	}
}

func TestRender_MessyFileKeepsValues(t *testing.T) {
	old := load(t, "\n\nMONGO_URL='old'\n\n\nMONGO_URL='new'\nexport K='v'\nZ=1\n\n# x\nZ=2\n  SPACED = 'y'  \n")
	before := old.Values()
	out := load(t, text(t, Render(old, scaffoldKeys, nil).File))
	for k, v := range before {
		if got, ok := out.Get(k); !ok || got != v {
			t.Errorf("key %q: value changed or missing after render", k)
		}
	}
}

func TestRender_OutputParsesWithExistingLoader(t *testing.T) {
	old := load(t, "MONGO_URL='x'\nMY_KEY=\"a'b\"\n# OLD=1\n")
	r := Render(old, scaffoldKeys, nil)
	p := filepath.Join(t.TempDir(), ".zenify", ".env")
	if err := r.File.Save(p); err != nil {
		t.Fatal(err)
	}
	back, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"MONGO_URL": "x", "MY_KEY": "a'b", "MYSQL_URL": "", "E2E_DOMAIN": "", "MONGO_URL_PRD": ""}
	if got := back.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("keys after Save/Load = %v", back.Keys())
	}
}

// A manifest description shaped like an assignment must not be kept as a
// stray line on every render (that would grow the file each run).
func TestRender_IdempotentWithAssignmentShapedDescription(t *testing.T) {
	keys := []manifest.PlacedKey{key("A", "", "FOO=bar", "", "", true, "")}
	once := text(t, Render(load(t, "A='w'\n"), keys, nil).File)
	twice := text(t, Render(load(t, once), keys, nil).File)
	if once != twice || strings.Contains(once, "Custom") {
		t.Fatalf("second render changed the file:\n%s\n---\n%s", once, twice)
	}
}

// A commented-out assignment directly above a manifest key may hold an old
// value: it moves to the custom section instead of being dropped with the
// rest of that comment block.
func TestRender_CommentedAssignmentAboveManifestKeyKept(t *testing.T) {
	old := load(t, "# của tôi\n# OLD_MONGO=abc\nMONGO_URL='x'\n") //znf:allow-lang
	once := text(t, Render(old, scaffoldKeys[:1], nil).File)
	want := lines(
		"# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.", //znf:allow-lang
		"",
		"# ══ Staging ══",
		"# Mongo staging",
		"# Lấy: hỏi lead · shared · secret", //znf:allow-lang
		"MONGO_URL='x'",
		"",
		"# ══ Custom (ngoài manifest) ══", //znf:allow-lang
		"# OLD_MONGO=abc",
	)
	if once != want {
		t.Fatalf("output:\n%s\nwant:\n%s", once, want)
	}
	if twice := text(t, Render(load(t, once), scaffoldKeys[:1], nil).File); twice != once {
		t.Fatalf("second render changed the file:\n%s", twice)
	}
}

// A key whose group is not one of the three must not lose its value: it is
// rendered in the staging section.
func TestRender_UnknownGroupFoldsIntoStaging(t *testing.T) {
	keys := []manifest.PlacedKey{key("A", "bogus", "a", "", "", true, "")}
	r := Render(load(t, "A='w'\n"), keys, nil)
	want := lines(
		"# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.", //znf:allow-lang
		"",
		"# ══ Staging ══",
		"# a",
		"# secret",
		"A='w'",
	)
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestRender_DuplicateManifestNameRenderedOnce(t *testing.T) {
	keys := []manifest.PlacedKey{
		key("A", "", "a", "", "", true, ""),
		key("A", manifest.GroupTooling, "a again", "", "", true, "repo"),
	}
	r := Render(load(t, "A='w'\n"), keys, nil)
	want := lines(
		"# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.", //znf:allow-lang
		"",
		"# ══ Staging ══",
		"# a",
		"# secret",
		"A='w'",
	)
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestRender_CommentedAssignmentAboveRemovedKeyKept(t *testing.T) {
	keys := []manifest.PlacedKey{key("A", "", "a", "", "", true, "")}
	r := Render(load(t, "# b desc\n# OLD_B=1\nB='v'\nA='w'\n"), keys, []string{"A", "B"})
	want := lines(
		"# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.", //znf:allow-lang
		"",
		"# ══ Staging ══",
		"# a",
		"# secret",
		"A='w'",
		"",
		"# ══ Custom (ngoài manifest) ══", //znf:allow-lang
		"# OLD_B=1",
	)
	if got := text(t, r.File); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if !reflect.DeepEqual(r.Removed, []string{"B"}) {
		t.Fatalf("Removed = %v, want [B]", r.Removed)
	}
}
