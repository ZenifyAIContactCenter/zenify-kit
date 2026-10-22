// Package server is zenify-broker's HTTP router.
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

// Deps wires the server. Later endpoints add their own fields.
type Deps struct {
	JWTKey []byte
	Org    string // GitHub org whose active members get a token
	// LeadTeam / DevTeam are the org team slugs granting role lead / dev; an
	// org member in neither is refused. DenyLogins are refused outright
	// (case-insensitive).
	LeadTeam, DevTeam string
	DenyLogins        []string
	// AgentIDs are the GitHub databaseIds whose installation token (ghs_)
	// gets role agent; empty = no installation token is accepted.
	AgentIDs     []int64
	GitHub       auth.GitHub
	Audit        *audit.Logger    // nil = stdout
	Now          func() time.Time // nil = time.Now
	Query        http.Handler     // POST /query; nil = route not served
	Redis        http.Handler     // POST /redis; nil = route not served
	E2E          http.Handler     // POST /e2e/account; nil = route not served
	Creds        http.Handler     // POST /creds; nil = route not served
	Catalog      http.Handler     // POST /catalog; nil = route not served
	CatalogDrift http.Handler     // POST /catalog/drift; nil = route not served
	ObsRoute     http.Handler     // POST /obs/route; nil = route not served
	ObsTrace     http.Handler     // POST /obs/trace; nil = route not served
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
	if cfg.Creds != nil {
		mux.Handle("POST /creds", require(fixed("/creds"), cfg.Creds))
	}
	if cfg.Catalog != nil {
		mux.Handle("POST /catalog", require(fixed("/catalog"), cfg.Catalog))
	}
	if cfg.CatalogDrift != nil {
		mux.Handle("POST /catalog/drift", require(fixed("/catalog/drift"), cfg.CatalogDrift))
	}
	if cfg.ObsRoute != nil {
		mux.Handle("POST /obs/route", require(fixed("/obs/route"), cfg.ObsRoute))
	}
	if cfg.ObsTrace != nil {
		mux.Handle("POST /obs/trace", require(fixed("/obs/trace"), cfg.ObsTrace))
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
				cfg.Audit.Log(audit.Entry{TS: cfg.Now(), Login: auth.Login(r.Context()), Role: auth.Role(r.Context()),
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
	login, role, status := "", "", http.StatusOK
	defer func() {
		s.d.Audit.Log(audit.Entry{TS: start, Login: login, Role: role, Endpoint: "/token",
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
	var err error
	if strings.HasPrefix(req.GitHubToken, "ghs_") {
		login, role, err = s.agent(r, req.GitHubToken)
	} else {
		var l string
		if l, err = s.d.GitHub.Member(r.Context(), req.GitHubToken, s.d.Org); err == nil {
			login = l
			role, err = s.role(r, req.GitHubToken, l)
		}
	}
	switch {
	case errors.Is(err, errLoginDenied):
		fail(http.StatusForbidden, api.CodeLoginDenied, "login "+login+" is not allowed to use the broker")
		return
	case errors.Is(err, errNotAgent):
		fail(http.StatusForbidden, api.CodeNotAllowed, "GitHub App installation is not in agent_ids")
		return
	case errors.Is(err, errNoTeam):
		fail(http.StatusForbidden, api.CodeNotAllowed, "not an active member of team "+s.d.LeadTeam+" or "+s.d.DevTeam)
		return
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
	tok, exp, err := auth.Issue(s.d.JWTKey, login, role, s.d.Now())
	if err != nil {
		fail(http.StatusInternalServerError, api.CodeBrokerMisconfigured, "the broker's signing key is misconfigured")
		return
	}
	writeJSON(w, http.StatusOK, api.TokenResponse{Token: tok, ExpiresAt: exp.UTC().Format(time.RFC3339), Login: login, Role: role})
}

var (
	errLoginDenied = errors.New("login denied")
	errNoTeam      = errors.New("in neither team")
	errNotAgent    = errors.New("installation not in agent_ids")
)

// agent identifies an installation token's bot via GraphQL (GitHub refuses it
// on /user): deny_logins first, then the bot's databaseId must be in agent_ids.
// login is returned on errLoginDenied too, for the error message and audit.
func (s *server) agent(r *http.Request, ghToken string) (string, string, error) {
	login, id, err := s.d.GitHub.Viewer(r.Context(), ghToken)
	if err != nil {
		return "", "", err
	}
	if s.denied(login) {
		return login, "", errLoginDenied
	}
	if !slices.Contains(s.d.AgentIDs, id) {
		return login, "", errNotAgent
	}
	return login, api.RoleAgent, nil
}

func (s *server) denied(login string) bool {
	for _, d := range s.d.DenyLogins {
		if strings.EqualFold(d, login) {
			return true
		}
	}
	return false
}

// role decides an org member's role (FR-14.1): deny_logins first, then the
// lead team, then the dev team, each asked with the caller's own token.
func (s *server) role(r *http.Request, ghToken, login string) (string, error) {
	if s.denied(login) {
		return "", errLoginDenied
	}
	for _, t := range []struct{ team, role string }{{s.d.LeadTeam, api.RoleLead}, {s.d.DevTeam, api.RoleDev}} {
		ok, err := s.d.GitHub.TeamMember(r.Context(), ghToken, s.d.Org, t.team, login)
		if err != nil {
			return "", err
		}
		if ok {
			return t.role, nil
		}
	}
	return "", errNoTeam
}
