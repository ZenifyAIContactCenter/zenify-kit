package wt

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// RemoteTimeout bounds every network git call wt makes (ls-remote, fetch of one
// branch, push), so an unreachable origin cannot hang wt new / rm / sweep.
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
