package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultGitHubURL is the only host a GitHub token is ever sent to (FR-1.4).
const DefaultGitHubURL = "https://api.github.com"

// Membership verdicts. Any other error means GitHub could not be asked.
var (
	ErrTokenInvalid = errors.New("github token invalid")
	ErrNotMember    = errors.New("not an active org member")
	// ErrOrgAccessDenied: the membership endpoint answered 403, i.e. the token
	// is not SSO-authorized for the org or lacks read:org.
	ErrOrgAccessDenied = errors.New("github token cannot read org membership")
	// ErrGitHubRateLimited: GitHub answered 429, or a 403 carrying Retry-After
	// or X-RateLimit-Remaining: 0 (its primary and secondary rate limits).
	ErrGitHubRateLimited = errors.New("github rate limited")
)

// GitHub checks org membership. Empty BaseURL means DefaultGitHubURL; nil HTTP
// means a client with a 10s timeout. The token is used per call, never kept.
type GitHub struct {
	BaseURL string
	HTTP    *http.Client
}

var defaultGitHubClient = &http.Client{Timeout: 10 * time.Second}

// Member returns the token owner's login when they are an active member of org.
func (g GitHub) Member(ctx context.Context, token, org string) (login string, err error) {
	var user struct {
		Login string `json:"login"`
	}
	switch code, err := g.get(ctx, token, "/user", &user); {
	case err != nil:
		return "", err
	case code == http.StatusUnauthorized:
		return "", ErrTokenInvalid
	case code == http.StatusTooManyRequests:
		return "", ErrGitHubRateLimited
	case code != http.StatusOK || user.Login == "":
		return "", fmt.Errorf("github /user: status %d", code)
	}

	var m struct {
		State string `json:"state"`
	}
	switch code, err := g.get(ctx, token, "/user/memberships/orgs/"+url.PathEscape(org), &m); {
	case err != nil:
		return "", err
	case code == http.StatusUnauthorized:
		return "", ErrTokenInvalid
	case code == http.StatusNotFound:
		return "", ErrNotMember
	case code == http.StatusTooManyRequests:
		return "", ErrGitHubRateLimited
	case code == http.StatusForbidden:
		return "", ErrOrgAccessDenied
	case code != http.StatusOK:
		return "", fmt.Errorf("github membership: status %d", code)
	case m.State != "active":
		return "", ErrNotMember
	}
	return user.Login, nil
}

// TeamMember reports whether login is an active member of org's team, asked
// with the caller's own token. 404 and a pending membership both mean no.
// 401 and 429 map as in Member; a 403 is a plain error, not
// ErrOrgAccessDenied: the caller is already a confirmed org member, so it
// must not be reported as one who is not.
func (g GitHub) TeamMember(ctx context.Context, token, org, team, login string) (bool, error) {
	var m struct {
		State string `json:"state"`
	}
	path := "/orgs/" + url.PathEscape(org) + "/teams/" + url.PathEscape(team) + "/memberships/" + url.PathEscape(login)
	switch code, err := g.get(ctx, token, path, &m); {
	case err != nil:
		return false, err
	case code == http.StatusUnauthorized:
		return false, ErrTokenInvalid
	case code == http.StatusNotFound:
		return false, nil
	case code == http.StatusTooManyRequests:
		return false, ErrGitHubRateLimited
	case code != http.StatusOK:
		return false, fmt.Errorf("github team membership: status %d", code)
	}
	return m.State == "active", nil
}

// get calls GitHub and decodes a 200 body into out. A 403 carrying Retry-After
// or X-RateLimit-Remaining: 0 is a rate limit, not a permission refusal, and
// comes back as 429. Errors never include the token.
func (g GitHub) get(ctx context.Context, token, path string, out any) (int, error) {
	base := g.BaseURL
	if base == "" {
		base = DefaultGitHubURL
	}
	client := g.HTTP
	if client == nil {
		client = defaultGitHubClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("github %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		if resp.StatusCode == http.StatusForbidden &&
			(resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0") {
			return http.StatusTooManyRequests, nil
		}
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return 0, fmt.Errorf("github %s: decode: %w", path, err)
	}
	return resp.StatusCode, nil
}
