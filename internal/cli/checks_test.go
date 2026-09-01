package cli

import (
	"strings"
	"testing"
)

type stubGH struct{ auth, list []byte }

func (s stubGH) Run(args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "auth" {
		return s.auth, nil
	}
	return s.list, nil
}

func TestGitAuthCheck(t *testing.T) {
	ok, detail := gitAuthCheck(stubGH{auth: []byte("  ✓ Logged in to github.com account natepxn\n  - Token scopes: 'read:org', 'repo'\n")})
	if !ok {
		t.Errorf("expected ok, detail=%q", detail)
	}
	if strings.Contains(detail, "gho_") {
		t.Errorf("token leaked: %q", detail)
	}
	ok2, _ := gitAuthCheck(stubGH{auth: []byte("You are not logged into any GitHub hosts.\n")})
	if ok2 {
		t.Errorf("logged-out should be not-ok")
	}
}

func TestGithubAccessCheck(t *testing.T) {
	ok, detail := githubAccessCheck(stubGH{list: []byte(`[{"name":"a","viewerPermission":"ADMIN"},{"name":"b","viewerPermission":"NONE"}]`)}, "ZenifyAIContactCenter")
	if !ok || !strings.Contains(detail, "1") {
		t.Errorf("access count wrong: ok=%v detail=%q", ok, detail)
	}
}
