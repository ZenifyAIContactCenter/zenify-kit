// Package api is the HTTP contract of zenify-broker: request/response bodies
// and error codes of every endpoint. The kit client imports it, so fields and
// JSON tags may only be added to, never renamed or removed.
package api

import "github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"

// Error codes returned in Error.Error.
const (
	CodeGitHubTokenInvalid = "github_token_invalid" //nolint:gosec // G101 -- an error code name, not a credential
	CodeNotOrgMember       = "not_org_member"
	CodeRateLimited        = "rate_limited"
	CodeUnauthorized       = "unauthorized"
	CodeInvalidQuery       = "invalid_query"
	CodeInvalidTarget      = "invalid_target"
	CodeTooManyConcurrent  = "too_many_concurrent"
	CodeTimeout            = "timeout"
	// CodeQueryFailed (502): the guards passed but mongosh/mysql failed; the
	// message is the child's masked stderr.
	CodeQueryFailed       = "query_failed"
	CodeCommandNotAllowed = "command_not_allowed"
	CodeUnknownRole       = "unknown_role"
	CodeProvisionFailed   = "provision_failed"
	// CodeGitHubUnavailable (502): GitHub could not be asked, so no
	// membership verdict exists.
	CodeGitHubUnavailable = "github_unavailable"
	// CodeUpstreamUnavailable (502): an /mcp/* upstream could not be reached.
	CodeUpstreamUnavailable = "upstream_unavailable"
	// CodeBodyTooLarge (413): the request body is over the endpoint's cap.
	CodeBodyTooLarge = "body_too_large"
	// CodeInvalidBody (400): the request body could not be read.
	CodeInvalidBody = "invalid_body"
	// CodeBrokerMisconfigured (500): the broker itself is wired wrong (a
	// signing key too short, a credential missing for the asked target).
	CodeBrokerMisconfigured = "broker_misconfigured"
	// CodeLeadOnly (403): the caller's role is dev and the request needs lead.
	CodeLeadOnly = "lead_only"
	// CodeNotAllowed (403): an org member in neither the lead nor the dev team.
	CodeNotAllowed = "not_allowed"
	// CodeLoginDenied (403): the login is in the broker's deny_logins.
	CodeLoginDenied = "login_denied"
	// CodeCatalogUnavailable (503): the catalog was never fetched and GitHub
	// cannot be reached (or serves an unusable file).
	CodeCatalogUnavailable = "catalog_unavailable"
)

// Roles a broker JWT carries.
const (
	RoleLead = "lead"
	RoleDev  = "dev"
)

// Error is the body of every non-2xx response.
type Error struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// TokenRequest is the body of POST /token.
type TokenRequest struct {
	GitHubToken string `json:"github_token"`
}

// TokenResponse is the 200 body of POST /token. ExpiresAt is RFC3339.
type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Login     string `json:"login"`
	Role      string `json:"role"` // RoleLead or RoleDev
}

// QueryRequest is the body of POST /query. Env is stg|prd, Engine mongo|mysql,
// Target main|audit (empty = main), Op one of collections|doc|count|find|
// aggregate|explain|indexes|stats|sql|tables. Mongo fields mirror dbread.MongoQuery.
type QueryRequest struct {
	Env        string `json:"env"`
	Engine     string `json:"engine"`
	Target     string `json:"target"`
	Op         string `json:"op"`
	Coll       string `json:"coll"`
	Filter     string `json:"filter"`
	Projection string `json:"projection"`
	Sort       string `json:"sort"`
	Pipeline   string `json:"pipeline"`
	Limit      int    `json:"limit"`
	SQL        string `json:"sql"`
}

// QueryResponse is the 200 body of POST /query.
type QueryResponse struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
	Rows      int    `json:"rows"`
}

// RedisRequest is the body of POST /redis.
type RedisRequest struct {
	Env  string   `json:"env"`
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

// RedisResponse is the 200 body of POST /redis.
type RedisResponse struct {
	Result    any  `json:"result"`
	Truncated bool `json:"truncated"`
}

// E2EAccountRequest is the body of POST /e2e/account.
type E2EAccountRequest struct {
	Role string `json:"role"`
}

// E2EAccountResponse is the 200 body of POST /e2e/account.
type E2EAccountResponse struct {
	Domain   string `json:"domain"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// E2ERoles are the roles POST /e2e/account accepts: be's UserRole minus
// end_user and chatbot.
var E2ERoles = []string{"administrator", "supervisor", "team_leader", "agent", "sale", "sale_team_leader"}

// Modes POST /creds accepts.
const (
	CredsModeRead  = "read"
	CredsModeWrite = "write"
)

// CredsRequest is the body of POST /creds. Env is stg|prd, Mode read|write;
// Lumi opts in to the lumi-agent credentials.
type CredsRequest struct {
	Env  string `json:"env"`
	Mode string `json:"mode"`
	Lumi bool   `json:"lumi,omitempty"`
}

// CredsResponse is the 200 body of POST /creds: fixed variable name -> value.
// An engine with no secret is absent.
type CredsResponse struct {
	Values map[string]string `json:"values"`
}

// CatalogResponse is the 200 body of POST /catalog. StaleAgeSeconds is 0 for a
// fresh catalog and the age of the last good fetch when GitHub could not be
// reached or served an unusable file.
type CatalogResponse struct {
	Catalog         *catalog.Catalog `json:"catalog"`
	StaleAgeSeconds int              `json:"stale_age_seconds"`
}

// DriftRequest is the body of POST /catalog/drift. Env "" means prd and stg.
type DriftRequest struct {
	Env string `json:"env"`
}

// DriftResponse is the 200 body of POST /catalog/drift: env -> source (loki,
// tempo, prometheus) -> result.
type DriftResponse struct {
	Envs map[string]map[string]DriftResult `json:"envs"`
}

// DriftResult compares one source's live label values over its drift window
// with the catalog. Unlisted: live, in neither the catalog nor ignore.
// Missing: in the catalog, not live. Both sorted. Error is set when the source
// could not be read; the lists are then empty.
type DriftResult struct {
	Unlisted []string `json:"unlisted"`
	Missing  []string `json:"missing"`
	Error    string   `json:"error,omitempty"`
}
