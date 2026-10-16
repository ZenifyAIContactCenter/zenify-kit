package api

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// keys marshals v and returns its top-level JSON keys, sorted.
func keys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestContract_JSONKeys pins every wire field name to the spec's Contract section.
func TestContract_JSONKeys(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want []string
	}{
		{"TokenRequest", TokenRequest{}, []string{"github_token"}},
		{"TokenResponse", TokenResponse{}, []string{"expires_at", "login", "role", "token"}},
		{"Error", Error{}, []string{"error", "message"}},
		{"QueryRequest", QueryRequest{}, []string{"coll", "engine", "env", "filter", "limit", "op", "pipeline", "projection", "sort", "sql", "target"}},
		{"QueryResponse", QueryResponse{}, []string{"rows", "stderr", "stdout", "truncated"}},
		{"RedisRequest", RedisRequest{}, []string{"args", "cmd", "env"}},
		{"RedisResponse", RedisResponse{}, []string{"result", "truncated"}},
		{"E2EAccountRequest", E2EAccountRequest{}, []string{"role"}},
		{"E2EAccountResponse", E2EAccountResponse{}, []string{"domain", "email", "password", "role"}},
	}
	for _, c := range cases {
		if got := keys(t, c.v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s keys = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestContract_Codes(t *testing.T) {
	got := []string{CodeGitHubTokenInvalid, CodeNotOrgMember, CodeRateLimited, CodeUnauthorized,
		CodeInvalidQuery, CodeInvalidTarget, CodeTooManyConcurrent, CodeTimeout,
		CodeCommandNotAllowed, CodeUnknownRole, CodeProvisionFailed, CodeGitHubUnavailable}
	want := []string{"github_token_invalid", "not_org_member", "rate_limited", "unauthorized",
		"invalid_query", "invalid_target", "too_many_concurrent", "timeout",
		"command_not_allowed", "unknown_role", "provision_failed", "github_unavailable"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("codes = %v, want %v", got, want)
	}
	roles := []string{"administrator", "supervisor", "team_leader", "agent", "sale", "sale_team_leader"}
	if !reflect.DeepEqual(E2ERoles, roles) {
		t.Errorf("E2ERoles = %v, want %v", E2ERoles, roles)
	}
}
