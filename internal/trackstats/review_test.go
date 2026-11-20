package trackstats

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const reviewFixture = `{"data":{"repository":{"pullRequest":{
  "createdAt":"2026-11-18T01:00:00Z","mergedAt":"2026-11-18T05:00:00Z",
  "commits":{"nodes":[
    {"commit":{"oid":"aaa111","committedDate":"2026-11-18T01:30:00Z"}},
    {"commit":{"oid":"bbb222","committedDate":"2026-11-18T03:00:00Z"}}]},
  "comments":{"nodes":[
    {"author":{"__typename":"Bot","login":"zenie-reviewer"},"createdAt":"2026-11-18T02:00:00Z","body":"Review round 1"},
    {"author":{"__typename":"User","login":"dev"},"createdAt":"2026-11-18T02:10:00Z","body":"thanks"}]},
  "reviewThreads":{"nodes":[
    {"isResolved":true,"path":"internal/cli/track.go","line":42,"comments":{"nodes":[
      {"author":{"__typename":"Bot","login":"zenie-reviewer"},"createdAt":"2026-11-18T02:01:00Z","body":"nil check"}]}}]}
}}}}`

func TestParseReview(t *testing.T) {
	s, err := parseReview([]byte(reviewFixture))
	if err != nil {
		t.Fatal(err)
	}
	if s.CreatedAt != "2026-11-18T01:00:00Z" || s.MergedAt != "2026-11-18T05:00:00Z" {
		t.Errorf("times = %q %q", s.CreatedAt, s.MergedAt)
	}
	if len(s.Commits) != 2 || s.Commits[1].OID != "bbb222" || s.Commits[1].CommittedDate != "2026-11-18T03:00:00Z" {
		t.Errorf("commits = %+v", s.Commits)
	}
	if len(s.BotComments) != 1 || s.BotComments[0].Author != "zenie-reviewer" || s.BotComments[0].Body != "Review round 1" {
		t.Errorf("bot comments = %+v, want only the Bot one", s.BotComments)
	}
	if len(s.Threads) != 1 {
		t.Fatalf("threads = %+v", s.Threads)
	}
	th := s.Threads[0]
	if th.Path != "internal/cli/track.go" || th.Line != 42 || !th.IsResolved || len(th.Comments) != 1 || th.Comments[0].Body != "nil check" {
		t.Errorf("thread = %+v", th)
	}
	if th.Comments[0].AuthorType != "Bot" || s.BotComments[0].AuthorType != "Bot" {
		t.Errorf("authorType thread %q bot comment %q, want Bot", th.Comments[0].AuthorType, s.BotComments[0].AuthorType)
	}
	if s.TrackID != "" || s.FetchedAt != "" {
		t.Errorf("TrackID %q FetchedAt %q, want both empty for the CLI to fill", s.TrackID, s.FetchedAt)
	}
}

