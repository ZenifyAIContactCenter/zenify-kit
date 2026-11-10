package trackstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxPROpen caps how many open PRs one stats run asks gh about.
const maxPROpen = 10

// PRStatus is the review state of one open pull request. CI is success, failure, pending or -;
// Review is approved, changes_requested, review_required or -. An empty CreatedAt marks a PR
// whose status is unknown: gh was not asked (or failed) and nothing was cached.
type PRStatus struct {
	URL       string `json:"url"`
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	CreatedAt string `json:"createdAt"`
	CI        string `json:"ci"`
	Review    string `json:"review"`
	Threads   int    `json:"threads"`
	Ready     bool   `json:"ready"`
}

// PRStatusFunc returns the review state of the pull request at url.
type PRStatusFunc func(url string) (PRStatus, error)

const prStatusQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){createdAt reviewDecision mergeStateStatus commits(last:1){nodes{commit{statusCheckRollup{state}}}} reviewThreads(first:100){nodes{isResolved}}}}}`

var prURLRe = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/]+)/pull/(\d+)`)

func prStatusURL(url string) (host, owner, name string, number int, err error) {
	m := prURLRe.FindStringSubmatch(url)
	if m == nil {
		return "", "", "", 0, fmt.Errorf("not a pull request url: %s", url)
	}
	number, err = strconv.Atoi(m[4])
	if err != nil {
		return "", "", "", 0, fmt.Errorf("pull request number in %s: %w", url, err)
	}
	return m[1], m[2], m[3], number, nil
}

// GhPRStatus asks GitHub (through gh api graphql) for the review state of a pull request.
func GhPRStatus(url string) (PRStatus, error) {
	host, owner, name, number, err := prStatusURL(url)
	if err != nil {
		return PRStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	// owner and name use -f (always a string): -F would turn an all-digit name into an Int.
	cmd := exec.CommandContext(ctx, "gh", "api", "--hostname", host, "graphql", "-f", "query="+prStatusQuery, "-f", "owner="+owner, "-f", "name="+name, "-F", "number="+strconv.Itoa(number)) //nolint:gosec // G204 -- fixed binary, url parts passed as single arguments
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return PRStatus{}, fmt.Errorf("gh api graphql %s: %w after %s", url, ErrGhTimeout, ghTimeout)
	}
	if err != nil {
		return PRStatus{}, fmt.Errorf("gh api graphql %s: %w: %s", url, err, strings.TrimSpace(stderr.String()))
	}
	return parsePRStatus(url, out)
}

func parsePRStatus(url string, out []byte) (PRStatus, error) {
	var v struct {
		Data struct {
			Repository struct {
				PullRequest *struct {
					CreatedAt        string  `json:"createdAt"`
					ReviewDecision   *string `json:"reviewDecision"`
					MergeStateStatus string  `json:"mergeStateStatus"`
					Commits          struct {
						Nodes []struct {
							Commit struct {
								StatusCheckRollup *struct {
									State string `json:"state"`
								} `json:"statusCheckRollup"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool `json:"isResolved"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return PRStatus{}, fmt.Errorf("parse gh output: %w", err)
	}
	pr := v.Data.Repository.PullRequest
	if pr == nil {
		return PRStatus{}, fmt.Errorf("pull request not found: %s", url)
	}
	_, _, name, number, err := prStatusURL(url)
	if err != nil {
		return PRStatus{}, err
	}
	st := PRStatus{URL: url, Repo: name, Number: number, CreatedAt: pr.CreatedAt, CI: "-", Review: "-"}
	if len(pr.Commits.Nodes) > 0 && pr.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		switch pr.Commits.Nodes[0].Commit.StatusCheckRollup.State {
		case "SUCCESS":
			st.CI = "success"
		case "FAILURE", "ERROR":
			st.CI = "failure"
		case "PENDING", "EXPECTED":
			st.CI = "pending"
		}
	}
	if pr.ReviewDecision != nil {
		switch *pr.ReviewDecision {
		case "APPROVED":
			st.Review = "approved"
		case "CHANGES_REQUESTED":
			st.Review = "changes_requested"
		case "REVIEW_REQUIRED":
			st.Review = "review_required"
		}
	}
	for _, t := range pr.ReviewThreads.Nodes {
		if !t.IsResolved {
			st.Threads++
		}
	}
	st.Ready = pr.MergeStateStatus == "CLEAN" && st.Threads == 0
	return st, nil
}

// openPRStatus lists the status of up to maxPROpen open PRs: tracked PRs not in the merged cache,
// newest first end first, then plan-only PRs. It asks in.PRStatus when set, and reads
// in.PRStatusCache otherwise (or when gh fails for a PR), and lists a PR in neither with its
// status unknown; the first gh error lands in res.
// ghDown (gh missing or hanging in this run) skips gh entirely, and a missing or hanging gh here
// is returned so later gh calls in the same run are skipped too: the pane gives stats 20s.
func openPRStatus(in Input, tracked []string, firstEnd map[string]time.Time, plan []string, merged map[string]string, ghDown bool, res *Result) ([]PRStatus, bool) {
	open := []PRStatus{}
	ordered := append([]string(nil), tracked...)
	sortNewestFirst(ordered, firstEnd)
	seen := map[string]bool{}
	var cands []string
	for _, u := range append(ordered, plan...) {
		if _, done := merged[u]; done || seen[u] {
			continue
		}
		seen[u] = true
		cands = append(cands, u)
	}
	if len(cands) > maxPROpen {
		cands = cands[:maxPROpen]
	}
	down := ghDown
	for _, u := range cands {
		if in.PRStatus != nil && !down {
			st, err := in.PRStatus(u)
			if err == nil {
				open = append(open, st)
				continue
			}
			if res.Today.PRStatusError == "" {
				res.Today.PRStatusError = fmt.Sprintf("%s: %v", u, err)
			}
			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, ErrGhTimeout) {
				down = true
			}
		}
		if st, ok := in.PRStatusCache[u]; ok {
			open = append(open, st)
		} else if _, _, name, number, err := prStatusURL(u); err == nil {
			// Neither asked nor cached: still listed, its status unknown (no CreatedAt).
			open = append(open, PRStatus{URL: u, Repo: name, Number: number, CI: "-", Review: "-"})
		}
	}
	return open, down
}
