// Package grafanaproxy reads from Grafana's datasource proxy with a
// service-account token. Errors name the status or the cause only, never the
// token or the upstream body.
package grafanaproxy

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

const timeout = 30 * time.Second

var (
	// ErrNotFound is returned when Grafana answers 404.
	ErrNotFound = errors.New("grafana returned 404")
	// ErrTooLarge is returned when the response body exceeds maxBytes.
	ErrTooLarge = errors.New("grafana response too large")
)

// Client calls the datasource proxy of one Grafana instance.
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// New returns a Client for baseURL authenticated with token.
func New(baseURL, token string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: &http.Client{Timeout: timeout}}
}

// Get calls GET <baseURL>/api/datasources/proxy/uid/<uid><path>?<q> and decodes
// the JSON body into out (skipped when out is nil). A body longer than maxBytes
// yields ErrTooLarge.
func (c *Client) Get(ctx context.Context, uid, path string, q url.Values, maxBytes int64, out any) error {
	u := c.baseURL + "/api/datasources/proxy/uid/" + url.PathEscape(uid) + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return errors.New("grafana request could not be built")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.New("grafana unreachable")
	}
	defer func() {
		// Drain a bounded tail so the keep-alive connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grafana returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return errors.New("grafana response could not be decoded")
	}
	if int64(len(body)) > maxBytes {
		return ErrTooLarge
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errors.New("grafana response could not be decoded")
	}
	return nil
}
