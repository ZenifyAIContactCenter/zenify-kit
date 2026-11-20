package trackstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ReviewFunc fetches the review data of a merged pull request.
type ReviewFunc func(url string) (ReviewSnapshot, error)

// ReviewSnapshot is the review data of one merged PR, kept for the close-day feedback.
type ReviewSnapshot struct {
	URL         string          `json:"url"`
	TrackID     string          `json:"trackId"` // "-" when no end row names the url
	CreatedAt   string          `json:"createdAt"`
	MergedAt    string          `json:"mergedAt"`
	FetchedAt   string          `json:"fetchedAt"`
	Commits     []ReviewCommit  `json:"commits"`
	BotComments []ReviewComment `json:"botComments"`
	Threads     []ReviewThread  `json:"threads"`
}

// ReviewCommit is one commit of the PR.
type ReviewCommit struct {
	OID           string `json:"oid"`
	CommittedDate string `json:"committedDate"`
}

// ReviewComment is one comment on the PR or in a review thread.
type ReviewComment struct {
	Author     string `json:"author"`
	AuthorType string `json:"authorType"` // GraphQL __typename of the author: "Bot", "User", ...
	CreatedAt  string `json:"createdAt"`
	Body       string `json:"body"`
}

// ReviewThread is one inline review thread.
type ReviewThread struct {
	Path       string          `json:"path"`
	Line       int             `json:"line"`
	IsResolved bool            `json:"isResolved"`
	Comments   []ReviewComment `json:"comments"`
}

const reviewQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){` +
	`pullRequest(number:$number){createdAt mergedAt ` +
	`commits(last:100){nodes{commit{oid committedDate}}} ` +
	`comments(first:100){nodes{author{__typename login} createdAt body}} ` +
	`reviewThreads(first:100){nodes{isResolved path line comments(first:50){nodes{author{__typename login} createdAt body}}}}}}}`

// GhReview asks gh for the review data of the PR at url in one graphql call.
// TrackID and FetchedAt are left for the caller.
func GhReview(prURL string) (ReviewSnapshot, error) {
	owner, name, number, ok := splitPRURL(prURL)
	if !ok {
		return ReviewSnapshot{}, fmt.Errorf("not a pull request url: %s", prURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	// -f keeps owner and name strings even when they look like numbers; -F makes number an Int.
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql", "-f", "query="+reviewQuery, //nolint:gosec // G204 -- fixed binary, values parsed from a log url passed as single arguments
		"-f", "owner="+owner, "-f", "name="+name, "-F", "number="+number)
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err := ghReviewErr(prURL, ctx.Err() != nil, err, stderr.String()); err != nil {
		return ReviewSnapshot{}, err
	}
	s, err := parseReview(out)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	s.URL = prURL
	return s, nil
}

// GhTransportError is a review fetch that failed because gh could not answer at all (timeout,
// no network, bad credentials, rate limit): it says nothing about the PR itself.
type GhTransportError struct{ Err error }

func (e *GhTransportError) Error() string { return e.Err.Error() }
func (e *GhTransportError) Unwrap() error { return e.Err }

// ghTransportMarks are the lowercased gh stderr texts of a transport failure.
var ghTransportMarks = []string{
	"could not resolve host", "bad credentials", "http 401", "requires authentication", "gh auth login",
	"authentication required", "connection refused", "connection reset", "network is unreachable",
	"i/o timeout", "tls handshake timeout", "rate limit", "secondary rate limit", "error connecting to", "no such host",
}

// ghReviewErr is the error of one gh review call, nil when it succeeded. A timeout, or a failure
// whose stderr (never the url) carries a transport mark, is a *GhTransportError.
func ghReviewErr(prURL string, timedOut bool, err error, stderr string) error {
	if timedOut {
		return &GhTransportError{Err: fmt.Errorf("gh api graphql %s: %w after %s", prURL, ErrGhTimeout, ghTimeout)}
	}
	if err == nil {
		return nil
	}
	stderr = strings.TrimSpace(stderr)
	e := fmt.Errorf("gh api graphql %s: %w: %s", prURL, err, stderr)
	// gh could not be started at all: missing binary, bad PATH entry, no permission.
	var execErr *exec.Error
	var pathErr *fs.PathError
	if errors.Is(err, exec.ErrNotFound) || errors.As(err, &execErr) || errors.As(err, &pathErr) {
		return &GhTransportError{Err: e}
	}
	low := strings.ToLower(stderr)
	for _, m := range ghTransportMarks {
		if strings.Contains(low, m) {
			return &GhTransportError{Err: e}
		}
	}
	return e
}

type ghComment struct {
	Author struct {
		Typename string `json:"__typename"`
		Login    string `json:"login"`
	} `json:"author"`
	CreatedAt string `json:"createdAt"`
	Body      string `json:"body"`
}

func toComment(c ghComment) ReviewComment {
	return ReviewComment{Author: c.Author.Login, AuthorType: c.Author.Typename, CreatedAt: c.CreatedAt, Body: c.Body}
}

func parseReview(out []byte) (ReviewSnapshot, error) {
	var v struct {
		Data struct {
			Repository struct {
				PullRequest *struct {
					CreatedAt string `json:"createdAt"`
					MergedAt  string `json:"mergedAt"`
					Commits   struct {
						Nodes []struct {
							Commit ReviewCommit `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
					Comments struct {
						Nodes []ghComment `json:"nodes"`
					} `json:"comments"`
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool   `json:"isResolved"`
							Path       string `json:"path"`
							Line       int    `json:"line"`
							Comments   struct {
								Nodes []ghComment `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return ReviewSnapshot{}, fmt.Errorf("parse gh output: %w", err)
	}
	pr := v.Data.Repository.PullRequest
	if pr == nil {
		return ReviewSnapshot{}, errors.New("parse gh output: no pullRequest in the answer")
	}
	// Empty lists stay [] in the file, never null.
	s := ReviewSnapshot{
		CreatedAt: pr.CreatedAt, MergedAt: pr.MergedAt,
		Commits: []ReviewCommit{}, BotComments: []ReviewComment{}, Threads: []ReviewThread{},
	}
	for _, n := range pr.Commits.Nodes {
		s.Commits = append(s.Commits, n.Commit)
	}
	for _, c := range pr.Comments.Nodes {
		if c.Author.Typename == "Bot" {
			s.BotComments = append(s.BotComments, toComment(c))
		}
	}
	for _, n := range pr.ReviewThreads.Nodes {
		th := ReviewThread{Path: n.Path, Line: n.Line, IsResolved: n.IsResolved, Comments: []ReviewComment{}}
		for _, c := range n.Comments.Nodes {
			th.Comments = append(th.Comments, toComment(c))
		}
		s.Threads = append(s.Threads, th)
	}
	return s, nil
}

// splitPRURL splits https://github.com/<owner>/<repo>/pull/<number>.
func splitPRURL(raw string) (owner, name, number string, ok bool) {
	u, err := url.Parse(raw)
	// Only github.com: a snapshot is fetched with gh, and a look-alike host must never be asked.
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != "github.com" || u.User != nil {
		return "", "", "", false
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(p) != 4 || p[0] == "" || p[1] == "" || p[2] != "pull" {
		return "", "", "", false
	}
	if n, err := strconv.Atoi(p[3]); err != nil || n <= 0 {
		return "", "", "", false
	}
	return p[0], p[1], p[3], true
}

// ReviewFileName is the snapshot file name of a PR url: ".../o/r/pull/9" → "o__r__9.json".
// A GitHub owner has no "_", so the first "__" splits owner from repo and the name cannot collide
// across look-alike repos; readers still check the snapshot's URL.
func ReviewFileName(prURL string) (string, bool) {
	owner, name, number, ok := splitPRURL(prURL)
	if !ok {
		return "", false
	}
	return owner + "__" + name + "__" + number + ".json", true
}

// OwnerRun is the id of the last end event whose PR list holds url, or "-".
func OwnerRun(events []Event, prURL string) string {
	id := "-"
	for _, e := range events {
		if e.Type == "end" && len(e.F) > 0 && slices.Contains(strings.Split(e.F[0], ","), prURL) {
			id = e.ID
		}
	}
	return id
}
