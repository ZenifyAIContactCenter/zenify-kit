// Package server is zenify-broker's HTTP router.
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

// Deps wires the server. Later endpoints add their own fields.
type Deps struct {
	JWTKey []byte
	Org    string // GitHub org whose active members get a token
	GitHub auth.GitHub
	Audit  *audit.Logger    // nil = stdout
	Now    func() time.Time // nil = time.Now
	Query  http.Handler     // POST /query; nil = route not served
	Redis  http.Handler     // POST /redis; nil = route not served
	E2E    http.Handler     // POST /e2e/account; nil = route not served
	// MCP maps a name to its proxy, served on /mcp/<name> (any method, any
	// subpath); an unknown name is 404. Empty = no /mcp routes.
	MCP map[string]http.Handler
}

// /token limit per client address (FR-1.3).
const (
	tokenBurst  = 30
	tokenWindow = time.Minute
)

// maxTokenBody caps the POST /token body.
const maxTokenBody = 64 << 10

type server struct {
	d       Deps
	tokenRL *limiter
}

// New returns the broker's handler.
func New(cfg Deps) http.Handler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Audit == nil {
		cfg.Audit = &audit.Logger{W: os.Stdout}
	}
	s := &server{d: cfg, tokenRL: newLimiter(tokenBurst, tokenWindow)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /token", s.token)
	// require audits a 401 under endpoint (FR-6.1).
	require := func(endpoint func(*http.Request) string, h http.Handler) http.Handler {
		return auth.RequireAudited(cfg.JWTKey, cfg.Audit, endpoint, h)
	}
	fixed := func(e string) func(*http.Request) string { return func(*http.Request) string { return e } }
	if cfg.Query != nil {
		mux.Handle("POST /query", require(fixed("/query"), cfg.Query))
	}
	if cfg.Redis != nil {
		mux.Handle("POST /redis", require(fixed("/redis"), cfg.Redis))
	}
	if cfg.E2E != nil {
		mux.Handle("POST /e2e/account", require(fixed("/e2e/account"), cfg.E2E))
	}
	if len(cfg.MCP) > 0 {
		// The name is caller-chosen; only a configured one reaches the audit log.
		mcpEndpoint := func(r *http.Request) string {
			if name := r.PathValue("name"); cfg.MCP[name] != nil {
				return "/mcp/" + name
			}
			return "/mcp/?"
		}
		mcp := require(mcpEndpoint, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h, ok := cfg.MCP[r.PathValue("name")]
			if !ok {
				http.NotFound(w, r)
				cfg.Audit.Log(audit.Entry{TS: cfg.Now(), Login: auth.Login(r.Context()),
					Endpoint: "/mcp/" + r.PathValue("name"), Op: r.Method, Status: http.StatusNotFound})
				return
			}
			h.ServeHTTP(w, r)
		}))
		mux.Handle("/mcp/{name}", mcp)
		mux.Handle("/mcp/{name}/{rest...}", mcp)
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// token exchanges a GitHub token for a broker JWT. The GitHub token goes only
// to d.GitHub and is never stored or logged; neither is the issued JWT.
func (s *server) token(w http.ResponseWriter, r *http.Request) {
	start := s.d.Now()
	login, status := "", http.StatusOK
	defer func() {
		s.d.Audit.Log(audit.Entry{TS: start, Login: login, Endpoint: "/token",
			MS: s.d.Now().Sub(start).Milliseconds(), Status: status})
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}

	if !s.tokenRL.allow(clientIP(r), start) {
		fail(http.StatusTooManyRequests, api.CodeRateLimited, "too many /token requests, retry in a minute")
		return
	}
	var req api.TokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTokenBody)).Decode(&req); err != nil || req.GitHubToken == "" {
		fail(http.StatusUnauthorized, api.CodeGitHubTokenInvalid, "body must be {\"github_token\":\"...\"}")
		return
	}
	l, err := s.d.GitHub.Member(r.Context(), req.GitHubToken, s.d.Org)
	switch {
	case errors.Is(err, auth.ErrTokenInvalid):
		fail(http.StatusUnauthorized, api.CodeGitHubTokenInvalid, "GitHub rejected the token")
		return
	case errors.Is(err, auth.ErrNotMember):
		fail(http.StatusForbidden, api.CodeNotOrgMember, "not an active member of "+s.d.Org)
		return
	case errors.Is(err, auth.ErrOrgAccessDenied):
		fail(http.StatusForbidden, api.CodeNotOrgMember, "GitHub refused to show membership of "+s.d.Org+
			": authorize the token for the org's SSO and grant it the read:org scope")
		return
	case errors.Is(err, auth.ErrGitHubRateLimited):
		fail(http.StatusTooManyRequests, api.CodeRateLimited, "GitHub rate-limited the token check, retry later")
		return
	case err != nil:
		fail(http.StatusBadGateway, api.CodeGitHubUnavailable, "could not reach GitHub")
		return
	}
	login = l
	tok, exp := auth.Issue(s.d.JWTKey, login, s.d.Now())
	writeJSON(w, http.StatusOK, api.TokenResponse{Token: tok, ExpiresAt: exp.UTC().Format(time.RFC3339), Login: login})
}
