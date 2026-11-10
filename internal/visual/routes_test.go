package visual

import (
	"errors"
	"os"
	"os/exec"
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

func TestLoadRoutes_Strict(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"null", `null`, "array"},
		{"trailing garbage", `[] garbage`, "after"},
		{"second array", `[{"name":"a","path":"/a"}] [1]`, "after"},
		{"wrong case key", `[{"Name":"a","Path":"/a"}]`, "routes[0]"},
		{"wrong type", `[{"name":"a","path":"/a"},{"name":5,"path":"/b"}]`, "routes[1].name"},
		{"mask wrong type", `[{"name":"a","path":"/a","mask":"x"}]`, "routes[0].mask"},
		{"element not object", `[{"name":"a","path":"/a"}, null]`, "routes[1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := t.TempDir()
			writeRoutes(t, repo, c.body)
			_, err := LoadRoutes(repo)
			if err == nil || exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want BadArgs containing %q, got %v", c.want, err)
			}
		})
	}
	t.Run("trailing whitespace ok", func(t *testing.T) {
		repo := t.TempDir()
		writeRoutes(t, repo, "[]\n\n")
		if _, err := LoadRoutes(repo); err != nil {
			t.Fatal(err)
		}
	})
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
		if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "routes[0]") || !strings.Contains(err.Error(), "area") {
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

func TestAddRoute_MissingRepo(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "typo")
	err := AddRoute(repo, Route{Name: "a", Path: "/a"})
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
	}
	if _, serr := os.Stat(repo); serr == nil {
		t.Error("repo dir must not be created")
	}
}

func TestAddRoute_StatErrorNotCreate(t *testing.T) {
	repo := t.TempDir()
	// .znf/visual is a file, so stat of routes.json fails with ENOTDIR, not ErrNotExist.
	touch(t, repo, ".znf/visual")
	if err := AddRoute(repo, Route{Name: "a", Path: "/a"}); err == nil {
		t.Fatal("want error")
	}
}

func TestAddRoute_NoHTMLEscapeNoLeftovers(t *testing.T) {
	repo := t.TempDir()
	writeRoutes(t, repo, `[{"name":"a","path":"/a","mask":["div > p"]}]`)
	if err := AddRoute(repo, Route{Name: "b", Path: "/b", Mask: []string{"a & b"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(RoutesPath(repo))
	if strings.Contains(string(b), `\u00`) || !strings.Contains(string(b), "div > p") || !strings.Contains(string(b), "a & b") {
		t.Errorf("escaped: %s", b)
	}
	if !strings.HasSuffix(string(b), "]\n") || strings.HasSuffix(string(b), "\n\n") {
		t.Errorf("want one trailing newline: %q", b)
	}
	es, _ := os.ReadDir(filepath.Dir(RoutesPath(repo)))
	if len(es) != 1 {
		t.Errorf("leftover files: %v", es)
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

func touch(t *testing.T, repo string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUntrackedBaselines(t *testing.T) {
	const rj = ".znf/visual/routes.json"
	const png = ".znf/visual/__snapshots__/a-chromium-linux.png"
	const aria = ".znf/visual/__snapshots__/a.aria.yml"
	listing := func(out string, err error) GitRunner {
		return func(string, ...string) (string, error) { return out, err }
	}
	t.Run("image not listed", func(t *testing.T) {
		repo := t.TempDir()
		touch(t, repo, rj, png)
		got, checked := UntrackedBaselines(listing(rj+"\n", nil), repo, []Route{{Name: "a", Path: "/a"}})
		if !checked || !reflect.DeepEqual(got, []string{png}) {
			t.Errorf("got %v %v", got, checked)
		}
	})
	t.Run("aria not listed", func(t *testing.T) {
		repo := t.TempDir()
		touch(t, repo, rj, png, aria)
		got, _ := UntrackedBaselines(listing(rj+"\n"+png+"\n", nil), repo, []Route{{Name: "a", Path: "/a", Aria: "#c"}})
		if !reflect.DeepEqual(got, []string{aria}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("route without baseline on disk", func(t *testing.T) {
		repo := t.TempDir()
		touch(t, repo, rj)
		got, checked := UntrackedBaselines(listing(rj+"\n", nil), repo, []Route{{Name: "a", Path: "/a"}})
		if !checked || len(got) != 0 {
			t.Errorf("got %v %v", got, checked)
		}
	})
	t.Run("routes.json untracked", func(t *testing.T) {
		repo := t.TempDir()
		touch(t, repo, rj, png)
		got, _ := UntrackedBaselines(listing(png+"\n", nil), repo, []Route{{Name: "a", Path: "/a"}})
		if !reflect.DeepEqual(got, []string{rj}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("git error", func(t *testing.T) {
		repo := t.TempDir()
		touch(t, repo, rj)
		got, checked := UntrackedBaselines(listing("", errors.New("not a git repo")), repo, nil)
		if checked || got != nil {
			t.Errorf("got %v %v", got, checked)
		}
	})
	t.Run("no candidates skips git", func(t *testing.T) {
		git := func(string, ...string) (string, error) { t.Fatal("git called"); return "", nil }
		got, checked := UntrackedBaselines(git, t.TempDir(), nil)
		if !checked || got != nil {
			t.Errorf("got %v %v", got, checked)
		}
	})
}

func TestUntrackedBaselines_RealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	git := func(dir string, args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
		return string(out), err
	}
	const rj = ".znf/visual/routes.json"
	const png = ".znf/visual/__snapshots__/a-chromium-linux.png"
	rs := []Route{{Name: "a", Path: "/a"}}
	repo := t.TempDir()
	if _, err := git(repo, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte(".znf/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touch(t, repo, rj, png)
	got, checked := UntrackedBaselines(git, repo, rs)
	if !checked || !reflect.DeepEqual(got, []string{rj, png}) {
		t.Fatalf("before add: %v %v", got, checked)
	}
	if _, err := git(repo, "add", "-f", rj, png); err != nil {
		t.Fatal(err)
	}
	got, checked = UntrackedBaselines(git, repo, rs)
	if !checked || len(got) != 0 {
		t.Fatalf("after staged add: %v %v", got, checked)
	}
}

// Home.png and home.png are one file on a case-insensitive filesystem.
func TestValidateRoutes_NameCaseCollision(t *testing.T) {
	err := ValidateRoutes([]Route{{Name: "home", Path: "/a"}, {Name: "Home", Path: "/b"}})
	if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "routes[1].name") {
		t.Fatalf("err=%v, want BadArgs on routes[1].name", err)
	}
}

func TestAddRoute_KeepsFileMode(t *testing.T) {
	repo := t.TempDir()
	if err := AddRoute(repo, Route{Name: "a", Path: "/a"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(RoutesPath(repo), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AddRoute(repo, Route{Name: "b", Path: "/b"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(RoutesPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode=%v, want 0640", info.Mode().Perm())
	}
}
