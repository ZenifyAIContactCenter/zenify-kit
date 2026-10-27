package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
)

// PR is one open pull request of the current gh user.
type PR struct {
	Number            int    `json:"number"`
	URL               string `json:"url"`
	Repo              string `json:"repo"` // owner/repo
	Branch            string `json:"branch"`
	MergeState        string `json:"merge_state"`
	ReviewDecision    string `json:"review_decision"`
	Checks            string `json:"checks"` // SUCCESS | FAILURE | PENDING | ""
	FailedURL         string `json:"failed_url"`
	Alert             string `json:"alert"` // ci-failed | ready | new-review | ""
	Draft             bool   `json:"draft"`
	UnresolvedThreads int    `json:"unresolved_threads"`
	AlertAt           int64  `json:"alert_at"`
}

const (
	AlertCIFailed  = "ci-failed"
	AlertReady     = "ready"
	AlertNewReview = "new-review"
)

const prQuery = `query { search(query: "is:open is:pr author:@me archived:false", type: ISSUE, first: 100) { nodes { ... on PullRequest { number url isDraft headRefName mergeStateStatus reviewDecision repository { nameWithOwner } commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 50) { nodes { ... on CheckRun { conclusion detailsUrl } ... on StatusContext { state targetUrl } } } } } } } reviewThreads(first: 100) { nodes { isResolved } } } } } }`

// FetchPRs lists the user's open PRs through `gh api graphql`.
func FetchPRs(r ghx.Runner) ([]PR, error) {
	out, err := r.Run("api", "graphql", "-f", "query="+prQuery)
	if err != nil {
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	return parsePRs(out)
}

type prResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		Search struct {
			Nodes []struct {
				Number           int    `json:"number"`
				URL              string `json:"url"`
				IsDraft          bool   `json:"isDraft"`
				HeadRefName      string `json:"headRefName"`
				MergeStateStatus string `json:"mergeStateStatus"`
				ReviewDecision   string `json:"reviewDecision"`
				Repository       struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State    string `json:"state"`
								Contexts struct {
									Nodes []struct {
										Conclusion string `json:"conclusion"` // CheckRun
										DetailsURL string `json:"detailsUrl"`
										State      string `json:"state"` // StatusContext
										TargetURL  string `json:"targetUrl"`
									} `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
				ReviewThreads struct {
					Nodes []struct {
						IsResolved bool `json:"isResolved"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
}

// failedConclusions are the CheckRun conclusions that count as a failed job.
var failedConclusions = map[string]bool{
	"FAILURE": true, "TIMED_OUT": true, "STARTUP_FAILURE": true, "CANCELLED": true, "ACTION_REQUIRED": true,
}

// parsePRs decodes the prQuery response body.
func parsePRs(body []byte) ([]PR, error) {
	var resp prResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode PR query: %w", err)
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, 0, len(resp.Errors))
		for _, e := range resp.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("PR query: %s", strings.Join(msgs, "; "))
	}
	var prs []PR
	for _, n := range resp.Data.Search.Nodes {
		if n.Number == 0 { // not a PullRequest node
			continue
		}
		pr := PR{
			Number: n.Number, URL: n.URL, Repo: n.Repository.NameWithOwner, Branch: n.HeadRefName,
			MergeState: n.MergeStateStatus, ReviewDecision: n.ReviewDecision, Draft: n.IsDraft,
		}
		if len(n.Commits.Nodes) > 0 {
			if roll := n.Commits.Nodes[0].Commit.StatusCheckRollup; roll != nil {
				pr.Checks = checksState(roll.State)
				for _, c := range roll.Contexts.Nodes {
					if failedConclusions[c.Conclusion] && c.DetailsURL != "" {
						pr.FailedURL = c.DetailsURL
						break
					}
					if (c.State == "FAILURE" || c.State == "ERROR") && c.TargetURL != "" {
						pr.FailedURL = c.TargetURL
						break
					}
				}
			}
		}
		for _, th := range n.ReviewThreads.Nodes {
			if !th.IsResolved {
				pr.UnresolvedThreads++
			}
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

// checksState folds GitHub's StatusState (ERROR, EXPECTED, ...) into
// SUCCESS | FAILURE | PENDING.
func checksState(s string) string {
	switch s {
	case "ERROR", "FAILURE":
		return "FAILURE"
	case "EXPECTED", "PENDING":
		return "PENDING"
	}
	return s
}

func prKey(p PR) string { return p.Repo + "#" + strconv.Itoa(p.Number) }

func isReady(p PR) bool { return p.MergeState == "CLEAN" && p.UnresolvedThreads == 0 }

// DetectAlerts compares this round's PRs with the previous round's (matched by
// Repo and Number). A new alert is stamped with now and returned in fired; a PR
// without a new alert keeps its previous Alert/AlertAt. A PR not seen last
// round has nothing to compare against and never fires.
func DetectAlerts(prev, cur []PR, now int64) (out []PR, fired []PR) {
	before := make(map[string]PR, len(prev))
	for _, p := range prev {
		before[prKey(p)] = p
	}
	for _, p := range cur {
		old, seen := before[prKey(p)]
		if !seen {
			out = append(out, p)
			continue
		}
		var alert string
		switch {
		case p.Checks == "FAILURE" && old.Checks != "FAILURE":
			alert = AlertCIFailed
		case p.UnresolvedThreads > old.UnresolvedThreads:
			alert = AlertNewReview
		case isReady(p) && !isReady(old):
			alert = AlertReady
		}
		if alert == "" {
			p.Alert, p.AlertAt = old.Alert, old.AlertAt
		} else {
			p.Alert, p.AlertAt = alert, now
			fired = append(fired, p)
		}
		out = append(out, p)
	}
	return out, fired
}

// AcksDir holds one file per acknowledged PR alert.
func AcksDir(home string) string { return filepath.Join(Dir(home), "acks") }

// ackName is the ack file name for p: <owner>_<repo>_<n>.
func ackName(p PR) string {
	return strings.ReplaceAll(p.Repo, "/", "_") + "_" + strconv.Itoa(p.Number)
}

// DropAcked clears the alert of every PR whose ack file is at least as new as
// AlertAt. It returns the updated copy plus any error statting an ack file
// (such a PR keeps its alert).
func DropAcked(prs []PR, ackDir string) ([]PR, error) {
	out := make([]PR, len(prs))
	copy(out, prs)
	var errs []error
	for i, p := range out {
		if p.Alert == "" {
			continue
		}
		fi, err := os.Stat(filepath.Join(ackDir, ackName(p)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("stat ack for %s: %w", prKey(p), err))
			continue
		}
		if fi.ModTime().Unix() >= p.AlertAt {
			out[i].Alert, out[i].AlertAt = "", 0
		}
	}
	return out, errors.Join(errs...)
}

// GitHubInterval is 20s while any PR has pending checks, 60s otherwise.
func GitHubInterval(s State) time.Duration {
	for _, p := range allPRs(s) {
		if p.Checks == "PENDING" {
			return 20 * time.Second
		}
	}
	return 60 * time.Second
}

// allPRs returns every PR in s: those attached to task repos, then OtherPRs.
func allPRs(s State) []PR {
	var out []PR
	for _, t := range s.Tasks {
		for _, r := range t.Repos {
			if r.PR != nil {
				out = append(out, *r.PR)
			}
		}
	}
	return append(out, s.OtherPRs...)
}
