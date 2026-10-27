package hub

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// fakeTmux records every tmux invocation and returns err.
type fakeTmux struct {
	err   error
	calls [][]string
}

func (f *fakeTmux) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return nil, f.err
}

// failGit fails every git call; round tests use an empty root so it is never reached.
type failGit struct{}

func (failGit) Run(dir string, args ...string) ([]byte, error) {
	return nil, errors.New("git must not be called")
}

func TestNormalizeOrigin(t *testing.T) {
	for _, url := range []string{
		"git@github.com:ZenifyAIContactCenter/chatting.git",
		"https://github.com/ZenifyAIContactCenter/chatting.git",
	} {
		got, err := normalizeOrigin(url)
		if err != nil || got != "ZenifyAIContactCenter/chatting" {
			t.Errorf("%s: got %q, %v", url, got, err)
		}
	}
	for _, bad := range []string{"", "/srv/git/chatting", "https://github.com/onlyowner"} {
		if got, err := normalizeOrigin(bad); err == nil {
			t.Errorf("%q: want error, got %q", bad, got)
		}
	}
}

func TestAssemble(t *testing.T) {
	tasks := []Task{{Slug: "redis-scan-storm", Repos: []TaskRepo{{
		Repo: "ZenifyAIContactCenter/contact-center-be", Name: "contact-center-be",
		Branch: "phuthuycoding/fix/redis-scan-storm", Path: "/ws/repos/contact-center-be/.worktrees/redis-scan-storm",
	}}}}
	prs := []PR{
		{Number: 812, Repo: "ZenifyAIContactCenter/contact-center-be", Branch: "phuthuycoding/fix/redis-scan-storm", MergeState: "CLEAN"},
		{Number: 431, Repo: "ZenifyAIContactCenter/contact-center-hub", Branch: "phuthuycoding/fix/redis-scan-storm"},
	}
	agents := map[string]Agent{
		"A": {SessionID: "A", Slug: "redis-scan-storm", State: Blocked},
		"B": {SessionID: "B", Slug: "gone", State: Working},
	}
	s := Assemble(tasks, prs, agents)

	if len(s.Tasks) != 1 {
		t.Fatalf("tasks: %+v", s.Tasks)
	}
	task := s.Tasks[0]
	if pr := task.Repos[0].PR; pr == nil || pr.Number != 812 || pr.MergeState != "CLEAN" {
		t.Errorf("task PR: %+v", pr)
	}
	if len(s.OtherPRs) != 1 || s.OtherPRs[0].Number != 431 {
		t.Errorf("OtherPRs: %+v", s.OtherPRs)
	}
	if !reflect.DeepEqual(task.Agents, []string{"A"}) || task.State != Blocked {
		t.Errorf("task agents %v state %q", task.Agents, task.State)
	}
	if len(s.Unattached) != 1 || s.Unattached[0].SessionID != "B" {
		t.Errorf("Unattached: %+v", s.Unattached)
	}
	if len(s.Agents) != 2 {
		t.Errorf("Agents map: %+v", s.Agents)
	}
	if tasks[0].Repos[0].PR != nil {
		t.Error("Assemble must not mutate its input tasks")
	}
}

func TestAssemble_NoAgentsIsIdle(t *testing.T) {
	s := Assemble([]Task{{Slug: "x", State: Blocked}}, nil, nil)
	if s.Tasks[0].State != Idle {
		t.Errorf("state %q, want idle", s.Tasks[0].State)
	}
}

func TestWriteState_Atomic(t *testing.T) {
	home := t.TempDir()
	in := State{OtherPRs: []PR{{Number: 431, Repo: "o/hub"}}, LocalAt: 7, Error: "boom"}
	if err := WriteState(home, in); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if err := WriteState(home, in); err != nil { // overwrite path
		t.Fatalf("WriteState again: %v", err)
	}
	entries, err := os.ReadDir(Dir(home))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"state.json"}) {
		t.Errorf("dir entries %v, want only state.json", names)
	}
	got, err := ReadState(home)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, in)
	}
}

func TestReadState_Missing(t *testing.T) {
	s, err := ReadState(t.TempDir())
	if err != nil || !reflect.DeepEqual(s, State{}) {
		t.Errorf("got %+v, %v", s, err)
	}
}

func TestLoadAgents(t *testing.T) {
	home := t.TempDir()
	if got, err := LoadAgents(home); err != nil || len(got) != 0 {
		t.Fatalf("missing dir: %v %v", got, err)
	}
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(AgentsDir(home), name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.json", `{"session_id":"a","state":"working","slug":"x"}`)
	write("bad.json", `{not json`)
	write(".agent-1.tmp", `{"session_id":"tmp"}`)
	got, err := LoadAgents(home)
	if err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("want decode error naming bad.json, got %v", err)
	}
	if len(got) != 1 || got["a"].Slug != "x" || got["a"].State != Working {
		t.Errorf("agents: %+v", got)
	}
}

