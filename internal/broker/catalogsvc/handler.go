// Package catalogsvc serves POST /catalog: the service catalog, fetched from a
// GitHub repo with the broker's own token and cached in memory.
package catalogsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/config"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
)

const (
	defaultTTL     = 5 * time.Minute
	defaultBaseURL = "https://api.github.com"
	fetchTimeout   = 10 * time.Second
	maxFileBytes   = 1 << 20
	// retryInterval: after a failed fetch with a stale copy in hand, GitHub is
	// not asked again for this long; callers get the stale copy at once.
	retryInterval = 30 * time.Second
)

// Handler is the POST /catalog handler. It must sit behind auth.RequireAudited.
type Handler struct {
	cfg     config.Catalog
	token   string
	baseURL string
	ttl     time.Duration
	now     func() time.Time
	client  *http.Client
	audit   *audit.Logger // nil = no audit rows
	slots   int           // NewDrift only: concurrent drift requests per login

	mu          sync.Mutex // held across fetch+cache so concurrent callers share one fetch
	cat         *catalog.Catalog
	fetchedAt   time.Time
	lastFailure time.Time // zero = the last fetch succeeded (or none failed yet)
}

// Option tunes a Handler.
type Option func(*Handler)

// WithBaseURL points the GitHub client elsewhere (tests).
func WithBaseURL(u string) Option { return func(h *Handler) { h.baseURL = u } }

// WithClock replaces time.Now (tests).
func WithClock(now func() time.Time) Option { return func(h *Handler) { h.now = now } }

// WithAudit makes ServeHTTP write one audit row per request.
func WithAudit(l *audit.Logger) Option { return func(h *Handler) { h.audit = l } }

// WithSlots caps how many /catalog/drift requests one login runs at once
// (default 1); it applies to NewDrift only.
func WithSlots(n int) Option { return func(h *Handler) { h.slots = n } }

// WithTTL overrides the 5-minute cache lifetime.
func WithTTL(d time.Duration) Option { return func(h *Handler) { h.ttl = d } }

// New builds a Handler for the catalog file cfg names, read with token.
func New(cfg config.Catalog, token string, opts ...Option) *Handler {
	h := &Handler{cfg: cfg, token: token, baseURL: defaultBaseURL, ttl: defaultTTL, now: time.Now,
		client: &http.Client{Timeout: fetchTimeout}}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Current returns the catalog and the age in seconds of the copy served (0 =
// fresh). When GitHub cannot be asked or serves an unusable file, the last
// good copy is returned with its age; with none, the error. After a failure
// with a stale copy, GitHub is not asked again for retryInterval.
func (h *Handler) Current(ctx context.Context) (*catalog.Catalog, int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if h.cat != nil && now.Sub(h.fetchedAt) < h.ttl {
		return h.cat, 0, nil
	}
	stale := func() (*catalog.Catalog, int, error) {
		return h.cat, int(now.Sub(h.fetchedAt) / time.Second), nil
	}
	if h.cat != nil && !h.lastFailure.IsZero() && now.Sub(h.lastFailure) < retryInterval {
		return stale()
	}
	c, err := h.fetch(ctx)
	if err != nil {
		h.lastFailure = now
		if h.cat == nil {
			return nil, 0, err
		}
		return stale()
	}
	h.cat, h.fetchedAt, h.lastFailure = c, now, time.Time{}
	return c, 0, nil
}

// fetch never puts the token in an error: it names the status or the cause only.
func (h *Handler) fetch(ctx context.Context) (*catalog.Catalog, error) {
	u := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", h.baseURL, h.cfg.Repo, h.cfg.Path, url.QueryEscape(h.cfg.Ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, errors.New("catalog request could not be built")
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, errors.New("catalog fetch failed: GitHub unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog fetch failed: GitHub returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes+1))
	if err != nil {
		return nil, errors.New("catalog fetch failed: reading the body")
	}
	if len(b) > maxFileBytes {
		return nil, fmt.Errorf("catalog fetch failed: file is over %d bytes", maxFileBytes)
	}
	return catalog.Parse(b)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.now()
	status := http.StatusOK
	defer func() {
		if h.audit != nil {
			h.audit.Log(audit.Entry{TS: start, Login: auth.Login(r.Context()), Role: auth.Role(r.Context()),
				Endpoint: "/catalog", Op: r.Method, Status: status})
		}
	}()
	c, age, err := h.Current(r.Context())
	if err != nil {
		status = http.StatusServiceUnavailable
		auth.WriteError(w, status, api.CodeCatalogUnavailable, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.CatalogResponse{Catalog: c, StaleAgeSeconds: age})
}
