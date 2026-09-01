package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// initGitRepo makes a real repo with one commit on branch main so worktree add
// has a base ref, plus a worktree.json and a file to seed.
func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	os.WriteFile(filepath.Join(root, ".claude", "worktree.json"),
		[]byte(`{"abbrev":"ccbe","user":"namph","baseRef":"main","portRange":[3200,3249],"deps":"install","copy":["seed.txt"]}`), 0o644)
	os.WriteFile(filepath.Join(root, "seed.txt"), []byte("hi"), 0o644)
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return root
}

func TestWtNew_Integration_CreatesWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("WT_SESSION", "") // guards off for a clean single-shot test
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t)
	t.Setenv("WT_REPO_ROOT", root)

	out, err := runWt(t, root, "new", "my-task", "--type", "feat")
	if err != nil {
		t.Fatalf("wt new failed: %v\n%s", err, out)
	}
	wtPath := filepath.Join(root, ".worktrees", "my-task")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree dir not created: %v", err)
	}
	// seeded copy target present
	if _, err := os.Stat(filepath.Join(wtPath, "seed.txt")); err != nil {
		t.Fatalf("copy target not seeded: %v", err)
	}
	// PORT written into .env within range
	env, _ := os.ReadFile(filepath.Join(wtPath, ".env"))
	if !strings.Contains(string(env), "PORT=32") {
		t.Fatalf(".env missing allocated PORT: %q", env)
	}
	// state records the worktree by slug
	stateRaw, err := os.ReadFile(filepath.Join(root, ".wt", "state.json"))
	if err != nil || !strings.Contains(string(stateRaw), `"my-task"`) {
		t.Fatalf("state.json did not record the worktree: err=%v content=%q", err, stateRaw)
	}
}

func TestWtNew_SeedsIdentityEnv(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t) // worktree.json abbrev=ccbe
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "id-task", "--type", "feat"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".worktrees", "id-task", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "WT_SLUG=id-task") {
		t.Errorf("env missing WT_SLUG; got:\n%s", got)
	}
	if !strings.Contains(got, "COMPOSE_PROJECT_NAME=ccbe-id-task") {
		t.Errorf("env missing COMPOSE_PROJECT_NAME=ccbe-id-task; got:\n%s", got)
	}
}

// initGitRepoWithPortCount is initGitRepo but with a "portCount" key in
// worktree.json, for exercising the monorepo block-allocation path.
func initGitRepoWithPortCount(t *testing.T, portCount int) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	cfg := `{"abbrev":"hub","user":"namph","baseRef":"main","portRange":[3200,3249],"deps":"install","copy":["seed.txt"],"portCount":` + strconv.Itoa(portCount) + `}`
	os.WriteFile(filepath.Join(root, ".claude", "worktree.json"), []byte(cfg), 0o644)
	os.WriteFile(filepath.Join(root, "seed.txt"), []byte("hi"), 0o644)
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return root
}

func TestWtNew_PortCountGreaterThanOne_SeedsBlockAndBase(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepoWithPortCount(t, 3)
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "hub-task", "--type", "feat"); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(root, ".worktrees", "hub-task", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(env)
	if !strings.Contains(got, "WT_PORT_BASE=") {
		t.Fatalf(".env missing WT_PORT_BASE for a portCount=3 worktree; got:\n%s", got)
	}
	stateRaw, err := os.ReadFile(filepath.Join(root, ".wt", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := string(stateRaw)
	if !strings.Contains(state, `"portBase"`) {
		t.Fatalf("state.json missing portBase for a portCount=3 worktree: %q", state)
	}
}

func TestWtNew_PortCountAbsent_NoRegression(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t) // no portCount key → default 1
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "plain-task", "--type", "feat"); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(root, ".worktrees", "plain-task", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(env)
	if strings.Contains(got, "WT_PORT_BASE") {
		t.Fatalf("a portCount=1 (default) worktree must NOT get a WT_PORT_BASE line; got:\n%s", got)
	}
}
