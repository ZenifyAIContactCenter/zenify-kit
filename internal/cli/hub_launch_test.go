package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/hub"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
)

// recTmux records every tmux call; calls named in failOn return an error,
// others answer out[subcommand]. new-window prints window ids @1, @2, ...
type recTmux struct {
	calls   []string
	failOn  map[string]bool
	out     map[string]string
	windows int
}

func (r *recTmux) Run(args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if len(args) > 2 && args[0] == "-f" { // config file for a server new-session starts
		args = args[2:]
	}
	if r.failOn[args[0]] {
		return nil, errors.New("fail " + args[0])
	}
	switch args[0] {
	case "-V":
		return []byte("tmux 3.4\n"), nil
	case "new-session", "new-window":
		r.windows++
		return []byte(fmt.Sprintf("@%d\n", r.windows)), nil
	}
	return []byte(r.out[args[0]]), nil
}

func withHubEnv(t *testing.T, rt *recTmux, attached *[]string) string {
	t.Helper()
	home := t.TempDir()
	old := hubLauncher
	hubLauncher = hubEnv{
		home:     func() (string, error) { return home, nil },
		lookPath: lookPathWith("tmux"),
		runner:   rt,
		self:     func() (string, error) { return "/Users/John Doe/it's/zenify", nil },
		attach: func(args ...string) error {
			*attached = args
			return nil
		},
	}
	t.Cleanup(func() { hubLauncher = old })
	t.Setenv("ZENIFY_HUB_SOCKET", "hubtest")
	return home
}

// confArg is the -f argument new-session gets for home.
func confArg(home string) string {
	return "-f " + filepath.Join(home, ".zenify", "hub", "tmux.conf") + " "
}

func hubWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".zenify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".zenify", "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runHub(t *testing.T) (string, error) {
	t.Helper()
	cmd := newHubCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	err := cmd.Execute()
	return out.String(), err
}

func TestHubCmd_InsideOtherTmux(t *testing.T) { // I2
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	out, err := runHub(t)
	if err != nil {
		t.Fatalf("want exit 0, got %v", err)
	}
	want := `already inside another tmux — run "zenify hub" from a plain terminal, or attach with: tmux -L hubtest attach -t =zenify` + "\n"
	if out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if attached != nil {
		t.Errorf("must not attach, got %v", attached)
	}
}

func TestHubCmd_InsideHubTmux(t *testing.T) { // I2
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "/tmp/tmux-1000/hubtest,123,0")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	out, err := runHub(t)
	if err != nil {
		t.Fatalf("want exit 0, got %v", err)
	}
	if want := "zenify hub is already open in this tmux — Alt+n opens a new agent\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if attached != nil {
		t.Errorf("must not attach, got %v", attached)
	}
}

func TestHubCmd_NewSession(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	want := []string{
		confArg(home) + "new-session -d -s zenify -P -F #{window_id} -c " + root + " -n agent claude",
		`split-window -d -h -b -l 30% -t @1 -c ` + root + ` '/Users/John Doe/it'\''s/zenify' hub sidebar`,
	}
	for _, w := range want {
		found := false
		for _, c := range rt.calls {
			if c == w {
				found = true
			}
		}
		if !found {
			t.Errorf("calls missing %q in %q", w, rt.calls)
		}
	}
	if got := strings.Join(attached, " "); got != "-L hubtest attach -t =zenify" {
		t.Errorf("attach args = %q", got)
	}
}

func TestHubCmd_ExistingSessionOnlyAttaches(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	for _, c := range rt.calls {
		if strings.HasPrefix(c, "-f ") || strings.HasPrefix(c, "new-session") || strings.HasPrefix(c, "split-window") {
			t.Errorf("unexpected %q for existing session", c)
		}
	}
	b, err := os.ReadFile(filepath.Join(home, ".zenify", "hub", "tmux.conf"))
	if err != nil || !strings.Contains(string(b), "set -g mouse on") {
		t.Errorf("tmux.conf not written every launch: %q, %v", b, err)
	}
	if got := strings.Join(attached, " "); got != "-L hubtest attach -t =zenify" {
		t.Errorf("attach args = %q", got)
	}
}

