package visual

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func writeRoutes(t *testing.T, repo, body string) {
	t.Helper()
	p := RoutesPath(repo)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRoutes(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string // "" = valid
	}{
		{"dup name", `[{"name":"a","path":"/a"},{"name":"a","path":"/b"}]`, "routes[1].name"},
		{"dup path", `[{"name":"a","path":"/a"},{"name":"b","path":"/a"}]`, "routes[1].path"},
		{"space in name", `[{"name":"a b","path":"/a"}]`, "routes[0].name"},
		{"dot in name", `[{"name":"a.b","path":"/a"}]`, "routes[0].name"},
		{"empty name", `[{"name":"","path":"/a"}]`, "routes[0].name"},
		{"path no slash", `[{"name":"a","path":"tickets"}]`, "routes[0].path"},
		{"empty waitFor", `[{"name":"a","path":"/a","waitFor":""}]`, "routes[0].waitFor"},
		{"empty mask item", `[{"name":"a","path":"/a","mask":[""]}]`, "routes[0].mask"},
		{"valid", `[{"name":"settings-webhook","path":"/settings/webhook","waitFor":"[data-sidebar=\"sidebar\"]","mask":["[data-testid=\"app-header\"]"],"aria":"#content"}]`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := t.TempDir()
			writeRoutes(t, repo, c.json)
			_, err := LoadRoutes(repo)
			if c.want == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want BadArgs containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestLoadRoutes(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := LoadRoutes(t.TempDir())
		if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "repo has no visual config") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unknown key", func(t *testing.T) {
		repo := t.TempDir()
		writeRoutes(t, repo, `[{"name":"a","path":"/a","area":"x"}]`)
		_, err := LoadRoutes(repo)
		if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "area") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("not an array", func(t *testing.T) {
		repo := t.TempDir()
		writeRoutes(t, repo, `{"name":"a"}`)
		if _, err := LoadRoutes(repo); exitcode.Code(err) != exitcode.BadArgs {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("empty array", func(t *testing.T) {
		repo := t.TempDir()
		writeRoutes(t, repo, `[]`)
		rs, err := LoadRoutes(repo)
		if err != nil || len(rs) != 0 {
			t.Fatalf("got %v, %v", rs, err)
		}
	})
}

const webRoutes = `[
  {"name":"home","path":"/","waitFor":"[data-sidebar=\"sidebar\"]","mask":["[data-testid=\"app-header\"]","[data-testid=\"clock\"]"]},
  {"name":"settings-webhook","path":"/settings/webhook","waitFor":"#content","mask":["[data-testid=\"app-header\"]","[data-testid=\"badge\"]"]}
]
`

func TestAddRoute_KeepsExisting(t *testing.T) {
	repo := t.TempDir()
	writeRoutes(t, repo, webRoutes)
	before, err := LoadRoutes(repo)
	if err != nil {
		t.Fatal(err)
	}
	nr := Route{Name: "ticket-list", Path: "/tickets", Mask: []string{`[data-testid="app-header"]`}}
	if err := AddRoute(repo, nr); err != nil {
		t.Fatal(err)
	}
	after, err := LoadRoutes(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 || !reflect.DeepEqual(after[:2], before) || !reflect.DeepEqual(after[2], nr) {
		t.Fatalf("got %+v", after)
	}
	b, _ := os.ReadFile(RoutesPath(repo))
	if !strings.HasSuffix(string(b), "]\n") {
		t.Errorf("file must end with newline: %q", b)
	}
}

func TestAddRoute_Duplicate(t *testing.T) {
	for _, nr := range []Route{{Name: "home", Path: "/new"}, {Name: "new", Path: "/settings/webhook"}} {
		repo := t.TempDir()
		writeRoutes(t, repo, webRoutes)
		err := AddRoute(repo, nr)
		if exitcode.Code(err) != exitcode.BadArgs {
			t.Errorf("%+v: want BadArgs, got %v", nr, err)
		}
		if b, _ := os.ReadFile(RoutesPath(repo)); string(b) != webRoutes {
			t.Errorf("file changed on rejected add")
		}
	}
}

func TestAddRoute_CreatesFile(t *testing.T) {
	repo := t.TempDir()
	if err := AddRoute(repo, Route{Name: "a", Path: "/a"}); err != nil {
		t.Fatal(err)
	}
	rs, err := LoadRoutes(repo)
	if err != nil || len(rs) != 1 || rs[0].Name != "a" {
		t.Fatalf("got %v, %v", rs, err)
	}
}

func TestBaselineFiles(t *testing.T) {
	got := BaselineFiles(Route{Name: "a", Path: "/a"})
	if !reflect.DeepEqual(got, []string{".znf/visual/__snapshots__/a-chromium-linux.png"}) {
		t.Errorf("got %v", got)
	}
	got = BaselineFiles(Route{Name: "a", Path: "/a", Aria: "#c"})
	want := []string{".znf/visual/__snapshots__/a-chromium-linux.png", ".znf/visual/__snapshots__/a.aria.yml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}