func newPoller(t *testing.T, gh *fakeGH, tm *fakeTmux, now int64) *Poller {
	t.Helper()
	return &Poller{Home: t.TempDir(), Root: t.TempDir(), Git: failGit{}, GH: gh, Tmux: tm, Now: func() int64 { return now }}
}

func TestRound_GitHubErrorKeepsOld(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	p := newPoller(t, &fakeGH{err: errors.New("gh: HTTP 502")}, &fakeTmux{}, 2000)
	// A real repo gives the local round a workspace owner, so it keeps the PR.
	p.Git = gitx.ExecRunner()
	mkRepo(t, p.Root, "contact-center-hub", "git@github.com:ZenifyAIContactCenter/contact-center-hub.git", "alpha", "u/feat/alpha")
	old := State{OtherPRs: []PR{{Number: 431, Repo: "ZenifyAIContactCenter/contact-center-hub", Checks: "PENDING"}}, GitHubAt: 1000}
	if err := WriteState(p.Home, old); err != nil {
		t.Fatal(err)
	}
	if err := p.Local(); err != nil {
		t.Fatalf("Local: %v", err)
	}
	err := p.GitHub()
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("GitHub error: %v", err)
	}
	got, rerr := ReadState(p.Home)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(got.OtherPRs) != 1 || got.OtherPRs[0].Number != 431 {
		t.Errorf("OtherPRs not kept: %+v", got.OtherPRs)
	}
	if got.GitHubAt != 1000 {
		t.Errorf("GitHubAt %d, want old 1000", got.GitHubAt)
	}
	if !strings.Contains(got.Error, "HTTP 502") || !strings.Contains(got.Error, "1970-01-01T00:33:20Z") {
		t.Errorf("Error %q, want message and time", got.Error)
	}
	if got.LocalAt != 2000 {
		t.Errorf("LocalAt %d, want 2000", got.LocalAt)
	}

	// A clean round clears the error.
	p.GH = &fakeGH{body: []byte(`{"data":{"search":{"nodes":[]}}}`)}
	if err := p.GitHub(); err != nil {
		t.Fatalf("clean GitHub round: %v", err)
	}
	got, rerr = ReadState(p.Home)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got.Error != "" || len(got.OtherPRs) != 0 || got.GitHubAt != 2000 {
		t.Errorf("after clean tick: %+v", got)
	}
}

