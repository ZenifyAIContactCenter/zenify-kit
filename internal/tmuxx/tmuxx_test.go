package tmuxx

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	out   string
	err   error
}

func (f *fakeRunner) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte(f.out), f.err
}

func found(string) (string, error) { return "/usr/bin/tmux", nil }

func TestParseVersion(t *testing.T) {
	cases := []struct {
		out    string
		ver    string
		wantOK bool
		detail string
	}{
		{"tmux 3.4\n", "3.4", true, ""},
		{"tmux 3.3a\n", "3.3a", true, ""},
		{"tmux 3.0a\n", "3.0a", false, "older than 3.1"},
		{"tmux next-3.5\n", "3.5", true, ""},
	}
	for _, c := range cases {
		t.Run(c.out, func(t *testing.T) {
			ver, ok, detail := Available(found, &fakeRunner{out: c.out})
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (detail %q)", ok, c.wantOK, detail)
			}
			if ver != c.ver {
				t.Errorf("version = %q, want %q", ver, c.ver)
			}
			if c.detail != "" && !strings.Contains(detail, c.detail) {
				t.Errorf("detail %q missing %q", detail, c.detail)
			}
		})
	}
}

func TestAvailable_Missing(t *testing.T) {
	lp := func(string) (string, error) { return "", errors.New("not found") }
	_, ok, detail := Available(lp, &fakeRunner{})
	if ok {
		t.Fatal("ok = true, want false")
	}
	for _, want := range []string{"brew install tmux", "sudo apt install tmux"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q missing %q", detail, want)
		}
	}
}

func TestAvailable_RunError(t *testing.T) {
	_, ok, detail := Available(found, &fakeRunner{err: errors.New("boom")})
	if ok || !strings.Contains(detail, "boom") {
		t.Fatalf("ok=%v detail=%q, want false and error surfaced", ok, detail)
	}
}

func TestSendKeys(t *testing.T) {
	r := &fakeRunner{}
	if err := SendKeys(r, "%3", "/znf:fix X"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"send-keys", "-t", "%3", "-l", "/znf:fix X"},
		{"send-keys", "-t", "%3", "Enter"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestSendKeys_StopsOnError(t *testing.T) {
	r := &fakeRunner{err: errors.New("boom")}
	if err := SendKeys(r, "%3", "x"); err == nil {
		t.Fatal("want error")
	}
	if len(r.calls) != 1 {
		t.Errorf("calls = %d, want 1 (no Enter after failed text)", len(r.calls))
	}
}

func TestDisplayMessage(t *testing.T) {
	r := &fakeRunner{}
	if err := DisplayMessage(r, "be#812: ci-failed"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"display-message", "-d", "5000", "-t", "zenify", "be#812: ci-failed"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestSelectPane(t *testing.T) {
	r := &fakeRunner{}
	if err := SelectPane(r, "%2"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"select-window", "-t", "%2"},
		{"select-pane", "-t", "%2"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestHasSession(t *testing.T) {
	if !HasSession(&fakeRunner{}) {
		t.Error("want true when has-session succeeds")
	}
	if HasSession(&fakeRunner{err: errors.New("no session")}) {
		t.Error("want false when has-session fails")
	}
}
