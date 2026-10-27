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
	want := [][]string{{"display-message", "-d", "5000", "-t", "=zenify", "be#812: ci-failed"}}
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
	r := &fakeRunner{}
	if !HasSession(r) {
		t.Error("want true when has-session succeeds")
	}
	if want := [][]string{{"has-session", "-t", "=zenify"}}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v (exact-match target)", r.calls, want)
	}
	if HasSession(&fakeRunner{err: errors.New("no session")}) {
		t.Error("want false when has-session fails")
	}
}

func TestServerPID(t *testing.T) {
	f := &fakeRunner{out: "4242\n"}
	pid, err := ServerPID(f)
	if err != nil || pid != "4242" {
		t.Fatalf("ServerPID = %q, %v", pid, err)
	}
	want := [][]string{{"display-message", "-p", "#{pid}"}}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func TestLivePanes(t *testing.T) {
	f := &fakeRunner{out: "%0 0 @1\n%3 2 @4\n%12 10 @11\n"}
	panes, err := LivePanes(f)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Pane{"%0": {Window: 0, WindowID: "@1"}, "%3": {Window: 2, WindowID: "@4"}, "%12": {Window: 10, WindowID: "@11"}}
	if !reflect.DeepEqual(panes, want) {
		t.Errorf("panes = %v, want %v", panes, want)
	}
	if got := f.calls[0]; !reflect.DeepEqual(got, []string{"list-panes", "-a", "-F", "#{pane_id} #{window_index} #{window_id}"}) {
		t.Errorf("args = %q", got)
	}
}

func TestSocket(t *testing.T) {
	t.Setenv("ZENIFY_HUB_SOCKET", "")
	if got := Socket(); got != "zenify" {
		t.Errorf("Socket() = %q, want zenify", got)
	}
	t.Setenv("ZENIFY_HUB_SOCKET", "hubtest")
	if got := Socket(); got != "hubtest" {
		t.Errorf("Socket() = %q, want hubtest", got)
	}
}

func TestExecArgs_PrependsSocket(t *testing.T) {
	t.Setenv("ZENIFY_HUB_SOCKET", "hubtest")
	got := execArgs([]string{"has-session", "-t", "=zenify"})
	want := []string{"-L", "hubtest", "has-session", "-t", "=zenify"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("execArgs = %q, want %q", got, want)
	}
}

func TestLivePanes_BadLine(t *testing.T) {
	if _, err := LivePanes(&fakeRunner{out: "%1 x\n"}); err == nil {
		t.Error("want parse error")
	}
}

func TestNoServer(t *testing.T) {
	cases := map[string]bool{
		"tmux list-panes: exit status 1: no server running on /tmp/tmux-1000/default":                            true,
		"tmux list-panes: exit status 1: error connecting to /tmp/tmux-1000/default (No such file or directory)": true,
		"tmux list-panes: exit status 1: can't find session":                                                     false,
	}
	for msg, want := range cases {
		if got := NoServer(errors.New(msg)); got != want {
			t.Errorf("NoServer(%q) = %v, want %v", msg, got, want)
		}
	}
	if NoServer(nil) {
		t.Error("NoServer(nil) = true")
	}
}