func TestHubCmd_ExistingSessionSourcesConf(t *testing.T) { // M3
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"source-file": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	if _, err := runHub(t); err != nil {
		t.Fatalf("source-file failure must not fail: %v", err)
	}
	conf := filepath.Join(home, ".zenify", "hub", "tmux.conf")
	want := []string{"-V", "has-session -t =zenify", "source-file " + conf}
	if !reflect.DeepEqual(rt.calls, want) {
		t.Errorf("calls = %q, want %q", rt.calls, want)
	}
	if attached == nil {
		t.Error("must still attach")
	}
	b, err := os.ReadFile(filepath.Join(home, ".zenify", "hub", "hub.log"))
	if err != nil || !strings.Contains(string(b), "fail source-file") {
		t.Errorf("hub.log = %q, %v", b, err)
	}
}

func TestHubCmd_NoWorkspace(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TMUX", "")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "not inside a zenify workspace") {
		t.Fatalf("err = %v", err)
	}
}

func TestHubCmd_SplitFailureKillsSession(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "split-window": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "setting up tmux session failed: fail split-window") {
		t.Fatalf("err = %v", err)
	}
	if last := rt.calls[len(rt.calls)-1]; last != "kill-session -t =zenify" {
		t.Errorf("last call = %q, want kill-session", last)
	}
	if attached != nil {
		t.Errorf("must not attach, got %v", attached)
	}
}

func TestHubCmd_KillFailureJoined(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "split-window": true, "kill-session": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "fail kill-session") {
		t.Fatalf("kill error swallowed: %v", err)
	}
}

func TestHubCmd_AttachErrorWrapped(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{}
	var attached []string
	withHubEnv(t, rt, &attached)
	hubLauncher.attach = func(...string) error { return errors.New("boom") }
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "tmux attach: boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestHubCmd_NewSessionFailureKillsNothing(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "new-session": true}}
	var attached []string
	withHubEnv(t, rt, &attached)
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "fail new-session") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range rt.calls {
		if strings.HasPrefix(c, "kill-session") {
			t.Errorf("killed a session it did not create: %q", rt.calls)
		}
	}
}

const (
	idNew = "0b5e7c1a-9f3d-4c2e-8a6b-1d2e3f4a5b6c"
	idOld = "1c6f8d2b-0a4e-4d3f-9b7c-2e3f4a5b6c7d"
)

// hubSock is a socket path of the hub tmux server.
func hubSock() string { return "/tmp/tmux-1000/" + tmuxx.Socket() }

