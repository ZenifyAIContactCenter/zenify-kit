package hub

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var pr812 = PR{
	Number: 812,
	Repo:   "acme/api",
	URL:    "https://github.com/acme/api/pull/812",
}

func TestCopyURL(t *testing.T) {
	if got, want := CopyURL(pr812), "https://github.com/acme/api/pull/812"; got != want {
		t.Errorf("CopyURL = %q, want %q", got, want)
	}
}

func TestMergeCommand(t *testing.T) {
	if got, want := MergeCommand(pr812), "gh pr merge 812 --repo acme/api --merge"; got != want {
		t.Errorf("MergeCommand = %q, want %q", got, want)
	}
}

func TestOSC52(t *testing.T) {
	t.Setenv("TMUX", "")
	if got, want := OSC52("ab"), "\x1b]52;c;YWI=\a"; got != want {
		t.Errorf("OSC52 outside tmux = %q, want %q", got, want)
	}
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	if got, want := OSC52("ab"), "\x1bPtmux;\x1b\x1b]52;c;YWI=\a\x1b\\"; got != want {
		t.Errorf("OSC52 inside tmux = %q, want %q", got, want)
	}
}

func TestFixPrompt(t *testing.T) {
	pr := PR{FailedURL: "https://github.com/o/r/actions/runs/1/job/2"}
	if got, want := FixPrompt(pr), "/znf:fix https://github.com/o/r/actions/runs/1/job/2"; got != want {
		t.Errorf("FixPrompt = %q, want %q", got, want)
	}
}

func TestAck_CreatesFile(t *testing.T) {
	home := t.TempDir()
	if err := Ack(home, pr812); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	p := filepath.Join(home, ".zenify", "hub", "acks", "acme_api_812")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("ack file: %v", err)
	}
}

func TestTargetAgent(t *testing.T) {
	s := State{
		Tasks: []Task{{Slug: "t1", Agents: []string{"old", "new"}}},
		Agents: map[string]Agent{
			"old": {SessionID: "old", Pane: "%1", State: Blocked, UpdatedAt: 100},
			"new": {SessionID: "new", Pane: "%2", State: Working, UpdatedAt: 200},
		},
	}
	a, ok := TargetAgent(s, Item{Kind: KindTask, Task: &s.Tasks[0]})
	if !ok || a.SessionID != "old" {
		t.Errorf("TargetAgent = %+v, %v; want blocked agent old", a, ok)
	}
}

func TestTargetAgent_TieNewest(t *testing.T) {
	s := State{
		Tasks: []Task{{Slug: "t1", Agents: []string{"a", "b"}}},
		Agents: map[string]Agent{
			"a": {SessionID: "a", Pane: "%1", State: Working, UpdatedAt: 100},
			"b": {SessionID: "b", Pane: "%2", State: Working, UpdatedAt: 200},
		},
	}
	if a, ok := TargetAgent(s, Item{Kind: KindTask, Task: &s.Tasks[0]}); !ok || a.SessionID != "b" {
		t.Errorf("TargetAgent = %+v, %v; want newest agent b", a, ok)
	}
}

func TestTargetAgent_PrefersOpen(t *testing.T) {
	s := State{
		Tasks: []Task{{Slug: "t1", Agents: []string{"closed", "open"}}},
		Agents: map[string]Agent{
			"closed": {SessionID: "closed", Pane: "%1", State: Blocked, UpdatedAt: 300},
			"open":   {SessionID: "open", Pane: "%2", State: Idle, UpdatedAt: 100, Open: true},
		},
	}
	if a, ok := TargetAgent(s, Item{Kind: KindTask, Task: &s.Tasks[0]}); !ok || a.SessionID != "open" {
		t.Errorf("TargetAgent = %+v, %v; want open agent", a, ok)
	}
}

