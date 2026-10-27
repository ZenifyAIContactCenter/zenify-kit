package hub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
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
	if err := Report(home, strings.NewReader("not json"), "%1", tmuxEnv, 1); err == nil {
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
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "", "", 1); err != nil {
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
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", tmuxEnv, 5); err != nil {
		t.Fatal(err)
	}
	a, err := readAgent(home, "s")
	if err != nil || a == nil || a.State != Done || a.Pane != "%2" || a.UpdatedAt != 5 {
		t.Fatalf("agent: %+v %v", a, err)
	}
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"SessionEnd"}`), "%2", tmuxEnv, 6); err != nil {
		t.Fatal(err)
	}
	if a, _ := readAgent(home, "s"); a != nil {
		t.Fatalf("not removed: %+v", a)
	}
	if err := Report(home, strings.NewReader(`{"session_id":"a/b","hook_event_name":"Stop"}`), "%2", tmuxEnv, 6); err == nil {
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

func TestSlugFromToolInput_PathKeysFirst(t *testing.T) {
	in := `{"content":"see /ws/repos/be/.worktrees/bbb/x.go","file_path":"/ws/repos/be/.worktrees/aaa/y.go"}`
	if got := SlugFromToolInput(json.RawMessage(in)); got != "aaa" {
		t.Errorf("got %q, want aaa (file_path wins over content)", got)
	}
}

func TestSlugFromToolInput_Terminators(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"command":"git -C \"/ws/repos/be/.worktrees/foo\" status"}`, "foo"},
		{`{"command":"cd /ws/repos/be/.worktrees/foo; ls"}`, "foo"},
		{`{"command":"(cd /ws/repos/be/.worktrees/foo)"}`, "foo"},
	}
	for _, c := range cases {
		if got := SlugFromToolInput(json.RawMessage(c.in)); got != c.want {
			t.Errorf("%s: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestApplyEvent_TitleStripsControl(t *testing.T) {
	n, _ := ApplyEvent(nil, HookInput{HookEventName: "UserPromptSubmit", Prompt: "a\nb\x1b[31mc"}, "%1", 1)
	if n.Title != "a b [31mc" {
		t.Errorf("title %q, want %q", n.Title, "a b [31mc")
	}
}

// FR-4.2: only compact continues the session; every other source starts idle.
func TestApplyEvent_SessionStartOnlyCompactKeepsState(t *testing.T) {
	n, _ := ApplyEvent(&Agent{State: Working}, HookInput{HookEventName: "SessionStart", Source: "compact"}, "%1", 1)
	if n.State != Working {
		t.Errorf("compact: state %q, want working", n.State)
	}
	for _, src := range []string{"resume", "startup", "clear", "fork"} {
		n, _ := ApplyEvent(&Agent{State: Working}, HookInput{HookEventName: "SessionStart", Source: src}, "%1", 1)
		if n.State != Idle {
			t.Errorf("source %s: state %q, want idle", src, n.State)
		}
	}
	n, _ = ApplyEvent(nil, HookInput{HookEventName: "SessionStart", Source: "compact"}, "%1", 1)
	if n.State != Idle {
		t.Errorf("compact without record: state %q, want idle", n.State)
	}
}

// errReader fails every read.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("pipe broken") }

func TestReport_NoTmuxDrainsStdin(t *testing.T) {
	r := strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`)
	if err := Report(t.TempDir(), r, "", "", 1); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 0 {
		t.Errorf("stdin not drained: %d bytes left", r.Len())
	}
	if err := Report(t.TempDir(), errReader{}, "", "", 1); err == nil || !strings.Contains(err.Error(), "pipe broken") {
		t.Errorf("want drain error, got %v", err)
	}
}

func TestReport_PayloadLimit(t *testing.T) {
	big := `{"session_id":"s","hook_event_name":"Stop","prompt":"` + strings.Repeat("x", 8<<20) + `"}`
	err := Report(t.TempDir(), strings.NewReader(big), "%1", tmuxEnv, 1)
	if err == nil || !strings.Contains(err.Error(), "hook payload over 8 MiB") {
		t.Errorf("want payload limit error, got %v", err)
	}
}

// writeSlugAt writes agent id with slug and an explicit mtime, as a concurrent
// hook report would.
func writeSlugAt(t *testing.T, home, id, slug string, at time.Time) {
	t.Helper()
	p := agentPath(home, id)
	body := `{"session_id":"` + id + `","state":"working","slug":"` + slug + `"}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestReport_ConcurrentReportSlugSurvives(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	writeSlugAt(t, home, "s", "old-task", time.Unix(1000, 0))
	calls := 0
	reportHook = func() {
		calls++
		if calls == 1 { // a report naming a newer worktree lands mid-flight
			writeSlugAt(t, home, "s", "new-task", time.Unix(2000, 0))
		}
	}
	t.Cleanup(func() { reportHook = func() {} })
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", tmuxEnv, 5); err != nil {
		t.Fatalf("Report: %v", err)
	}
	a, err := readAgent(home, "s")
	if err != nil || a == nil || a.Slug != "new-task" || a.State != Done {
		t.Fatalf("agent %+v %v, want slug new-task with the Stop applied", a, err)
	}
	if calls != 2 {
		t.Errorf("hook calls %d, want 2 (one re-apply)", calls)
	}
}

func TestReport_ConcurrentCreateIsReapplied(t *testing.T) {
	home := t.TempDir()
	calls := 0
	reportHook = func() {
		calls++
		if calls == 1 { // the file did not exist at read time; another report creates it
			if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
				t.Fatal(err)
			}
			writeSlugAt(t, home, "s", "new-task", time.Unix(2000, 0))
		}
	}
	t.Cleanup(func() { reportHook = func() {} })
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", tmuxEnv, 5); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if a, err := readAgent(home, "s"); err != nil || a == nil || a.Slug != "new-task" {
		t.Fatalf("agent %+v %v, want slug new-task kept", a, err)
	}
}

