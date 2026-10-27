package hub

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeClipEnv is a machine with the given env vars and tools on PATH; run
// records each tool call and fails with err.
type fakeClipEnv struct {
	env   map[string]string
	tools map[string]bool
	err   error
	calls [][]string // stdin, then path and args
	out   bytes.Buffer
}

func (f *fakeClipEnv) clip(goos string) clipboard {
	return pickClipboard(clipEnv{
		goos:   goos,
		getenv: func(k string) string { return f.env[k] },
		lookPath: func(name string) (string, error) {
			if f.tools[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		run: func(stdin, name string, args ...string) error {
			f.calls = append(f.calls, append([]string{stdin, name}, args...))
			return f.err
		},
		out:     &f.out,
		timeout: time.Second,
	})
}

func TestPickClipboard(t *testing.T) {
	all := map[string]bool{"wl-copy": true, "xclip": true, "xsel": true, "pbcopy": true}
	cases := []struct {
		name  string
		goos  string
		env   map[string]string
		tools map[string]bool
		want  []string // nil: OSC 52
	}{
		{"wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, all, []string{"u", "/usr/bin/wl-copy"}},
		{"wayland without wl-copy falls to xclip", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, map[string]bool{"xclip": true}, []string{"u", "/usr/bin/xclip", "-selection", "clipboard"}},
		{"x11 xclip", "linux", map[string]string{"DISPLAY": ":0"}, all, []string{"u", "/usr/bin/xclip", "-selection", "clipboard"}},
		{"x11 xsel", "linux", map[string]string{"DISPLAY": ":0"}, map[string]bool{"xsel": true}, []string{"u", "/usr/bin/xsel", "--clipboard", "--input"}},
		{"wl-copy needs WAYLAND_DISPLAY", "linux", map[string]string{}, map[string]bool{"wl-copy": true}, nil},
		{"darwin", "darwin", map[string]string{}, map[string]bool{"pbcopy": true}, []string{"u", "/usr/bin/pbcopy"}},
		{"pbcopy only on darwin", "linux", map[string]string{}, map[string]bool{"pbcopy": true}, nil},
		{"nothing", "windows", map[string]string{}, map[string]bool{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TMUX", "")
			f := &fakeClipEnv{env: c.env, tools: c.tools}
			if err := f.clip(c.goos)("u"); err != nil {
				t.Fatalf("copy: %v", err)
			}
			if c.want == nil {
				if len(f.calls) != 0 || f.out.String() != "\x1b]52;c;dQ==\a" {
					t.Errorf("want OSC 52 only, got calls %q out %q", f.calls, f.out.String())
				}
				return
			}
			if !reflect.DeepEqual(f.calls, [][]string{c.want}) || f.out.Len() != 0 {
				t.Errorf("calls %q out %q, want %q", f.calls, f.out.String(), c.want)
			}
		})
	}
}

func TestPickClipboard_ToolError(t *testing.T) {
	f := &fakeClipEnv{env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, tools: map[string]bool{"wl-copy": true}, err: errors.New("exit status 1")}
	err := f.clip("linux")("u")
	if err == nil || err.Error() != "wl-copy: exit status 1" {
		t.Errorf("err = %v, want %q", err, "wl-copy: exit status 1")
	}
}

func TestKeyY_ClipboardErrorInFooter(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Idle))
	m.clip = func(string) error { return errors.New("wl-copy: exit status 1") }
	m = update(t, m, key("j"))
	m = update(t, m, key("y"))
	lines := strings.Split(m.View(), "\n")
	if got := lines[len(lines)-2]; got != "wl-copy: exit status 1" {
		t.Errorf("footer line = %q, want the clipboard error\n%s", got, m.View())
	}
}

func TestKeyY_HungClipboardToolTimesOut(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	clip := pickClipboard(clipEnv{
		goos:     "linux",
		getenv:   func(k string) string { return map[string]string{"WAYLAND_DISPLAY": "wayland-0"}[k] },
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		run: func(string, string, ...string) error {
			<-block // a tool that never exits
			return nil
		},
		out:     &bytes.Buffer{},
		timeout: 50 * time.Millisecond,
	})
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Idle))
	m.clip = clip
	m = update(t, m, key("j"))
	done := make(chan Model, 1)
	go func() { done <- update(t, m, key("y")) }()
	select {
	case m = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Update blocked on a hung clipboard tool")
	}
	lines := strings.Split(m.View(), "\n")
	if got := lines[len(lines)-2]; got != "wl-copy: timed out after 50ms" {
		t.Errorf("footer line = %q, want the timeout error", got)
	}
}
