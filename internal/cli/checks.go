package cli

import (
	"fmt"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
)

func gitAuthCheck(gh ghx.Runner) (bool, string) {
	auth, err := ghx.CheckAuth(gh)
	if err != nil {
		return false, "gh not available"
	}
	if !auth.LoggedIn {
		return false, "not logged in — run `gh auth login`"
	}
	if !auth.HasScopes("read:org", "repo") {
		return false, fmt.Sprintf("logged in as %s but missing scope read:org/repo", auth.Account)
	}
	return true, fmt.Sprintf("logged in as %s (%s), scopes ok", auth.Account, auth.Protocol)
}

func githubAccessCheck(gh ghx.Runner, org string) (bool, string) {
	repos, err := ghx.ListRepos(gh, org)
	if err != nil {
		return false, "cannot list repos"
	}
	n := 0
	for _, r := range repos {
		if r.HasAccess() {
			n++
		}
	}
	return true, fmt.Sprintf("%d accessible repos in %s", n, org)
}

func init() {
	RegisterCheck(Check{Name: "git-auth", Run: func() (bool, string) {
		return gitAuthCheck(ghRunner)
	}})
	RegisterCheck(Check{Name: "github-access", Run: func() (bool, string) {
		return githubAccessCheck(ghRunner, "ZenifyAIContactCenter")
	}})
	RegisterCheck(Check{Name: "gh-stack", Run: func() (bool, string) {
		return ghStackStatus(ghRunner)
	}, Fix: func() (bool, string) {
		if ok, detail := ghStackStatus(ghRunner); ok || !strings.Contains(detail, "extension missing") {
			return ok, detail
		}
		if err := installGHStack(ghRunner); err != nil {
			return false, fmt.Sprintf("gh-stack install: %v", err)
		}
		return ghStackStatus(ghRunner)
	}})
}
