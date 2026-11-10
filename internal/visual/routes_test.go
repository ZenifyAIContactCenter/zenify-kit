package visual

import (
	"os"
	"path/filepath"
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