func TestReport_GivesUpAfterThreeChanges(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	writeSlugAt(t, home, "s", "old-task", time.Unix(1000, 0))
	calls := 0
	reportHook = func() {
		calls++
		writeSlugAt(t, home, "s", "task-"+string(rune('a'+calls)), time.Unix(int64(2000+calls), 0))
	}
	t.Cleanup(func() { reportHook = func() {} })
	err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", tmuxEnv, 5)
	if err == nil || !strings.Contains(err.Error(), "3 attempts") {
		t.Fatalf("want error after 3 attempts, got %v", err)
	}
	if calls != 3 {
		t.Errorf("hook calls %d, want 3", calls)
	}
	if a, _ := readAgent(home, "s"); a == nil || a.Slug != "task-d" || a.State != Working {
		t.Errorf("agent %+v, want the last concurrent write kept", a)
	}
}

// tmuxEnv is a TMUX value on server pid 4242.
const tmuxEnv = "/tmp/tmux-1000/default,4242,0"

func TestTmuxServer(t *testing.T) {
	cases := map[string]string{
		"/tmp/tmux-1000/default,4242,0": "4242",
		"/tmp/a,b/c,77,3":               "77", // a comma in the socket path
		"/tmp/tmux-1000/default":        "",
		"":                              "",
	}
	for env, want := range cases {
		if got := TmuxServer(env); got != want {
			t.Errorf("TmuxServer(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestReport_RecordsTmuxServer(t *testing.T) {
	home := t.TempDir()
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", tmuxEnv, 5); err != nil {
		t.Fatal(err)
	}
	a, err := readAgent(home, "s")
	if err != nil || a == nil || a.TmuxServer != "4242" || a.Pane != "%2" {
		t.Fatalf("agent: %+v %v", a, err)
	}
}

func TestReport_EmptyTMUXIsNoTmux(t *testing.T) {
	home := t.TempDir()
	if err := Report(home, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop"}`), "%2", "", 5); err != nil {
		t.Fatal(err)
	}
	if a, err := readAgent(home, "s"); a != nil || err != nil {
		t.Fatalf("agent written outside tmux: %+v %v", a, err)
	}
}

func TestReport_DropsOtherAgentOnSamePane(t *testing.T) { // SC-25
	home := t.TempDir()
	for _, a := range []Agent{
		{SessionID: "old", Pane: "%3", TmuxServer: "4242"},   // same server and pane: dropped
		{SessionID: "other", Pane: "%3", TmuxServer: "1111"}, // same pane on another server: kept
		{SessionID: "near", Pane: "%4", TmuxServer: "4242"},  // other pane: kept
		{SessionID: "legacy", Pane: "%3"},                    // no server recorded, same pane: kept (closed)
	} {
		if err := writeAgent(home, &a); err != nil {
			t.Fatal(err)
		}
	}
	if err := Report(home, strings.NewReader(`{"session_id":"new","hook_event_name":"SessionStart","source":"resume"}`), "%3", tmuxEnv, 9); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAgents(home)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for id := range got {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if want := []string{"legacy", "near", "new", "other"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("agents = %v, want %v", ids, want)
	}
}

func TestReport_DropSamePaneKeepsRecordRewrittenMeanwhile(t *testing.T) {
	home := t.TempDir()
	old := Agent{SessionID: "old", Pane: "%3", TmuxServer: "4242", State: Idle}
	if err := writeAgent(home, &old); err != nil {
		t.Fatal(err)
	}
	dropHook = func() { // a hook for "old" lands after the snapshot (rename: new inode)
		dropHook = func() {}
		moved := old
		moved.State = Working
		if err := writeAgent(home, &moved); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { dropHook = func() {} })
	if err := Report(home, strings.NewReader(`{"session_id":"new","hook_event_name":"SessionStart","source":"resume"}`), "%3", tmuxEnv, 9); err != nil {
		t.Fatal(err)
	}
	if a, err := readAgent(home, "old"); err != nil || a == nil || a.State != Working {
		t.Errorf("rewritten record dropped: %+v %v", a, err)
	}
}

func TestReport_DropSamePaneErrorReturned(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(AgentsDir(home), "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Report(home, strings.NewReader(`{"session_id":"new","hook_event_name":"Stop"}`), "%3", tmuxEnv, 9)
	if err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("want bad.json error, got %v", err)
	}
	if a, _ := readAgent(home, "new"); a == nil {
		t.Error("agent not written")
	}
}
