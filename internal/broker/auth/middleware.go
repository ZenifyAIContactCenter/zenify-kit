package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
)

type (
	ctxKey  struct{}
	roleKey struct{}
)

// Require lets a request through only with a valid "Authorization: Bearer
// <broker JWT>" and puts its login and role on the context; otherwise 401 unauthorized.
func Require(key []byte, next http.Handler) http.Handler {
	return RequireAudited(key, nil, nil, next)
}

// RequireAudited is Require that also writes one audit line for a refusal
// (FR-6.1): endpoint(r) names the route, login is empty. a nil skips it.
func RequireAudited(key []byte, a *audit.Logger, endpoint func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, tok, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		login, role, err := Verify(key, strings.TrimSpace(tok), time.Now())
		if !ok || !strings.EqualFold(scheme, "Bearer") || err != nil {
			WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "missing or invalid broker token")
			if a != nil {
				a.Log(audit.Entry{TS: time.Now(), Endpoint: endpoint(r), Op: r.Method, Status: http.StatusUnauthorized})
			}
			return
		}
		ctx := context.WithValue(context.WithValue(r.Context(), ctxKey{}, login), roleKey{}, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Login returns the login Require attached, or "" outside a protected handler.
func Login(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}

// Role returns the role Require attached, or "" outside a protected handler.
func Role(ctx context.Context) string {
	s, _ := ctx.Value(roleKey{}).(string)
	return s
}

// WriteError writes the contract's {"error","message"} body with status.
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.Error{Error: code, Message: msg})
}