// writeHubAgent writes an agent file as the hub-report hook would; an agent
// without a socket is recorded on the hub server.
func writeHubAgent(t *testing.T, home string, a hub.Agent) {
	t.Helper()
	if a.TmuxSocket == "" {
		a.TmuxSocket = hubSock()
	}
	dir := hub.AgentsDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, a.SessionID+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// sessionCalls keeps the calls that build the session, dropping probes.
func sessionCalls(calls []string) []string {
	var out []string
	for _, c := range calls {
		for _, p := range []string{"-f", "new-session", "new-window", "split-window", "set-option", "kill-session"} {
			if strings.HasPrefix(c, p+" ") {
				out = append(out, c)
			}
		}
	}
	return out
}

func TestHubCmd_ResumesClosedAgents(t *testing.T) { // SC-24
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	now := time.Now().Unix()
	writeHubAgent(t, home, hub.Agent{SessionID: idOld, Pane: "%1", TmuxServer: "99", UpdatedAt: now - 120})
	writeHubAgent(t, home, hub.Agent{SessionID: idNew, Pane: "%2", TmuxServer: "99", Slug: "be.2:x", UpdatedAt: now - 60})
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	sidebar := `'/Users/John Doe/it'\''s/zenify' hub sidebar`
	want := []string{
		confArg(home) + "new-session -d -s zenify -P -F #{window_id} -c " + root + " -n be-2-x claude --resume " + idNew,
		"split-window -d -h -b -l 30% -t @1 -c " + root + " " + sidebar,
		"new-window -d -t =zenify: -P -F #{window_id} -n agent -c " + root + " claude --resume " + idOld,
		"split-window -d -h -b -l 30% -t @2 -c " + root + " " + sidebar,
	}
	if got := sessionCalls(rt.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := strings.Join(attached, " "); got != "-L hubtest attach -t =zenify" {
		t.Errorf("attach args = %q", got)
	}
}

func TestHubCmd_SkipsOpenAgent(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	// Another tmux session on server 99 still runs idNew in pane %2.
	rt := &recTmux{failOn: map[string]bool{"has-session": true}, out: map[string]string{"display-message": "99\n", "list-panes": "%2 0 @0\n"}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	writeHubAgent(t, home, hub.Agent{SessionID: idNew, Pane: "%2", TmuxServer: "99", UpdatedAt: time.Now().Unix()})
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	if got := sessionCalls(rt.calls)[0]; got != confArg(home)+"new-session -d -s zenify -P -F #{window_id} -c "+root+" -n agent claude" {
		t.Errorf("first call = %q, want a fresh claude", got)
	}
}

func TestHubCmd_SkipsAgentWithFreshResumeLock(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	now := time.Now().Unix()
	writeHubAgent(t, home, hub.Agent{SessionID: idOld, UpdatedAt: now - 120})
	writeHubAgent(t, home, hub.Agent{SessionID: idNew, UpdatedAt: now - 60})
	lock := filepath.Join(hub.AgentsDir(home), idNew+".resuming")
	if err := os.WriteFile(lock, []byte(strconv.FormatInt(now-5, 10)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sessionCalls(rt.calls), "\n")
	if strings.Contains(got, idNew) || !strings.Contains(got, "claude --resume "+idOld) {
		t.Errorf("calls resume the locked agent or miss the other:\n%s", got)
	}
}

func TestHubCmd_ResumeWindowFailureKillsSession(t *testing.T) {
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true, "new-window": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	now := time.Now().Unix()
	writeHubAgent(t, home, hub.Agent{SessionID: idOld, UpdatedAt: now - 120})
	writeHubAgent(t, home, hub.Agent{SessionID: idNew, UpdatedAt: now - 60})
	_, err := runHub(t)
	if err == nil || !strings.Contains(err.Error(), "fail new-window") {
		t.Fatalf("err = %v", err)
	}
	if last := rt.calls[len(rt.calls)-1]; last != "kill-session -t =zenify" {
		t.Errorf("last call = %q, want kill-session", last)
	}
}

func TestResumeCandidates(t *testing.T) {
	const now = 1_000_000_000
	agents := map[string]hub.Agent{}
	add := func(id string, age int64, open bool) {
		agents[id] = hub.Agent{SessionID: id, UpdatedAt: now - age, Open: open, TmuxSocket: hubSock()}
	}
	for i := 0; i < 10; i++ { // ten closed agents, newest a0
		add(fmt.Sprintf("a%d", i), int64(i+1)*60, false)
	}
	add("b0", 30, true)             // open: never resumed
	add("c0", 7*24*3600+1, false)   // older than 7 days
	add("d0", 7*24*3600, false)     // exactly 7 days: kept by age, but over the cap
	add("bad; rm -rf ~", 10, false) // unsafe id: skipped with an error
	got, err := resumeCandidates(agents, now)
	if err == nil || !strings.Contains(err.Error(), `invalid session id "bad; rm -rf ~"`) {
		t.Errorf("err = %v", err)
	}
	var ids []string
	for _, a := range got {
		ids = append(ids, a.SessionID)
	}
	want := []string{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("candidates = %v, want %v", ids, want)
	}
}

func TestResumeCandidates_AgeCutoff(t *testing.T) {
	const now = 1_000_000_000
	agents := map[string]hub.Agent{
		"a1": {SessionID: "a1", UpdatedAt: now - 7*24*3600, TmuxSocket: hubSock()},
		"a2": {SessionID: "a2", UpdatedAt: now - 7*24*3600 - 1, TmuxSocket: hubSock()},
	}
	got, err := resumeCandidates(agents, now)
	if err != nil || len(got) != 1 || got[0].SessionID != "a1" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestHubCmd_SkipsAgentOfOtherTmuxServer(t *testing.T) { // I1
	root := hubWorkspace(t)
	t.Chdir(root)
	t.Setenv("TMUX", "")
	rt := &recTmux{failOn: map[string]bool{"has-session": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	now := time.Now().Unix()
	writeHubAgent(t, home, hub.Agent{SessionID: idNew, Pane: "%2", TmuxServer: "77", TmuxSocket: "/tmp/tmux-1000/default", UpdatedAt: now - 60})
	if _, err := runHub(t); err != nil {
		t.Fatal(err)
	}
	if got := sessionCalls(rt.calls)[0]; got != confArg(home)+"new-session -d -s zenify -P -F #{window_id} -c "+root+" -n agent claude" {
		t.Errorf("first call = %q, want a fresh claude", got)
	}
}

func TestResumeCandidates_OnlyHubServer(t *testing.T) { // I1
	const now = 1_000_000_000
	agents := map[string]hub.Agent{
		idNew: {SessionID: idNew, UpdatedAt: now - 60, TmuxSocket: "/tmp/tmux-1000/default"},
		idOld: {SessionID: idOld, UpdatedAt: now - 120}, // recorded before tmux_socket existed
	}
	got, err := resumeCandidates(agents, now)
	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v; want none", got, err)
	}
}

func TestHubNewAgent_FromWorkspaceCwd(t *testing.T) { // FR-12.2 [+ agent], FR-12.3 Alt+n
	root := hubWorkspace(t)
	rt := &recTmux{}
	if err := hubNewAgent(rt, root, "/bin/zenify"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"has-session -t =zenify",
		"new-window -t =zenify: -P -F #{window_id} -c " + root + " -n agent claude",
		"split-window -d -h -b -l 30% -t @1 -c " + root + " /bin/zenify hub sidebar",
		"switch-client -t @1",
	}
	if !reflect.DeepEqual(rt.calls, want) {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(rt.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestHubNewAgent_FallsBackToSessionPath(t *testing.T) {
	root := hubWorkspace(t)
	rt := &recTmux{out: map[string]string{"display-message": root + "\n"}}
	if err := hubNewAgent(rt, t.TempDir(), "/bin/zenify"); err != nil {
		t.Fatal(err)
	}
	if rt.calls[0] != "display-message -p -t =zenify #{session_path}" {
		t.Errorf("first call = %q", rt.calls[0])
	}
	if want := "new-window -t =zenify: -P -F #{window_id} -c " + root + " -n agent claude"; rt.calls[2] != want {
		t.Errorf("new-window call = %q, want %q", rt.calls[2], want)
	}
}

func TestHubNewAgent_NoWorkspace(t *testing.T) {
	outside := t.TempDir()
	rt := &recTmux{out: map[string]string{"display-message": outside + "\n"}}
	err := hubNewAgent(rt, outside, "/bin/zenify")
	if err == nil || !strings.Contains(err.Error(), "not inside a zenify workspace") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range rt.calls {
		if strings.HasPrefix(c, "new-window") {
			t.Errorf("opened a window outside a workspace: %q", rt.calls)
		}
	}
}

func TestHubNewAgentCmd_ErrorLoggedExitZero(t *testing.T) {
	t.Chdir(t.TempDir())
	rt := &recTmux{failOn: map[string]bool{"display-message": true}}
	var attached []string
	home := withHubEnv(t, rt, &attached)
	cmd := newHubCmd()
	cmd.SetArgs([]string{"new-agent"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("want exit 0, got %v", err)
	}
	b, err := os.ReadFile(filepath.Join(home, ".zenify", "hub", "hub.log"))
	if err != nil || !strings.Contains(string(b), "fail display-message") {
		t.Errorf("hub.log = %q, %v", b, err)
	}
}
