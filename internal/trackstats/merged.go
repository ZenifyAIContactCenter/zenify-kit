package trackstats

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// MergedFunc returns the merge time (RFC 3339) of the pull request at url, or
// "" when it is not merged yet.
type MergedFunc func(url string) (mergedAt string, err error)

const ghTimeout = 10 * time.Second

// GhMerged asks the gh CLI for the merge time of a pull request.
func GhMerged(url string) (string, error) { return runGh("gh", ghTimeout, url) }

func runGh(bin string, timeout time.Duration, url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "pr", "view", url, "--json", "mergedAt") //nolint:gosec // G204 -- bin is fixed by GhMerged, url is a log value passed as one argument
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("gh pr view %s: timed out after %s", url, timeout)
	}
	if err != nil {
		return "", fmt.Errorf("gh pr view %s: %w: %s", url, err, strings.TrimSpace(stderr.String()))
	}
	return parseMergedAt(out)
}

func parseMergedAt(out []byte) (string, error) {
	var v struct {
		MergedAt *string `json:"mergedAt"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("parse gh output: %w", err)
	}
	if v.MergedAt == nil {
		return "", nil
	}
	return *v.MergedAt, nil
}
