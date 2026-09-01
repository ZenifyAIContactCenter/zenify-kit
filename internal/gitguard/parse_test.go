package gitguard

import "testing"

func subs(calls []GitCall) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.Sub)
	}
	return out
}

func TestParseGitCalls(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want []string // subcommands, theo thứ tự
	}{
		{"plain commit", "git commit -m x", []string{"commit"}},
		{"global flag before sub", "git --no-pager commit -m x", []string{"commit"}},
		{"dashC then sub", "git -C /repo merge origin/x", []string{"merge"}},
		{"env prefix", "env GIT_X=1 git merge feature", []string{"merge"}},
		{"assignment prefix", "GIT_X=1 git merge feature", []string{"merge"}},
		{"cd then git", "cd /repo && git commit -m x", []string{"commit"}},
		{"subshell", "( git commit )", []string{"commit"}},
		{"cmdsubst is visited", "echo $(git push origin staging)", []string{"push"}},
		{"quoted string is NOT a git call", `echo "git push origin main"`, nil},
		{"merge-base not merge", "git merge-base --is-ancestor HEAD HEAD", []string{"merge-base"}},
		{"log with --merges flag", "git log --merges -3", []string{"log"}},
		{"two git calls", "git fetch && git push origin main", []string{"fetch", "push"}},
		{"degenerate bare git", "git", nil},
		{"degenerate git -C only", "git -C", nil},
		{"degenerate repeated -C", "git -C -C -C push", []string{"push"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := subs(ParseGitCalls(tc.cmd))
			if len(got) != len(tc.want) {
				t.Fatalf("cmd %q: got subs %v, want %v", tc.cmd, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("cmd %q: got subs %v, want %v", tc.cmd, got, tc.want)
				}
			}
		})
	}
}

func TestLeadingCd(t *testing.T) {
	if got := LeadingCd("cd /a/b && git push origin main"); got != "/a/b" {
		t.Fatalf("got %q, want /a/b", got)
	}
	if got := LeadingCd("git commit -m x"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
