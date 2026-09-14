package tui

import (
	"errors"
	"fmt"
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
		SelectFn: func(_ string, opts []huh.Option[string]) (string, error) {
			seen = keys(opts)
			return opts[0].Value, nil
		},
		InputFn: func(_, _ string, _ func(string) error) (string, error) { return "", nil },
	})
	if err != nil || res.Workspace != "/ws/A" {
		t.Fatalf("%+v %v", res, err)
	}
	if len(seen) != 2 || seen[0] != "existing" || seen[1] != "cwd" {
		t.Fatalf("options = %v", seen)
	}
}

func TestRunWhere_DefaultFailsValidation(t *testing.T) {
	wantErr := errors.New("thư mục không rỗng")
	res, err := RunWhere(WhereConfig{
		Cwd: "/tmp", OSDefault: "/x",
		Validate: func(d string) error {
			if d == "/x" {
				return wantErr
			}
			return nil
		},
		SelectFn: func(string, []huh.Option[string]) (string, error) { return "default", nil },
		InputFn:  func(_, _ string, _ func(string) error) (string, error) { return "", nil },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if IsAborted(err) {
		t.Fatalf("a validation refusal must not be reported as aborted: %v", err)
	}
	if res != (WhereResult{}) {
		t.Fatalf("result must be zero on error: %+v", res)
	}
}

func TestIsAborted(t *testing.T) {
	if !IsAborted(huh.ErrUserAborted) {
		t.Fatal("huh.ErrUserAborted must be aborted")
	}
	if !IsAborted(fmt.Errorf("wrapped: %w", huh.ErrUserAborted)) {
		t.Fatal("a wrapped huh.ErrUserAborted must still be aborted")
	}
	if IsAborted(errors.New("x")) {
		t.Fatal("a plain error must not be aborted")
	}
}

func TestRunWhere_CustomEmptyRejected(t *testing.T) {
	// The custom-path input's placeholder is cfg.OSDefault (non-empty), so the
	// "optional" short-circuit huhInput applies for placeholder=="" must NOT
	// apply here — an empty/whitespace-only value must be refused before it
	// ever reaches Validate (which would otherwise see "" and might pass).
	res, err := RunWhere(WhereConfig{
		Cwd: "/tmp", OSDefault: "/x",
		Validate: func(string) error { return nil }, // would wrongly accept "" if reached
		SelectFn: func(string, []huh.Option[string]) (string, error) { return "custom", nil },
		InputFn: func(_, placeholder string, validate func(string) error) (string, error) {
			if placeholder != "/x" {
				return "", nil // the sources-dir prompt: optional, leave it alone
			}
			return "", validate("   ") // whitespace-only custom path
		},
	})
	if err == nil {
		t.Fatal("empty custom path must be rejected, got nil error")
	}
	if IsAborted(err) {
		t.Fatalf("an empty-path refusal must not be reported as aborted: %v", err)
	}
	if res != (WhereResult{}) {
		t.Fatalf("result must be zero on error: %+v", res)
	}
}

func TestRequireOrOptional(t *testing.T) {
	called := false
	validate := func(s string) error { called = true; return nil }

	if err := requireOrOptional(true, validate)(""); err != nil {
		t.Fatalf("optional empty must pass: %v", err)
	}
	if called {
		t.Fatal("validate must not run on an accepted empty optional value")
	}
	if err := requireOrOptional(false, validate)("  "); err == nil {
		t.Fatal("required empty (whitespace-only) must be refused")
	}
	if called {
		t.Fatal("validate must not run on a refused empty required value")
	}
	if err := requireOrOptional(false, validate)(" /x "); err != nil {
		t.Fatalf("non-empty must delegate to validate: %v", err)
	}
	if !called {
		t.Fatal("validate must run for a non-empty value")
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
