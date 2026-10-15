package gitguard

import (
	"reflect"
	"testing"
)

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
		want []string // subcommands, in order
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
		{"alias resolved", "git -c alias.p=push p origin staging", []string{"p", "push"}},
		{"alias not used", "git -c alias.p=push status", []string{"status"}},
		{"shell alias kept", "git -c alias.p='!echo' p", []string{"p"}},
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
	for _, cmd := range []string{"cat f; cd /tmp", "git push origin x && cd ..", "echo $(cd /tmp)"} {
		if got := LeadingCd(cmd); got != "" {
			t.Errorf("LeadingCd(%q) = %q, want empty (cd not first)", cmd, got)
		}
	}
	for _, cmd := range []string{"cd /a && cd /b && git push", "cd /a; git push", "cd /a || exit 1; git push"} {
		if got := LeadingCd(cmd); got != "/a" {
			t.Errorf("LeadingCd(%q) = %q, want /a", cmd, got)
		}
	}
}

func TestParseNestedAndWrappers(t *testing.T) {
	cases := []struct {
		cmd     string
		wantGit []string
	}{
		{`bash -c 'git push origin staging'`, []string{"push"}},
		{`sh -lc "git commit -m x"`, []string{"commit"}},
		{`/bin/zsh -c 'git merge x'`, []string{"merge"}},
		{`eval "git push origin staging"`, []string{"push"}},
		{`eval git push origin staging`, []string{"push"}},
		{`timeout 60 git push --force origin x`, []string{"push"}},
		{`timeout -s KILL 60 git push origin x`, []string{"push"}},
		{`nohup nice -n 5 git push -f origin x`, []string{"push"}},
		{`nice -10 git push origin x`, []string{"push"}},
		{`nohup timeout 60 git push --force origin x`, []string{"push"}},
		{`sudo -E git push origin x`, []string{"push"}},
		{`echo main | xargs -n 1 git push origin`, []string{"push"}},
		{`time git commit -m x`, []string{"commit"}},
		{`command gh pr merge 1`, nil},
		{`exec git push origin staging`, []string{"push"}},
		{`env -i A=1 git push origin x`, []string{"push"}},
		{`env -u HOME -- git push origin x`, []string{"push"}},
		{`git "push" --force origin x`, []string{"push"}},
		{`git 'pu'sh origin x`, []string{"push"}},
		{`git pu\sh origin x`, []string{"push"}},
		{`sh -c "docker compose up"`, nil},
		{`bash -c "$CMD"`, nil},
		{`bash -o pipefail -c 'git push origin staging'`, []string{"push"}},
		{`bash -euo pipefail -c 'git push origin staging'`, []string{"push"}},
		{`bash -c "bash -c \"git push --force origin x\""`, []string{"push"}},
		{`bash -c 'sh -c "bash -c \"git push origin staging\""'`, []string{"push"}},
	}
	for _, c := range cases {
		if got := subs(ParseGitCalls(c.cmd)); len(got)+len(c.wantGit) > 0 && !reflect.DeepEqual(got, c.wantGit) {
			t.Errorf("%s: got %v want %v", c.cmd, got, c.wantGit)
		}
	}
}

func TestParseArgvAfterWrapper(t *testing.T) {
	p := Parse(`command gh pr merge 1`)
	if len(p.Cmds) != 1 {
		t.Fatalf("got %d cmds, want 1: %+v", len(p.Cmds), p.Cmds)
	}
	if want := []string{"gh", "pr", "merge", "1"}; !reflect.DeepEqual(p.Cmds[0].Argv, want) {
		t.Fatalf("Argv = %q, want %q", p.Cmds[0].Argv, want)
	}
	if want := []string{"gh", "pr", "merge", "1"}; !reflect.DeepEqual(p.Cmds[0].Src, want) {
		t.Fatalf("Src = %q, want %q", p.Cmds[0].Src, want)
	}
}