func TestParseReview_EmptyListsAreNotNull(t *testing.T) {
	s, err := parseReview([]byte(`{"data":{"repository":{"pullRequest":{"createdAt":"2026-11-18T01:00:00Z","mergedAt":"2026-11-18T05:00:00Z",` +
		`"commits":{"nodes":[]},"comments":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"commits":[]`, `"botComments":[]`, `"threads":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("snapshot %s lacks %s", b, want)
		}
	}
}

func TestParseReview_NoPullRequest(t *testing.T) {
	if _, err := parseReview([]byte(`{"data":{"repository":{"pullRequest":null}}}`)); err == nil {
		t.Fatal("want an error when the PR is missing")
	}
}

func TestReviewFileName(t *testing.T) {
	if got, ok := ReviewFileName("https://github.com/o/r/pull/9"); !ok || got != "o__r__9.json" {
		t.Errorf("ReviewFileName = %q %v, want o__r__9.json true", got, ok)
	}
	if got, ok := ReviewFileName("http://github.com/o/r/pull/9"); !ok || got != "o__r__9.json" {
		t.Errorf("ReviewFileName(http) = %q %v, want o__r__9.json true", got, ok)
	}
	for _, u := range []string{"https://x/y", "branch:x", "https://gitlab.com/o/r/pull/9", "https://github.com.evil.io/o/r/pull/9",
		"https://ghe.corp/o/r/pull/9", "https://user@github.com:8443/o/r/pull/9", "https://github.com/o/r/issues/9", "https://github.com/o/r/pull/x"} {
		if got, ok := ReviewFileName(u); ok {
			t.Errorf("ReviewFileName(%q) = %q true, want false", u, got)
		}
	}
}

// Names cannot collide across look-alike repos: a-b/c and a/b-c differ.
func TestReviewFileName_NoCollision(t *testing.T) {
	x, _ := ReviewFileName("https://github.com/a-b/c/pull/1")
	y, _ := ReviewFileName("https://github.com/a/b-c/pull/1")
	if x == y {
		t.Fatalf("both name %q", x)
	}
}

func TestOwnerRun_Review(t *testing.T) {
	a, b := "cook-261118-0001", "fix-261118-0002"
	ev, _ := ParseLog(lines(
		row(cAt(18, 1), "end", a, "https://a/pull/1,https://a/pull/2", "1", "1"),
		row(cAt(18, 2), "end", b, "https://a/pull/3", "0", "1"),
		row(cAt(18, 3), "end", b, "https://a/pull/1", "0", "1"),
	))
	if got := OwnerRun(ev, "https://a/pull/1"); got != b {
		t.Errorf("OwnerRun(pull/1) = %q, want the last end %q", got, b)
	}
	if got := OwnerRun(ev, "https://a/pull/2"); got != a {
		t.Errorf("OwnerRun(pull/2) = %q, want %q", got, a)
	}
	if got := OwnerRun(ev, "https://a/pull/3"); got != b {
		t.Errorf("OwnerRun(pull/3) = %q, want %q", got, b)
	}
	if got := OwnerRun(ev, "https://a/pull/9"); got != "-" {
		t.Errorf("OwnerRun(unknown) = %q, want -", got)
	}
}

// fakeGh puts a gh on PATH that prints stderr and exits 1.
func fakeGh(t *testing.T, stderr string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '" + stderr + "' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil { //nolint:gosec // G306 -- test helper must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// gh's stderr alone decides a transport failure; the url and per-PR GraphQL errors never do.
func TestGhReview_TransportFromStderr(t *testing.T) {
	cases := []struct {
		url, stderr string
		transport   bool
	}{
		{"https://github.com/o/r/pull/9", "could not resolve host: api.github.com", true},
		{"https://github.com/o/r/pull/9", "HTTP 401: Bad credentials (https://api.github.com/graphql)", true},
		{"https://github.com/o/r/pull/9", "API rate limit exceeded for user", true},
		{"https://github.com/o/r/pull/9", "GraphQL: Could not resolve to a PullRequest with the number of 9. (repository.pullRequest)", false},
		{"https://github.com/o/auth-service/pull/1", "GraphQL: Something went wrong", false},
	}
	for _, c := range cases {
		fakeGh(t, c.stderr)
		_, err := GhReview(c.url)
		if err == nil {
			t.Fatalf("%q: want an error", c.stderr)
		}
		var te *GhTransportError
		if got := errors.As(err, &te); got != c.transport {
			t.Errorf("%s %q: transport = %v, want %v (%v)", c.url, c.stderr, got, c.transport, err)
		}
	}
}

// A timed-out gh call is a transport failure that still wraps ErrGhTimeout.
func TestGhReviewErr_Timeout(t *testing.T) {
	err := ghReviewErr("https://github.com/o/r/pull/9", true, nil, "")
	var te *GhTransportError
	if !errors.As(err, &te) || !errors.Is(err, ErrGhTimeout) {
		t.Fatalf("err = %v, want a GhTransportError wrapping ErrGhTimeout", err)
	}
}
