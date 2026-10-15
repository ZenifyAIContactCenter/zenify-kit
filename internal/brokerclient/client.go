// Package brokerclient is the kit side of zenify-broker: it trades the
// developer's GitHub token for a short-lived broker JWT, caches it, and POSTs
// authenticated requests. Wire types live in internal/broker/api.
package brokerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// refreshWindow: a cached JWT with less than this left is replaced.
const refreshWindow = 60 * time.Second

// maxBody caps how much of a broker response is read.
const maxBody = 16 << 20

// accessHint explains a reply that did not come from the broker itself: a
// redirect, a 401/403 or a 2xx that is not JSON (the Cloudflare Access login
// page).
const accessHint = "reply is not from the broker (Cloudflare Access login page?) — turn on Cloudflare WARP and retry"

// Client talks to one broker. Zero Home/HTTP/Now/GHToken/Stderr fall back to
// os.UserHomeDir, NewHTTPClient(2 * time.Minute), time.Now, GHTokenExec and
// os.Stderr.
type Client struct {
	URL     string
	Home    string
	GHToken func(ctx context.Context) (string, error)
	HTTP    *http.Client
	Now     func() time.Time
	Stderr  io.Writer
}

// NewHTTPClient returns a client that never follows a redirect: a 307/308
// would replay the GitHub token body to the Location host.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// APIError is a non-2xx broker reply.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("broker %d %s: %s", e.Status, e.Code, e.Message)
}

type cached struct {
	URL       string `json:"url"` // the broker that issued Token; another URL is a cache miss
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Login     string `json:"login"`
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return NewHTTPClient(2 * time.Minute)
}

func (c *Client) cachePath() (string, error) {
	home := c.Home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = h
	}
	return filepath.Join(home, ".zenify", "broker", "token.json"), nil
}

// Token returns a JWT valid for at least 60 more seconds, from the cache when
// possible, otherwise from POST /token with the developer's GitHub token.
func (c *Client) Token(ctx context.Context) (string, error) {
	p, err := c.cachePath()
	if err != nil {
		return "", err
	}
	if b, err := os.ReadFile(p); err == nil { //nolint:gosec // G304 -- p is under the user's own ~/.zenify
		var ch cached
		if json.Unmarshal(b, &ch) == nil && ch.Token != "" && ch.URL == c.URL {
			if exp, err := time.Parse(time.RFC3339, ch.ExpiresAt); err == nil && exp.Sub(c.now()) >= refreshWindow {
				return ch.Token, nil
			}
		}
	}
	tok, _, err := c.refresh(ctx, p)
	return tok, err
}

// Refresh ignores the cache, trades the GitHub token for a new JWT, caches it
// and returns it with the login the broker issued it to. Use it when a cached
// JWT may be one the broker now refuses (left the org) or to re-check
// membership.
func (c *Client) Refresh(ctx context.Context) (token, login string, err error) {
	p, err := c.cachePath()
	if err != nil {
		return "", "", err
	}
	return c.refresh(ctx, p)
}

func (c *Client) refresh(ctx context.Context, cachePath string) (string, string, error) {
	gh := c.GHToken
	if gh == nil {
		gh = GHTokenExec
	}
	ghTok, err := gh(ctx)
	if err != nil {
		return "", "", err
	}
	var resp api.TokenResponse
	if err := c.do(ctx, "/token", "", api.TokenRequest{GitHubToken: ghTok}, &resp); err != nil {
		return "", "", err
	}
	if resp.Token == "" {
		return "", "", errors.New("broker /token: empty token")
	}
	if _, err := time.Parse(time.RFC3339, resp.ExpiresAt); err != nil {
		return "", "", fmt.Errorf("broker /token: bad expires_at %q", resp.ExpiresAt)
	}
	b, err := json.Marshal(cached{URL: c.URL, Token: resp.Token, ExpiresAt: resp.ExpiresAt, Login: resp.Login})
	if err != nil {
		return "", "", err
	}
	// Best effort: the Claude Code Bash sandbox denies writes under ~/.zenify.
	if err := writeAtomic(cachePath, b); err != nil {
		w := c.Stderr
		if w == nil {
			w = os.Stderr
		}
		_, _ = fmt.Fprintf(w, "warning: broker token not cached: %v\n", err)
	}
	return resp.Token, resp.Login, nil
}

// writeAtomic writes data to path via a 0600 temp file in the same directory.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".token-*") // CreateTemp creates mode 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Post sends in as JSON to path with a bearer token and decodes the 2xx body
// into out (nil = discard). On 401 it fetches a fresh token and retries once.
func (c *Client) Post(ctx context.Context, path string, in, out any) error {
	tok, err := c.Token(ctx)
	if err != nil {
		return err
	}
	err = c.do(ctx, path, tok, in, out)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
		p, perr := c.cachePath()
		if perr != nil {
			return perr
		}
		if tok, _, err = c.refresh(ctx, p); err != nil {
			return err
		}
		return c.do(ctx, path, tok, in, out)
	}
	return err
}

func (c *Client) do(ctx context.Context, path, bearer string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.httpClient().Do(req) //nolint:gosec // G704 -- URL comes from the team manifest, https-validated
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(b) > maxBody {
		return fmt.Errorf("broker %s: reply over %d bytes", path, maxBody)
	}
	if resp.StatusCode/100 == 3 {
		return &APIError{Status: resp.StatusCode, Code: "redirect", Message: accessHint}
	}
	if resp.StatusCode/100 != 2 {
		ae := &APIError{Status: resp.StatusCode}
		var e api.Error
		switch {
		case json.Unmarshal(b, &e) == nil && e.Error != "":
			ae.Code, ae.Message = e.Error, e.Message
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			ae.Code, ae.Message = "not_json", accessHint
		default:
			ae.Code, ae.Message = "not_json", fmt.Sprintf("broker lỗi HTTP %d", resp.StatusCode) //znf:allow-lang
		}
		return ae
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return &APIError{Status: resp.StatusCode, Code: "not_json", Message: accessHint}
	}
	return nil
}

// GHTokenExec returns the developer's GitHub token from `gh auth token`,
// killed when ctx ends.
func GHTokenExec(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "auth", "token") //nolint:gosec // fixed argv
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
				return "", fmt.Errorf("gh auth token: %w: %s", err, msg)
			}
		}
		return "", fmt.Errorf("gh auth token: %w", err)
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", fmt.Errorf("gh auth token: empty output")
	}
	return tok, nil
}

// FromManifest builds a Client when the manifest declares a broker.
func FromManifest(m *manifest.Manifest) (*Client, bool) {
	if m == nil || m.Broker == nil {
		return nil, false
	}
	return &Client{URL: m.Broker.URL}, true
}