func TestRound_LocalOnlyKeepsGitHubError(t *testing.T) {
	p := newPoller(t, &fakeGH{err: errors.New("gh: HTTP 502")}, &fakeTmux{}, 2000)
	if err := p.GitHub(); err == nil {
		t.Fatal("want GitHub error")
	}
	read := func() State {
		t.Helper()
		s, err := ReadState(p.Home)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	p.GH = &fakeGH{err: errors.New("must not be called")}
	if err := p.Local(); err != nil {
		t.Fatalf("Local returned %v, want nil (error already reported)", err)
	}
	if got := read(); !strings.Contains(got.Error, "HTTP 502") {
		t.Errorf("local-only round erased GitHub error: %q", got.Error)
	}

	p.GH = &fakeGH{body: []byte(`{"data":{"search":{"nodes":[]}}}`)}
	if err := p.GitHub(); err != nil {
		t.Fatalf("clean GitHub round: %v", err)
	}
	if got := read(); got.Error != "" || got.GitHubError != "" {
		t.Errorf("clean GitHub round kept error: %q / %q", got.Error, got.GitHubError)
	}
}

const failingPRBody = `{"data":{"search":{"nodes":[{"number":812,"url":"u","headRefName":"b",
	"mergeStateStatus":"BLOCKED","repository":{"nameWithOwner":"ZenifyAIContactCenter/contact-center-be"},
	"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[]}}}}]},
	"reviewThreads":{"nodes":[]}}]}}}`

func TestRound_FiresDisplayMessage(t *testing.T) {
	tm := &fakeTmux{}
	p := newPoller(t, &fakeGH{body: []byte(failingPRBody)}, tm, 3000)
	old := State{Owners: []string{"ZenifyAIContactCenter"}, OtherPRs: []PR{{Number: 812, Repo: "ZenifyAIContactCenter/contact-center-be", Branch: "b", Checks: "PENDING"}}}
	if err := WriteState(p.Home, old); err != nil {
		t.Fatal(err)
	}
	if err := p.GitHub(); err != nil {
		t.Fatalf("round: %v", err)
	}
	want := [][]string{{"display-message", "-d", "5000", "-t", "=zenify", "contact-center-be##812: ci-failed"}}
	if !reflect.DeepEqual(tm.calls, want) {
		t.Errorf("tmux calls %q, want %q", tm.calls, want)
	}
	got, err := ReadState(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OtherPRs) != 1 || got.OtherPRs[0].Alert != "ci-failed" || got.OtherPRs[0].AlertAt != 3000 {
		t.Errorf("OtherPRs %+v", got.OtherPRs)
	}

	// Same PR next round: no new alert, no new message.
	if err := p.GitHub(); err != nil {
		t.Fatalf("second round: %v", err)
	}
	if len(tm.calls) != 1 {
		t.Errorf("second round fired again: %q", tm.calls)
	}
}

func TestRound_TmuxErrorGoesToStateError(t *testing.T) {
	tm := &fakeTmux{err: errors.New("no server running")}
	p := newPoller(t, &fakeGH{body: []byte(failingPRBody)}, tm, 3000)
	if err := WriteState(p.Home, State{Owners: []string{"ZenifyAIContactCenter"}, OtherPRs: []PR{{Number: 812, Repo: "ZenifyAIContactCenter/contact-center-be", Checks: "PENDING"}}}); err != nil {
		t.Fatal(err)
	}
	if err := p.GitHub(); err == nil {
		t.Error("want tmux error returned")
	}
	got, err := ReadState(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Error, "no server running") {
		t.Errorf("Error %q", got.Error)
	}
}

func TestRound_LocalOnlySkipsGitHub(t *testing.T) {
	gh := &fakeGH{err: errors.New("must not be called")}
	p := newPoller(t, gh, &fakeTmux{}, 10)
	if err := p.Local(); err != nil {
		t.Fatalf("round: %v", err)
	}
	if len(gh.args) != 0 {
		t.Errorf("gh called: %q", gh.args)
	}
}

// git runs a real git command for fixtures.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil { //nolint:gosec // G204 -- test fixture
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// mkRepo creates root/repos/<name> with one commit, origin, worktree.json and
// a linked worktree at .worktrees/<slug> on branch with wt.slug=slug.
func mkRepo(t *testing.T, root, name, origin, slug, branch string) string {
	t.Helper()
	dir := filepath.Join(root, "repos", name)
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "worktree.json"), []byte(`{"deps":"none"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	if origin != "" {
		git(t, dir, "remote", "add", "origin", origin)
	}
	git(t, dir, "config", "extensions.worktreeConfig", "true")
	wtPath := filepath.Join(dir, ".worktrees", slug)
	git(t, dir, "worktree", "add", "-q", "-b", branch, wtPath)
	git(t, wtPath, "config", "--worktree", "wt.slug", slug)
	return wtPath
}

func TestCollectLocal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	bePath := mkRepo(t, root, "contact-center-be", "git@github.com:ZenifyAIContactCenter/contact-center-be.git", "redis-scan-storm", "u/fix/redis-scan-storm")
	hubPath := mkRepo(t, root, "contact-center-hub", "https://github.com/ZenifyAIContactCenter/contact-center-hub.git", "redis-scan-storm", "u/fix/redis-scan-storm")
	webPath := mkRepo(t, root, "contact-center-web", "", "alpha", "u/feat/alpha") // no origin → error, rows kept without Repo
	// broken worktree.json → this repo is dropped, error reported.
	broken := filepath.Join(root, "repos", "broken")
	if err := os.MkdirAll(filepath.Join(broken, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	git(t, broken, "init", "-q")
	if err := os.WriteFile(filepath.Join(broken, ".claude", "worktree.json"), []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	tasks, owners, err := CollectLocal(root, gitx.ExecRunner())
	if want := []string{"ZenifyAIContactCenter"}; !reflect.DeepEqual(owners, want) {
		t.Errorf("owners = %q, want %q", owners, want)
	}
	if err == nil || !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "contact-center-web") {
		t.Errorf("want errors naming broken and contact-center-web, got %v", err)
	}
	canon := func(p string) string {
		r, e := filepath.EvalSymlinks(p)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	for i := range tasks {
		for j := range tasks[i].Repos {
			tasks[i].Repos[j].Path = canon(tasks[i].Repos[j].Path)
		}
	}
	want := []Task{
		{Slug: "alpha", Repos: []TaskRepo{
			{Repo: "", Name: "contact-center-web", Branch: "u/feat/alpha", Path: canon(webPath)},
		}},
		{Slug: "redis-scan-storm", Repos: []TaskRepo{
			{Repo: "ZenifyAIContactCenter/contact-center-be", Name: "contact-center-be", Branch: "u/fix/redis-scan-storm", Path: canon(bePath)},
			{Repo: "ZenifyAIContactCenter/contact-center-hub", Name: "contact-center-hub", Branch: "u/fix/redis-scan-storm", Path: canon(hubPath)},
		}},
	}
	if !reflect.DeepEqual(tasks, want) {
		t.Errorf("tasks:\n got %+v\nwant %+v", tasks, want)
	}
}

func TestFilterOwners(t *testing.T) {
	prs := []PR{{Number: 1, Repo: "ZenifyAIContactCenter/x"}, {Number: 459, Repo: "someone/moicash"}}
	got := FilterOwners(prs, []string{"zenifyaicontactcenter"})
	if want := []PR{{Number: 1, Repo: "ZenifyAIContactCenter/x"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("FilterOwners = %+v, want %+v", got, want)
	}
	if got := FilterOwners(prs, nil); got != nil {
		t.Errorf("no known owner must drop every PR, got %+v", got)
	}
}

// twoOwnerBody is a PR query answer with one company PR and one personal PR.
const twoOwnerBody = `{"data":{"search":{"nodes":[
	{"number":1,"url":"u1","headRefName":"b1","repository":{"nameWithOwner":"ZenifyAIContactCenter/x"},"commits":{"nodes":[]},"reviewThreads":{"nodes":[]}},
	{"number":459,"url":"u2","headRefName":"b2","repository":{"nameWithOwner":"someone/moicash"},"commits":{"nodes":[]},"reviewThreads":{"nodes":[]}}]}}}`

func TestRound_GitHubKeepsWorkspaceOwnersOnly(t *testing.T) {
	p := newPoller(t, &fakeGH{body: []byte(twoOwnerBody)}, &fakeTmux{}, 2000)
	if err := WriteState(p.Home, State{Owners: []string{"ZenifyAIContactCenter"}}); err != nil {
		t.Fatal(err)
	}
	if err := p.GitHub(); err != nil {
		t.Fatalf("GitHub: %v", err)
	}
	got, err := ReadState(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OtherPRs) != 1 || got.OtherPRs[0].Number != 1 {
		t.Errorf("OtherPRs = %+v, want only #1", got.OtherPRs)
	}
}

// blockingGH answers only after release is closed; started is closed on the
// first call.
type blockingGH struct {
	started, release chan struct{}
	body             []byte
}

func (b *blockingGH) Run(args ...string) ([]byte, error) {
	close(b.started)
	<-b.release
	return b.body, nil
}

func TestRound_LocalNotBlockedByGitHub(t *testing.T) {
	gh := &blockingGH{started: make(chan struct{}), release: make(chan struct{}), body: []byte(failingPRBody)}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	p := newPoller(t, nil, &fakeTmux{}, 2000)
	p.GH = gh
	// A real repo gives the local rounds a workspace owner, so the GitHub
	// round keeps the company PR.
	p.Git = gitx.ExecRunner()
	mkRepo(t, p.Root, "contact-center-be", "git@github.com:ZenifyAIContactCenter/contact-center-be.git", "alpha", "u/feat/alpha")
	if err := writeAgent(p.Home, &Agent{SessionID: "s1", Slug: "gone"}); err != nil {
		t.Fatal(err)
	}
	ghDone := make(chan error, 1)
	go func() { ghDone <- p.GitHub() }()
	<-gh.started

	localDone := make(chan error, 1)
	go func() {
		var errs []error
		for i := 0; i < 2; i++ {
			errs = append(errs, p.Local())
		}
		localDone <- errors.Join(errs...)
	}()
	select {
	case err := <-localDone:
		if err != nil {
			t.Fatalf("Local: %v", err)
		}
	case <-time.After(5 * time.Second):
		close(gh.release)
		t.Fatal("local rounds blocked by the GitHub round in flight")
	}
	read := func() State {
		t.Helper()
		s, err := ReadState(p.Home)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := read(); got.LocalAt != 2000 || len(got.Unattached) != 1 || len(got.OtherPRs) != 0 {
		t.Fatalf("after local rounds: %+v", got)
	}

	close(gh.release)
	if err := <-ghDone; err != nil {
		t.Fatalf("GitHub: %v", err)
	}
	got := read()
	if got.GitHubAt != 2000 || len(got.OtherPRs) != 1 || got.OtherPRs[0].Number != 812 {
		t.Errorf("GitHub round lost its PRs: %+v", got)
	}
	if len(got.Unattached) != 1 || got.Unattached[0].SessionID != "s1" || got.LocalAt != 2000 {
		t.Errorf("GitHub round lost local data: %+v", got)
	}

	if err := p.Local(); err != nil {
		t.Fatalf("Local: %v", err)
	}
	if got := read(); len(got.OtherPRs) != 1 {
		t.Errorf("local round lost GitHub PRs: %+v", got.OtherPRs)
	}
}

func TestCollectLocal_UnmanagedUsesDirName(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	for _, name := range []string{"contact-center-be", "contact-center-web"} {
		wtPath := mkRepo(t, root, name, "git@github.com:ZenifyAIContactCenter/"+name+".git", "zns-campaign-trace", "u/feat/zns-"+name)
		git(t, wtPath, "config", "--worktree", "--unset", "wt.slug") // no wt.slug: wt.List says (unmanaged)
	}
	tasks, _, err := CollectLocal(root, gitx.ExecRunner())
	if err != nil {
		t.Fatalf("CollectLocal: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Slug != "zns-campaign-trace" || len(tasks[0].Repos) != 2 ||
		tasks[0].Repos[0].Name != "contact-center-be" || tasks[0].Repos[1].Name != "contact-center-web" {
		t.Errorf("tasks = %+v, want one task zns-campaign-trace with both repos", tasks)
	}
}

func TestAssemble_BlockedWinsOverWorking(t *testing.T) {
	agents := map[string]Agent{
		"a": {SessionID: "a", Slug: "x", State: Working},
		"b": {SessionID: "b", Slug: "x", State: Blocked},
	}
	s := Assemble([]Task{{Slug: "x"}}, nil, agents)
	if s.Tasks[0].State != Blocked {
		t.Errorf("task state %q, want blocked", s.Tasks[0].State)
	}
}

func TestRound_LocalPartialAdvancesLocalAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	p := newPoller(t, &fakeGH{err: errors.New("must not be called")}, &fakeTmux{}, 2000)
	p.Git = gitx.ExecRunner()
	mkRepo(t, p.Root, "contact-center-be", "git@github.com:ZenifyAIContactCenter/contact-center-be.git", "alpha", "u/feat/alpha")
	mkRepo(t, p.Root, "contact-center-web", "/srv/git/contact-center-web", "beta", "u/feat/beta") // unparseable origin
	if err := WriteState(p.Home, State{LocalAt: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := p.Local(); err == nil || !strings.Contains(err.Error(), "contact-center-web") {
		t.Errorf("want origin error naming contact-center-web, got %v", err)
	}
	got, err := ReadState(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocalAt != 2000 {
		t.Errorf("LocalAt %d, want 2000 (partial results collected)", got.LocalAt)
	}
	if !strings.Contains(got.LocalError, "contact-center-web") || len(got.Tasks) != 2 {
		t.Errorf("LocalError %q tasks %+v", got.LocalError, got.Tasks)
	}
}

// clockGH advances *now when the fetch returns, like a slow network call.
type clockGH struct {
	now  *int64
	body []byte
}

func (c *clockGH) Run(args ...string) ([]byte, error) {
	*c.now = 5000
	return c.body, nil
}

func TestRound_AlertAtAfterFetch(t *testing.T) {
	now := int64(3000)
	p := newPoller(t, nil, &fakeTmux{}, 0)
	p.GH = &clockGH{now: &now, body: []byte(failingPRBody)}
	p.Now = func() int64 { return now }
	old := State{Owners: []string{"ZenifyAIContactCenter"}, OtherPRs: []PR{{Number: 812, Repo: "ZenifyAIContactCenter/contact-center-be", Branch: "b", Checks: "PENDING"}}}
	if err := WriteState(p.Home, old); err != nil {
		t.Fatal(err)
	}
	if err := p.GitHub(); err != nil {
		t.Fatalf("GitHub: %v", err)
	}
	got, err := ReadState(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OtherPRs) != 1 || got.OtherPRs[0].AlertAt != 5000 || got.GitHubAt != 5000 {
		t.Errorf("want AlertAt and GitHubAt stamped after the fetch (5000), got %+v at %d", got.OtherPRs, got.GitHubAt)
	}
}

func TestRound_UnknownOwnersDropsPRs(t *testing.T) {
	p := newPoller(t, &fakeGH{body: []byte(twoOwnerBody)}, &fakeTmux{}, 2000)
	if err := p.GitHub(); err != nil {
		t.Fatalf("GitHub: %v", err)
	}
	got, err := ReadState(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OtherPRs) != 0 {
		t.Errorf("OtherPRs = %+v, want none while owners are unknown", got.OtherPRs)
	}
}