func TestParseWordValues(t *testing.T) {
	p := Parse(`git commit -m "$(date)" 'a b' "x\"y" a\ b "p$HOME"`)
	if len(p.Cmds) != 2 {
		t.Fatalf("got %d cmds, want 2 (git + date): %+v", len(p.Cmds), p.Cmds)
	}
	want := []string{"git", "commit", "-m", "", "a b", `x"y`, "a b", ""}
	if !reflect.DeepEqual(p.Cmds[0].Argv, want) {
		t.Fatalf("Argv = %q, want %q", p.Cmds[0].Argv, want)
	}
	if got, w := p.Cmds[0].Src[3], "$(date)"; got != w {
		t.Fatalf("Src[3] = %q, want %q", got, w)
	}
}

func TestParseUnparseable(t *testing.T) {
	want := map[string]bool{
		`git push --force origin x "`: true,
		`echo "abc`:                   true,
		`echo ok`:                     false,
		`bash -c 'sh -c "bash -c \"git status\""'`:        false,
		`bash -c "bash -c \"git push --force origin x\""`: false,
		`bash -c 'bash -c "bash -c \"bash -c git\""'`:     true,
		`bash -c 'echo "abc'`:                             true,
		`git -c alias.fp='push --force' fp`:               true,
		`git -c alias.m=merge m x`:                        true,
		`git -C /repo -c alias.fp='push --force' fp`:      true,
		`git --git-dir x -c alias.fp=push fp`:             true,
		`git -c alias.st=status st`:                       false,
		`git -c color.ui=never status`:                    false,
		`bash -c "git push origin $B"`:                    true,
		`eval "gh pr merge $N"`:                           true,
		`sh -c "go test $X"`:                              true,
		`echo hi | sh`:                                    true,
		"bash <<'EOF'\ngit status\nEOF":                   true,
		`sh -s arg`:                                       true,
		`bash script.sh`:                                  false,
		`bash -o pipefail script.sh`:                      false,
		`bash "$S"`:                                       false,
	}
	for cmd, w := range want {
		if got := Parse(cmd).Unparseable; got != w {
			t.Errorf("%s: Unparseable = %v, want %v", cmd, got, w)
		}
	}
}

func TestParsePRDExpansion(t *testing.T) {
	want := map[string]bool{
		`echo $REDIS_URL_PRD`:             true,
		`echo ${MONGO_URL_PRD:-x}`:        true,
		`echo "url=$MONGO_URL_PRD"`:       true,
		`bash -c 'echo ${MONGO_URL_PRD}'`: true,
		`X=$MONGO_URL_PRD mongosh`:        true,
		`git grep MONGO_URL_PRD`:          false,
		`echo $MONGO_URL`:                 false,
		`echo '$MONGO_URL_PRD'`:           false,
	}
	for cmd, w := range want {
		got := false
		for _, c := range Parse(cmd).Cmds {
			got = got || c.PRDExpansion
		}
		if got != w {
			t.Errorf("%s: PRDExpansion = %v, want %v", cmd, got, w)
		}
	}
}

func TestParseSrcStripsQuotes(t *testing.T) {
	p := Parse(`cat .zenify/".env"`)
	if len(p.Cmds) != 1 || len(p.Cmds[0].Argv) != 2 {
		t.Fatalf("unexpected cmds: %+v", p.Cmds)
	}
	if got := p.Cmds[0].Argv[1]; got != ".zenify/.env" {
		t.Fatalf("Argv[1] = %q, want .zenify/.env", got)
	}
	if got := p.Cmds[0].Src[1]; got != ".zenify/.env" {
		t.Fatalf("Src[1] = %q, want .zenify/.env", got)
	}
}

func TestParsedPRDExpansion(t *testing.T) {
	want := map[string]bool{
		`URL=$X_PRD; mongosh "$URL"`: true,
		`export U=$X_PRD`:            true,
		`cat <<< "$X_PRD"`:           true,
		`bash -c 'U=$X_PRD; echo x'`: true,
		`echo $MONGO_URL_PRD`:        true,
		`echo '$X_PRD'`:              false,
		`git grep MONGO_URL_PRD`:     false,
	}
	for cmd, w := range want {
		if got := Parse(cmd).PRDExpansion; got != w {
			t.Errorf("%s: Parsed.PRDExpansion = %v, want %v", cmd, got, w)
		}
	}
}
