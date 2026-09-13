package tui

import (
	"errors"
	"testing"

	"github.com/charmbracelet/huh"
)

func keys(opts []huh.Option[string]) []string {
	var out []string
	for _, o := range opts {
		out = append(out, o.Value)
	}
	return out
}

func TestRunWhere_NoPointer_HomeCwdHidesCurrentDir(t *testing.T) {
	var seen []string
	res, err := RunWhere(WhereConfig{
		Cwd: "/Users/u", OSDefault: "/Users/u/Developer/zenify",
		Validate: func(d string) error {
			if d == "/Users/u" {
				return errors.New("home")
			}
			return nil
		},
		SelectFn: func(_ string, opts []huh.Option[string]) (string, error) { seen = keys(opts); return "default", nil },
		InputFn:  func(_, _ string, _ func(string) error) (string, error) { return "", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range seen {
		if k == "cwd" {
			t.Fatalf("cwd option must be hidden when invalid: %v", seen)
		}
	}
	if res.Workspace != "/Users/u/Developer/zenify" || res.SourcesDir != "" {
		t.Fatalf("%+v", res)
	}
}

func TestRunWhere_PointerDefaultsToExisting(t *testing.T) {
	var seen []string
	res, err := RunWhere(WhereConfig{
		Cwd: "/tmp/B", Existing: "/ws/A", OSDefault: "/x",
		Validate: func(string) error { return nil },
		SelectFn: func(_ string, opts []huh.Option[string]) (string, error) { seen = keys(opts); return opts[0].Value, nil },
		InputFn:  func(_, _ string, _ func(string) error) (string, error) { return "", nil },
	})
	if err != nil || res.Workspace != "/ws/A" {
		t.Fatalf("%+v %v", res, err)
	}
	if len(seen) != 2 || seen[0] != "existing" || seen[1] != "cwd" {
		t.Fatalf("options = %v", seen)
	}
}

func TestRunWhere_CustomPathAndSources(t *testing.T) {
	inputs := []string{"/data/ws", "/data/projects"}
	res, err := RunWhere(WhereConfig{
		Cwd: "/tmp", OSDefault: "/x",
		Validate: func(string) error { return nil },
		SelectFn: func(string, []huh.Option[string]) (string, error) { return "custom", nil },
		InputFn: func(_, _ string, _ func(string) error) (string, error) {
			v := inputs[0]
			inputs = inputs[1:]
			return v, nil
		},
	})
	if err != nil || res.Workspace != "/data/ws" || res.SourcesDir != "/data/projects" {
		t.Fatalf("%+v %v", res, err)
	}
}