func TestWindowName_Sanitizes(t *testing.T) {
	cases := map[string]string{"Be_1 x": "be-1-x", "--a--": "a", "...": "agent", "zns#2": "zns-2"}
	for slug, want := range cases {
		if got := WindowName(slug); got != want {
			t.Errorf("WindowName(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestTargetAgent_NoAgent(t *testing.T) {
	s := State{Tasks: []Task{{Slug: "t1"}}}
	if _, ok := TargetAgent(s, Item{Kind: KindTask, Task: &s.Tasks[0]}); ok {
		t.Error("TargetAgent on a task without agents returned ok")
	}
}

func TestFlatten_Sections(t *testing.T) {
	s := State{
		Tasks: []Task{{
			Slug:   "t1",
			Repos:  []TaskRepo{{Name: "be", PR: &pr812}, {Name: "web"}},
			Agents: []string{"a"},
		}},
		Agents:   map[string]Agent{"a": {SessionID: "a", Slug: "t1"}},
		OtherPRs: []PR{{Number: 9, Repo: "o/r"}},
	}
	var kinds []ItemKind
	var titles []string
	for _, it := range Flatten(s, false, nil) {
		kinds = append(kinds, it.Kind)
		if it.Kind == KindHeader {
			titles = append(titles, it.Title)
		}
	}
	want := []ItemKind{KindHeader, KindRepoGroup, KindTask, KindTaskPR, KindRepoGroup, KindTask, KindHeader, KindAgent, KindHeader, KindPR}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
	if got := strings.Join(titles, ","); got != "TASKS,AGENTS,MY PRS" {
		t.Errorf("headers = %q", got)
	}
}

func TestNewAgentWindow(t *testing.T) {
	f := &scriptTmux{out: map[string]string{"new-window": "@7\n"}}
	if err := NewAgentWindow(f, "/ws", "/bin/zenify"); err != nil {
		t.Fatalf("NewAgentWindow: %v", err)
	}
	want := [][]string{
		{"has-session", "-t", "=zenify"},
		{"new-window", "-t", "=zenify:", "-P", "-F", "#{window_id}", "-c", "/ws", "-n", "agent", "claude"},
		{"split-window", "-d", "-h", "-b", "-l", "30%", "-t", "@7", "-c", "/ws", "/bin/zenify", "hub", "sidebar"},
		{"switch-client", "-t", "@7"},
		{"set-option", "-w", "-t", "@7", "allow-passthrough", "on"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q\nwant %q", f.calls, want)
	}
}

// failSetOption answers new-window with a window id and fails set-option.
type failSetOption struct{ calls [][]string }

func (f *failSetOption) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "new-window":
		return []byte("@7\n"), nil
	case "set-option":
		return nil, errors.New("invalid option: allow-passthrough")
	}
	return nil, nil
}

func TestNewAgentWindow_PassthroughErrorIsReported(t *testing.T) {
	f := &failSetOption{}
	err := NewAgentWindow(f, "/ws", "/bin/zenify")
	if err == nil || !strings.Contains(err.Error(), "allow-passthrough") {
		t.Errorf("want allow-passthrough error, got %v", err)
	}
	if len(f.calls) != 5 || f.calls[2][0] != "split-window" || f.calls[3][0] != "switch-client" {
		t.Errorf("window not fully built: %q", f.calls)
	}
}

func TestNewAgentWindow_NoZenifySessionUsesCurrent(t *testing.T) {
	f := &scriptTmux{
		out:  map[string]string{"new-window": "@8\n"},
		fail: map[string]error{"has-session": errors.New("can't find session: zenify")},
	}
	if err := NewAgentWindow(f, "/ws", "/bin/zenify"); err != nil {
		t.Fatalf("NewAgentWindow: %v", err)
	}
	want := [][]string{
		{"has-session", "-t", "=zenify"},
		{"new-window", "-P", "-F", "#{window_id}", "-c", "/ws", "-n", "agent", "claude"},
		{"split-window", "-d", "-h", "-b", "-l", "30%", "-t", "@8", "-c", "/ws", "/bin/zenify", "hub", "sidebar"},
		{"set-option", "-w", "-t", "@8", "allow-passthrough", "on"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q\nwant %q", f.calls, want)
	}
}

func TestResumeAgentWindow_ZenifySession(t *testing.T) {
	f := &scriptTmux{out: map[string]string{"new-window": "@3\n"}}
	id := "0b5e7c1a-9f3d-4c2e-8a6b-1d2e3f4a5b6c"
	if err := ResumeAgentWindow(f, "/ws", "/bin/zenify", id, "t1"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"has-session", "-t", "=zenify"},
		{"new-window", "-t", "=zenify:", "-P", "-F", "#{window_id}", "-c", "/ws", "-n", "t1", "claude", "--resume", id},
		{"split-window", "-d", "-h", "-b", "-l", "30%", "-t", "@3", "-c", "/ws", "/bin/zenify", "hub", "sidebar"},
		{"switch-client", "-t", "@3"},
		{"set-option", "-w", "-t", "@3", "allow-passthrough", "on"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q\nwant %q", f.calls, want)
	}
}

func TestResumeAgentWindow_InvalidSessionID(t *testing.T) {
	f := &scriptTmux{}
	err := ResumeAgentWindow(f, "/ws", "/bin/zenify", "abc; rm -rf ~", "t1")
	if err == nil || !strings.Contains(err.Error(), "invalid session id") {
		t.Errorf("err = %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("tmux called: %q", f.calls)
	}
}

func TestWindowName(t *testing.T) {
	cases := map[string]string{"": "agent", "be-1": "be-1", "v1.2:x": "v1-2-x"}
	for slug, want := range cases {
		if got := WindowName(slug); got != want {
			t.Errorf("WindowName(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestValidSessionID(t *testing.T) {
	for id, want := range map[string]bool{"0b5e7c1a-9f3d-4c2e-8a6b-1d2e3f4a5b6c": true, "ABCdef-01": true, "": false, "s1 x": false, "../x": false, "-": false, "-a1": false} {
		if got := ValidSessionID(id); got != want {
			t.Errorf("ValidSessionID(%q) = %v, want %v", id, got, want)
		}
	}
}
