// Package api is the HTTP contract of zenify-broker: request/response bodies
// and error codes of every endpoint. The kit client imports it, so fields and
// JSON tags may only be added to, never renamed or removed.
package api

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
	CodeCommandNotAllowed  = "command_not_allowed"
	CodeUnknownRole        = "unknown_role"
	CodeProvisionFailed    = "provision_failed"
	// CodeGitHubUnavailable (502): GitHub could not be asked, so no
	// membership verdict exists.
	CodeGitHubUnavailable = "github_unavailable"
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
}

// QueryRequest is the body of POST /query. Env is stg|prd, Engine mongo|mysql,
// Target main|audit (empty = main), Op one of collections|doc|count|find|
// aggregate|explain|sql|tables. Mongo fields mirror dbread.MongoQuery.
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
