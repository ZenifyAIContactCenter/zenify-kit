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

// Client talks to one broker. Zero Home/HTTP/Now/GHToken fall back to
// os.UserHomeDir, a 2-minute http.Client, time.Now and GHTokenExec.
type Client struct {
	URL     string
	Home    string
	GHToken func() (string, error)
	HTTP    *http.Client
	Now     func() time.Time
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
	return &http.Client{Timeout: 2 * time.Minute}
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
		if json.Unmarshal(b, &ch) == nil && ch.Token != "" {
			if exp, err := time.Parse(time.RFC3339, ch.ExpiresAt); err == nil && exp.Sub(c.now()) >= refreshWindow {
				return ch.Token, nil
			}
		}
	}
	return c.refresh(ctx, p)
}

func (c *Client) refresh(ctx context.Context, cachePath string) (string, error) {
	gh := c.GHToken
	if gh == nil {
		gh = GHTokenExec
	}
	ghTok, err := gh()
	if err != nil {
		return "", err
	}
	var resp api.TokenResponse
	if err := c.do(ctx, "/token", "", api.TokenRequest{GitHubToken: ghTok}, &resp); err != nil {
		return "", err
	}
	b, err := json.Marshal(cached{Token: resp.Token, ExpiresAt: resp.ExpiresAt, Login: resp.Login})
	if err != nil {
		return "", err
	}
	if err := writeAtomic(cachePath, b); err != nil {
		return "", fmt.Errorf("cache broker token: %w", err)
	}
	return resp.Token, nil
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
		if tok, err = c.refresh(ctx, p); err != nil {
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
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		ae := &APIError{Status: resp.StatusCode}
		var e api.Error
		if json.Unmarshal(b, &e) == nil {
			ae.Code, ae.Message = e.Error, e.Message
		}
		return ae
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

// GHTokenExec returns the developer's GitHub token from `gh auth token`.
func GHTokenExec() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output() //nolint:gosec // fixed argv
	if err != nil {
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
