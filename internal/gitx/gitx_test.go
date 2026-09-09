package gitx

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// scriptRunner answers by matching the first two git args.
type scriptRunner struct{ resp map[string]string }

func (s scriptRunner) Run(dir string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	for k, v := range s.resp {
		if strings.HasPrefix(key, k) {
			return []byte(v), nil
		}
	}
	return nil, nil
}

func TestNormalizeRemote(t *testing.T) {
	cases := []struct {
		url       string
		insteadOf map[string]string
		want      string
	}{
		{"git@github.com:ZenifyAIContactCenter/contact-center-be.git", nil, "ZenifyAIContactCenter/contact-center-be"},
		{"git@github-zenify:ZenifyAIContactCenter/chatting.git", map[string]string{"git@github-zenify:": "git@github.com:"}, "ZenifyAIContactCenter/chatting"},
		{"https://github.com/ZenifyAIContactCenter/notification.git", nil, "ZenifyAIContactCenter/notification"},
		{"git@github-zenify:ZenifyAIContactCenter/x.git", nil, "ZenifyAIContactCenter/x"}, // alias host stripped even without insteadOf
	}
	for _, c := range cases {
		if got := NormalizeRemote(c.url, c.insteadOf); got != c.want {
			t.Errorf("NormalizeRemote(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestScanNotCloned(t *testing.T) {
	st, err := Scan(scriptRunner{}, t.TempDir())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if st.Cloned {
		t.Errorf("empty dir should be not-cloned")
	}
}

// stubRunner adapts a func to Runner.
type stubRunner func(string, ...string) ([]byte, error)

func (f stubRunner) Run(d string, a ...string) ([]byte, error) { return f(d, a...) }

func TestHasWorktrees(t *testing.T) {
	// 1 worktree (main) → false; more than one → true
	one := stubRunner(func(dir string, args ...string) ([]byte, error) {
		return []byte("worktree /a\nHEAD abc\nbranch refs/heads/main\n"), nil
	})
	got, err := HasWorktrees(one, "/a")
	if err != nil || got {
		t.Fatalf("1 worktree must be false, got=%v err=%v", got, err)
	}
	many := stubRunner(func(dir string, args ...string) ([]byte, error) {
		return []byte("worktree /a\nHEAD abc\n\nworktree /a/.worktrees/x\nHEAD def\n"), nil
	})
	got, err = HasWorktrees(many, "/a")
	if err != nil || !got {
		t.Fatalf("2 worktrees must be true, got=%v err=%v", got, err)
	}
	errRunner := stubRunner(func(dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git failed")
	})
	if _, err := HasWorktrees(errRunner, "/a"); err == nil {
		t.Fatalf("expected error to propagate")
	}
}

// recRunner records the last Run call to assert args, and returns preconfigured out/err.
type recRunner struct {
	out     []byte
	err     error
	gotDir  string
	gotArgs []string
}

func (r *recRunner) Run(dir string, args ...string) ([]byte, error) {
	r.gotDir = dir
	r.gotArgs = args
	return r.out, r.err
}

func TestListWorktrees(t *testing.T) {
	// porcelain: main first, then 2 linked worktrees.
	out := "worktree /ws/repo\nHEAD a\nbranch refs/heads/main\n\n" +
		"worktree /ws/repo/.worktrees/wt1\nHEAD b\nbranch refs/heads/feat\n\n" +
		"worktree /home/u/.herdr/worktrees/repo/wc\nHEAD c\nbranch refs/heads/fix\n"
	got, err := ListWorktrees(&recRunner{out: []byte(out)}, "/ws/repo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"/ws/repo/.worktrees/wt1", "/home/u/.herdr/worktrees/repo/wc"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestListWorktreesNoneBeyondMain(t *testing.T) {
	out := "worktree /ws/repo\nHEAD a\nbranch refs/heads/main\n"
	got, err := ListWorktrees(&recRunner{out: []byte(out)}, "/ws/repo")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v err %v — want empty", got, err)
	}
}

func TestListWorktreesRunnerErr(t *testing.T) {
	_, err := ListWorktrees(&recRunner{err: errStub}, "/ws/repo")
	if err == nil {
		t.Fatal("want the Runner error to propagate")
	}
}

func TestRepairWorktree(t *testing.T) {
	rr := &recRunner{}
	if err := RepairWorktree(rr, "/ws/repos/repo", "/ws/repos/repo/.worktrees/wt1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	wantArgs := []string{"worktree", "repair", "/ws/repos/repo/.worktrees/wt1"}
	if rr.gotDir != "/ws/repos/repo" || !equalStr(rr.gotArgs, wantArgs) {
		t.Fatalf("gotDir=%q gotArgs=%v", rr.gotDir, rr.gotArgs)
	}
}

var errStub = fmt.Errorf("stub error")

func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
