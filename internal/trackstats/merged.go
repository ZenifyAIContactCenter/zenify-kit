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
	out, err := ghPRView(bin, timeout, url, "mergedAt,state")
	if err != nil {
		return "", err
	}
	return parseMergedAt(out)
}

// GhPRTitle asks the gh CLI for the title of a pull request.
func GhPRTitle(url string) (string, error) {
	out, err := ghPRView("gh", ghTimeout, url, "title")
	if err != nil {
		return "", err
	}
	return parsePRTitle(out)
}

// ghPRView runs gh pr view <url> --json <fields> and returns its stdout.
func ghPRView(bin string, timeout time.Duration, url, fields string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "pr", "view", url, "--json", fields) //nolint:gosec // G204 -- bin is fixed by the callers, url is a log value passed as one argument
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("gh pr view %s: %w after %s", url, ErrGhTimeout, timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("gh pr view %s: %w: %s", url, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func parsePRTitle(out []byte) (string, error) {
	var v struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("parse gh output: %w", err)
	}
	return v.Title, nil
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
