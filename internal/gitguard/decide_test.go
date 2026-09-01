package gitguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // G204 -- test helper, args are fixed test literals, not attacker-controlled
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func mkRepo(t *testing.T, dir, branch string, deny []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", branch)
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "f")
	git(t, dir, "commit", "-q", "-m", "init")
	if deny != nil {
		if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o750); err != nil {
			t.Fatal(err)
		}
		body := ""
		for _, b := range deny {
			body += b + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, ".claude", "deploy-branches"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecide(t *testing.T) {
	root := t.TempDir()
	deny := filepath.Join(root, "deny")     // deploy=main
	other := filepath.Join(root, "other")   // development-2, release*, staging
	nodeny := filepath.Join(root, "nodeny") // deploy-branches rỗng → không có nhánh cấm
	mkRepo(t, deny, "main", []string{"main"})
	mkRepo(t, other, "main", []string{"development-2", "release*", "staging"})
	mkRepo(t, nodeny, "main", []string{"# none"})

	getenv := func(string) string { return "" }
	dec := func(cwd, cmd string) bool {
		return Decide(cmd, cwd, getenv, nil).Deny
	}
	cases := []struct {
		want     bool
		cwd, cmd string
	}{
		{true, deny, "git commit -m x"},
		{true, deny, "git merge some-branch"},
		{true, deny, "git push origin main"},
		{false, deny, "git push origin namph/feat/x"},
		{false, deny, "cd " + nodeny + " && git commit -m wf"},
		{true, deny, "cd " + other + " && git push origin development-2"},
		{true, deny, "cd " + other + " && git push origin release80"},
		{false, deny, "cd " + other + " && git push origin namph/feat/x"},
		{true, deny, "cd " + nodeny + " && git push --all"},
		{true, deny, "cd " + nodeny + " && git -C " + other + " push origin staging"},
		{false, deny, "git merge-base --is-ancestor HEAD HEAD"},
		{false, deny, "git merge-tree --write-tree main main"},
		{false, deny, "git commit-tree -h"},
		{false, deny, "git commit-graph verify"},
		{false, deny, "git log --merges -3"},
		{false, deny, "git log --no-merges --oneline"},
		{false, deny, "git branch --contains HEAD"},
		{true, deny, "git -C " + deny + " merge origin/x"},
		{true, deny, "git --no-pager commit -m x"},
		{true, deny, "git merge origin/x"},
		{true, deny, "git merge"},
	}
	for _, tc := range cases {
		if got := dec(tc.cwd, tc.cmd); got != tc.want {
			t.Errorf("Deny=%v want %v | %s", got, tc.want, tc.cmd)
		}
	}
}

func TestDecideWorktreeOwnBranch(t *testing.T) {
	root := t.TempDir()
	deny := filepath.Join(root, "deny")
	mkRepo(t, deny, "main", []string{"main"})
	wt := filepath.Join(root, "deny-wt")
	git(t, deny, "worktree", "add", "-q", wt, "-b", "namph/feat/wt-case")
	if Decide("git commit -m wf", wt, func(string) string { return "" }, nil).Deny {
		t.Fatal("worktree trên feature branch phải allow commit")
	}
}
