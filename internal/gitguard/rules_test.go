package gitguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkWorkspace makes a temp workspace holding a fake .zenify/.env and returns
// its dir and the Options a hook would build for it.
func mkWorkspace(t *testing.T) (string, Options) {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", ".env"), []byte("MONGO_URL_PRD=fake\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ws, Options{
		ProdHosts: []ProdHost{{"MONGO_URL_PRD", "prod-host.example"}, {"REDIS_URL_PRD", "10.0.0.15"}},
		EnvFile:   filepath.Join(ws, ".zenify/.env"),
	}
}

func noenv(string) string { return "" }

func TestRules(t *testing.T) {
	_, opts := mkWorkspace(t)
	dir := filepath.Join(t.TempDir(), "repo")
	mkRepo(t, dir, "feat/x", []string{"staging"})

	deny := []string{
		`git push --force origin feat/x`, `git push -f origin feat/x`, `git push -uf origin feat/x`,
		`git push --force-with-lease origin feat/x`, `git push origin +feat/x`,
		`timeout 60 git push --force origin x`, `nohup nice -n 5 git push -f origin x`, `git "push" --force origin x`,
		`gh pr merge 1`, `command gh pr merge 1`, `bash -c 'gh pr merge 1'`, `gh api -X PUT repos/o/r/pulls/1/merge`,
		`wt rm x --force`, `zenify wt rm -f x`,
		`echo $REDIS_URL_PRD`, `bash -c 'echo ${MONGO_URL_PRD}'`, `printenv MONGO_URL_PRD`,
		`mongosh mongodb://prod-host.example/db`, `redis-cli -h 10.0.0.15`,
		`gh api -XPUT repos/o/r/pulls/1/merge`, `gh api --method PUT repos/o/r/pulls/1/merge`,
		`gh api --method=PUT repos/o/r/pulls/1/merge`, `gh api -X put repos/o/r/pulls/1/merge`,
		`gh pr merge 1 "`, `curl x/pulls/1/merge "`, `git push origin x -f "`, `echo $MONGO_URL_PRD "`,
		`cat .zenify/.env "`, `ping prod-host.example "`,
		`git push --force origin x "`, `git -c alias.fp='push --force' fp`,
	}
	allow := []string{
		`go test ./...`, `git push origin feat/x`, `git push -u origin feat/x`, `gh pr create --title t`,
		`sh -c "docker compose up"`, `wt rm x`, `wt sweep`, `gh api repos/o/r/pulls/1/merge`,
		`zenify db-read collections`, `zenify env list`, `git grep MONGO_URL_PRD`, `echo $MONGO_URL`,
		`mongosh mongodb://staging-host/db`, `ping 10.0.0.150`, `curl http://staging-prod-host.example.net`,
		`echo "abc`, `curl http://localhost:8080`, `git log --merges`, `rm -f build.log`,
	}
	for _, cmd := range deny {
		if d := Decide(cmd, dir, noenv, nil, opts); !d.Deny {
			t.Errorf("want deny: %s", cmd)
		}
	}
	for _, cmd := range allow {
		if d := Decide(cmd, dir, noenv, nil, opts); d.Deny {
			t.Errorf("want allow: %s → %s", cmd, d.Message)
		}
	}
}

func TestRuleEnvFilePath(t *testing.T) {
	ws, opts := mkWorkspace(t)
	repo := filepath.Join(t.TempDir(), "repo")
	mkRepo(t, repo, "feat/x", []string{"staging"})

	cases := []struct {
		cwd, cmd string
		deny     bool
	}{
		{ws, `cat .zenify/.env`, true},
		{ws, `source .zenify/.env`, true},
		{ws, `cat .zenify/".env"`, true},
		{ws, `mongosh "$(grep MONGO_URL_PRD .zenify/.env)"`, true},
		{ws, `cd .zenify && cat .env`, true},
		{ws, `cd .zenify && source .env`, true},
		{ws, "cat .zenify/.env; cd /tmp", true},
		{ws, "cat .zenify/.env && cd /tmp", true},
		{ws, `chmod 600 .zenify/.env`, false},
		{ws, `git commit -m "$(printf 'docs: ghi chú .zenify/.env')"`, false}, //znf:allow-lang
		{ws, `gh pr create --body "sửa .zenify/.env"`, false},                 //znf:allow-lang
		{repo, `grep -rn ".zenify/.env" internal/`, false},
	}
	for _, tc := range cases {
		if d := Decide(tc.cmd, tc.cwd, noenv, nil, opts); d.Deny != tc.deny {
			t.Errorf("Deny=%v want %v | %s (%s)", d.Deny, tc.deny, tc.cmd, d.Message)
		}
	}
}

func TestDenyMessageNoHost(t *testing.T) {
	_, opts := mkWorkspace(t)
	d := Decide(`mongosh mongodb://prod-host.example/db`, t.TempDir(), noenv, nil, opts)
	if !d.Deny {
		t.Fatal("want deny")
	}
	if !strings.Contains(d.Message, "host của MONGO_URL_PRD") { //znf:allow-lang
		t.Errorf("message lacks key: %q", d.Message)
	}
	// The sub label is literally "prod-host"; the host itself must not leak (SC-6).
	if strings.Contains(d.Message, "prod-host.example") {
		t.Errorf("message leaks host: %q", d.Message)
	}
}

// SC-1 / SC-11: a push or commit wrapped in bash -c / sh -c / eval / exec is
// still judged against the current deploy branch.
func TestDecideNestedOnDeployBranch(t *testing.T) {
	_, opts := mkWorkspace(t)
	dir := filepath.Join(t.TempDir(), "repo")
	mkRepo(t, dir, "staging", []string{"staging"})
	for _, cmd := range []string{
		`bash -c 'git push origin staging'`, `sh -c "git commit -m x"`,
		`eval "git push origin staging"`, `exec git push origin staging`,
	} {
		if d := Decide(cmd, dir, noenv, nil, opts); !d.Deny {
			t.Errorf("want deny: %s", cmd)
		}
	}
}
