package wt

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// RemoteTimeout bounds the network git calls routed through RemoteRunner
// (ls-remote and push for a remote branch; the push in wt new), so an
// unreachable origin cannot hang them. The full-repo `fetch origin` in wt new,
// rm and sweep is NOT bounded by it.
const RemoteTimeout = 20 * time.Second

// remoteOr returns rr when set, else the ordinary runner.
func remoteOr(rr, r gitx.Runner) gitx.Runner {
	if rr != nil {
		return rr
	}
	return r
}

var userinfoRe = regexp.MustCompile(`://[^/@\s]+@`)

// gitErrLine reduces a git failure to one safe line for a warning: the first
// non-empty stderr line (else the error text) with any URL userinfo
// (`scheme://user:pass@`, `://token@`) replaced by `://***@`.
func gitErrLine(err error) string {
	msg := err.Error()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		for _, l := range strings.Split(string(ee.Stderr), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				msg = l
				break
			}
		}
	}
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return userinfoRe.ReplaceAllString(msg, "://***@")
}

// pushRejectedHint adds the reason when origin already holds the branch.
func pushRejectedHint(detail string) string {
	d := strings.ToLower(detail)
	if strings.Contains(d, "non-fast-forward") || strings.Contains(d, "rejected") || strings.Contains(d, "already exists") {
		return detail + " — origin already has this branch with other commits; not overwritten"
	}
	return detail
}

// baselineDeploy are never deleted from origin, whatever deploy-branches says.
var baselineDeploy = map[string]bool{"main": true, "master": true, "staging": true, "develop": true, "production": true}

// remoteBranchExists is true when origin lists refs/heads/<branch>, and also when
// the lookup fails: not knowing counts as "already there", the safe answer.
func remoteBranchExists(rr gitx.Runner, repoRoot, branch string) bool {
	out, err := rr.Run(repoRoot, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return true
	}
	// ls-remote patterns match by suffix; accept only the exact ref name.
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == "refs/heads/"+branch {
			return true
		}
	}
	return false
}
