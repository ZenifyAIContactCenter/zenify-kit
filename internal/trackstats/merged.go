package trackstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// MergedFunc returns the merge time (RFC 3339) of the pull request at url,
// ClosedUnmerged when it was closed without a merge, or "" when it is still open.
type MergedFunc func(url string) (mergedAt string, err error)

const ghTimeout = 10 * time.Second

// ErrGhTimeout wraps a gh call that did not answer within its timeout.
var ErrGhTimeout = errors.New("timed out")

// ClosedUnmerged is cached for a PR closed without a merge, so gh is never asked about it again.
const ClosedUnmerged = "closed"

// GhMerged asks the gh CLI for the merge time of a pull request.
func GhMerged(url string) (string, error) { return runGh("gh", ghTimeout, url) }

func runGh(bin string, timeout time.Duration, url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "pr", "view", url, "--json", "mergedAt,state") //nolint:gosec // G204 -- bin is fixed by GhMerged, url is a log value passed as one argument
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("gh pr view %s: %w after %s", url, ErrGhTimeout, timeout)
	}
	if err != nil {
		return "", fmt.Errorf("gh pr view %s: %w: %s", url, err, strings.TrimSpace(stderr.String()))
	}
	return parseMergedAt(out)
}

func parseMergedAt(out []byte) (string, error) {
	var v struct {
		MergedAt *string `json:"mergedAt"`
		State    string  `json:"state"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("parse gh output: %w", err)
	}
	if v.MergedAt == nil {
		if v.State == "CLOSED" {
			return ClosedUnmerged, nil
		}
		return "", nil
	}
	return *v.MergedAt, nil
}
