package ghx

import (
	"errors"
	"testing"
)

// fakeRunner returns canned output/err per call; records args.
type fakeRunner struct {
	out  []byte
	err  error
	args []string
	// respond, when set, overrides out/err per call.
	respond func(args []string) ([]byte, error)
}

func (f *fakeRunner) Run(args ...string) ([]byte, error) {
	f.args = args
	if f.respond != nil {
		return f.respond(args)
	}
	return f.out, f.err
}

func TestCheckAuth(t *testing.T) {
	out := []byte(`github.com
  ✓ Logged in to github.com account natepxn (keyring)
  - Active account: true
  - Git operations protocol: ssh
  - Token: gho_************************************
  - Token scopes: 'admin:public_key', 'gist', 'read:org', 'repo'
`)
	a, err := CheckAuth(&fakeRunner{out: out})
	if err != nil {
		t.Fatalf("CheckAuth: %v", err)
	}
	if !a.LoggedIn || a.Account != "natepxn" || a.Protocol != "ssh" {
		t.Errorf("Auth = %+v", a)
	}
	if !a.HasScopes("read:org", "repo") {
		t.Errorf("HasScopes false, scopes=%v", a.Scopes)
	}
	if a.HasScopes("delete_repo") {
		t.Errorf("HasScopes should be false for delete_repo")
	}
	// token must never leak into any field
	for _, s := range a.Scopes {
		if len(s) > 3 && s[:3] == "gho" {
			t.Errorf("token leaked into scopes: %q", s)
		}
	}
}

func TestCheckAuthNotLoggedIn(t *testing.T) {
	f := &fakeRunner{out: []byte("You are not logged into any GitHub hosts.\n"), err: errors.New("exit 1")}
	a, err := CheckAuth(f)
	if err != nil {
		t.Fatalf("CheckAuth should not error on logged-out: %v", err)
	}
	if a.LoggedIn {
		t.Errorf("LoggedIn should be false")
	}
}

func TestListRepos(t *testing.T) {
	out := []byte(`[{"isArchived":false,"name":"zenify-kit","sshUrl":"git@github.com:ZenifyAIContactCenter/zenify-kit.git","viewerPermission":"ADMIN"},{"isArchived":true,"name":"old","sshUrl":"u","viewerPermission":"NONE"}]`)
	f := &fakeRunner{out: out}
	repos, err := ListRepos(f, "ZenifyAIContactCenter")
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 2 || repos[0].Name != "zenify-kit" || !repos[0].HasAccess() {
		t.Errorf("repos = %+v", repos)
	}
	if repos[1].HasAccess() {
		t.Errorf("NONE permission should be no-access")
	}
	// verify command shape
	wantArgs := []string{"repo", "list", "ZenifyAIContactCenter", "--json", "name,viewerPermission,sshUrl,isArchived", "--limit", "1000"}
	if len(f.args) != len(wantArgs) {
		t.Fatalf("args = %v", f.args)
	}
	for i := range wantArgs {
		if f.args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q want %q", i, f.args[i], wantArgs[i])
		}
	}
}

const activeAuthOut = `github.com
  ✓ Logged in to github.com account phuthuycoding (keyring)
  - Active account: true
  - Git operations protocol: ssh
  - Token: <redacted>
  - Token scopes: 'admin:public_key', 'gist', 'project', 'read:org', 'read:packages', 'repo'
`

func TestCheckActiveAuth_Args(t *testing.T) {
	f := &fakeRunner{out: []byte(activeAuthOut)}
	a, err := CheckActiveAuth(f, "github.com")
	if err != nil {
		t.Fatalf("CheckActiveAuth: %v", err)
	}
	want := []string{"auth", "status", "--active", "-h", "github.com"}
	if len(f.args) != len(want) {
		t.Fatalf("args = %v", f.args)
	}
	for i := range want {
		if f.args[i] != want[i] {
			t.Errorf("args[%d] = %q want %q", i, f.args[i], want[i])
		}
	}
	if !a.LoggedIn || !a.HasScopes("read:packages") {
		t.Errorf("Auth = %+v", a)
	}
}

func TestCheckActiveAuth_FallbackOnOldGh(t *testing.T) {
	f := &fakeRunner{respond: func(args []string) ([]byte, error) {
		for _, x := range args {
			if x == "--active" {
				return []byte("unknown flag: --active\n"), errors.New("exit 1")
			}
		}
		return []byte(activeAuthOut), nil
	}}
	a, err := CheckActiveAuth(f, "github.com")
	if err != nil {
		t.Fatalf("CheckActiveAuth: %v", err)
	}
	if !a.LoggedIn || !a.HasScopes("read:packages") {
		t.Errorf("Auth = %+v", a)
	}
}
