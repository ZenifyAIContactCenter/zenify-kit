package trackstats

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// MergedFunc returns the merge time (RFC 3339) of the pull request at url, or
// "" when it is not merged yet.
type MergedFunc func(url string) (mergedAt string, err error)

// GhMerged asks the gh CLI for the merge time of a pull request.
func GhMerged(url string) (string, error) {
	cmd := exec.Command("gh", "pr", "view", url, "--json", "mergedAt") //nolint:gosec // G204 -- fixed binary, url is a log value passed as one argument
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
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
