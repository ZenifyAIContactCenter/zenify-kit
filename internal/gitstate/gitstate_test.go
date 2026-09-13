package gitstate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// mkRepo creates <ws>/repos/<name> on branch <branch> with one commit.
func mkRepo(t *testing.T, ws, name, branch string) string {
	t.Helper()
	dir := filepath.Join(ws, "repos", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "-q", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func dirty(t *testing.T, repo string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "wip.txt"), []byte("wip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScope_ContainerFindsReposNotHome(t *testing.T) {
	ws := t.TempDir()
	a := mkRepo(t, ws, "a", "staging")
	b := mkRepo(t, ws, "b", "main")
	// node_modules and Library are skipped by NAME, not merely by the depth
	// cap: a repo two levels inside each is well short of scanDepth==3, so
	// only the name rule could be excluding it (verified by temporarily
	// removing that rule and confirming this assertion fails).
	for _, dir := range []string{
		filepath.Join(ws, "node_modules", "x"),
		filepath.Join(ws, "Library", "x"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "init", "-q", "-b", "main")
	}
	got := Scope(ws)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("Scope = %v, want [%s %s]", got, a, b)
	}
	// inside a repo (any subdirectory of it): just that repo
	if err := os.MkdirAll(filepath.Join(a, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// `git rev-parse --show-toplevel` resolves symlinks (e.g. macOS /var ->
	// /private/var), so compare against the resolved form of `a` rather than
	// its literal string — same physical directory, different spelling.
	wantA, err := filepath.EvalSymlinks(a)
	if err != nil {
		t.Fatal(err)
	}
	if got := Scope(filepath.Join(a, "sub")); len(got) != 1 || got[0] != wantA {
		t.Fatalf("Scope(inside) = %v, want [%s]", got, wantA)
	}
	// $HOME is refused
	t.Setenv("HOME", ws)
	if got := Scope(ws); got != nil {
		t.Fatalf("Scope(HOME) = %v, want nil", got)
	}
}

func TestScope_CapsAt40(t *testing.T) {
	ws := t.TempDir()
	for i := 0; i < 41; i++ {
		mkRepo(t, ws, fmt.Sprintf("r%02d", i), "main")
	}
	if got := Scope(ws); len(got) != 40 {
		t.Fatalf("len = %d, want 40", len(got))
	}
}

func TestDeployPatterns_UnionAndBaseline(t *testing.T) {
	ws := t.TempDir()
	repo := mkRepo(t, ws, "a", "main")
	if got := DeployPatterns(repo, ws); strings.Join(got, ",") != "main,master,production,staging,develop" {
		t.Fatalf("baseline = %v", got)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".claude", "deploy-branches"), []byte("# ws tier\nstaging\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "deploy-branches"), []byte("release*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := DeployPatterns(repo, ws)
	if strings.Join(got, ",") != "release*,staging" {
		t.Fatalf("union = %v, want [release* staging]", got)
	}
	if !IsDeploy("release84", got) || !IsDeploy("staging", got) || IsDeploy("namph/feat/x", got) {
		t.Fatal("IsDeploy glob mismatch")
	}
}

func TestRun_SessionAndStop(t *testing.T) {
	ws := t.TempDir()
	a := mkRepo(t, ws, "a", "staging")
	dirty(t, a)
	mkRepo(t, ws, "b", "namph/feat/x")

	out := Run(ws, ws, Session)
	for _, want := range []string{
		"<git-state>", "2 repo(s) in scope.", "Uncommitted work in flight:",
		"  a [staging] 1 file(s)  ⚠ EDITING ON A DEPLOY BRANCH",
		"Clean but parked on a task branch: b",
		"Rule #8:", "</git-state>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("session output missing %q\n%s", want, out)
		}
	}
	if strings.Count(out, "⚠ EDITING ON A DEPLOY BRANCH") != 1 {
		t.Errorf("want exactly one alarm\n%s", out)
	}

	stop := Run(ws, ws, Stop)
	if !strings.Contains(stop, "a[staging]") || !strings.Contains(stop, "DEPLOY") {
		t.Errorf("stop output missing alarm: %q", stop)
	}
	// only b (clean): stop mode says nothing
	if got := Run(filepath.Join(ws, "repos", "b"), ws, Stop); got != "" {
		t.Errorf("stop on clean repo = %q, want empty", got)
	}
}

func TestRun_Clean(t *testing.T) {
	ws := t.TempDir()
	mkRepo(t, ws, "a", "staging")
	out := Run(ws, ws, Session)
	if !strings.Contains(out, "1 repo(s) in scope.") || !strings.Contains(out, "All clean, all on their base branch") {
		t.Errorf("clean output wrong:\n%s", out)
	}
	if strings.Contains(out, "Uncommitted") {
		t.Errorf("clean output must be exceptions-only:\n%s", out)
	}
}

func TestInspect_BehindLocalBase(t *testing.T) {
	ws := t.TempDir()
	repo := mkRepo(t, ws, "a", "staging")
	// simulate origin/staging one commit ahead of local staging
	run(t, repo, "update-ref", "refs/remotes/origin/staging", "HEAD")
	run(t, repo, "checkout", "-q", "-b", "tmp")
	if err := os.WriteFile(filepath.Join(repo, "f2"), []byte("y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-q", "-m", "ahead")
	run(t, repo, "update-ref", "refs/remotes/origin/staging", "HEAD")
	run(t, repo, "checkout", "-q", "staging")
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "worktree.json"), []byte(`{"baseRef":"origin/staging"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st := Inspect(repo, Session)
	if st.BaseRef != "origin/staging" || st.Behind != 1 {
		t.Fatalf("Inspect = %+v, want BaseRef origin/staging Behind 1", st)
	}
	if Inspect(repo, Stop).Behind != 0 {
		t.Fatal("Stop mode must skip the behind check")
	}
}
