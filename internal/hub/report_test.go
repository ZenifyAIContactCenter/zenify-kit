package hub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugFromToolInput(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"command":"git -C repos/x/.worktrees/demo-hub/ status"}`, "demo-hub"},
		{`{"edits":[{"file_path":"/ws/repos/be/.worktrees/redis-scan/src/a.js"}]}`, "redis-scan"},
		{`{"file_path":"/ws/docs/a.md"}`, ""},
	}
	for _, c := range cases {
		if got := SlugFromToolInput(json.RawMessage(c.in)); got != c.want {
			t.Errorf("%s: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestApplyEvent(t *testing.T) {
	post := HookInput{
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     json.RawMessage(`{"command":"git -C repos/x/.worktrees/demo-hub/ status"}`),
	}
	next, rm := ApplyEvent(nil, post, "%3", 10)
	if rm || next.State != Working || next.Slug != "demo-hub" || next.Pane != "%3" {
		t.Fatalf("PostToolUse: %+v remove=%v", next, rm)
	}

	cases := []struct {
		name   string
		prev   *Agent
		in     HookInput
		want   AgentState
		remove bool
	}{
		{"permission prompt", &Agent{State: Working}, HookInput{HookEventName: "Notification", NotificationType: "permission_prompt"}, Blocked, false},
		{"idle prompt keeps working", &Agent{State: Working}, HookInput{HookEventName: "Notification", NotificationType: "idle_prompt"}, Working, false},
		{"stop", &Agent{State: Working}, HookInput{HookEventName: "Stop"}, Done, false},
		{"session start", nil, HookInput{HookEventName: "SessionStart"}, Idle, false},
		{"session end", &Agent{State: Idle}, HookInput{HookEventName: "SessionEnd"}, "", true},
	}
	for _, c := range cases {
		c.in.SessionID = "s1"
		n, rm := ApplyEvent(c.prev, c.in, "%1", 20)
		if rm != c.remove {
			t.Errorf("%s: remove=%v", c.name, rm)
			continue
		}
		if !rm && n.State != c.want {
			t.Errorf("%s: state %q want %q", c.name, n.State, c.want)
		}
	}

	// slug kept when new event has none; title set once from the prompt.
	n, _ := ApplyEvent(&Agent{Slug: "keep"}, HookInput{HookEventName: "UserPromptSubmit", Prompt: "hello"}, "%1", 30)
	if n.Slug != "keep" || n.Title != "hello" || n.State != Working {
		t.Errorf("prompt: %+v", n)
	}
	n, _ = ApplyEvent(&Agent{Title: "first"}, HookInput{HookEventName: "UserPromptSubmit", Prompt: "second"}, "%1", 31)
	if n.Title != "first" {
		t.Errorf("title overwritten: %q", n.Title)
	}
}

func TestReport_BadStdin(t *testing.T) {
	home := t.TempDir()
	if err := Report(home, strings.NewReader("not json"), "%1", 1); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(AgentsDir(home)); err == nil {
		entries, _ := os.ReadDir(AgentsDir(home))
		if len(entries) != 0 {
			t.Fatalf("files created: %v", entries)
		}
	}
}

func TestReport_NoTmux(t *testing.T) {
	home := t.TempDir()
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(AgentsDir(home)); err == nil {
		entries, _ := os.ReadDir(AgentsDir(home))
		if len(entries) != 0 {
			t.Fatalf("files created: %v", entries)
		}
	}
}

func TestReport_WritesAndRemoves(t *testing.T) {
	home := t.TempDir()
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", 5); err != nil {
		t.Fatal(err)
	}
	a, err := readAgent(home, "s")
	if err != nil || a == nil || a.State != Done || a.Pane != "%2" || a.UpdatedAt != 5 {
		t.Fatalf("agent: %+v %v", a, err)
	}
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"SessionEnd"}`), "%2", 6); err != nil {
		t.Fatal(err)
	}
	if a, _ := readAgent(home, "s"); a != nil {
		t.Fatalf("not removed: %+v", a)
	}
	if err := Report(home, strings.NewReader(`{"session_id":"a/b","hook_event_name":"Stop"}`), "%2", 6); err == nil {
		t.Fatal("want error for slash id")
	}
}

func TestLogError(t *testing.T) {
	home := t.TempDir()
	LogError(home, errors.New("boom"), 0)
	b, err := os.ReadFile(filepath.Join(Dir(home), "hub.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "1970-01-01T00:00:00Z boom\n" {
		t.Errorf("log line: %q", b)
	}
}
