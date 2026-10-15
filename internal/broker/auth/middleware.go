package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
)

type ctxKey struct{}

// Require lets a request through only with a valid "Authorization: Bearer
// <broker JWT>" and puts its login on the context; otherwise 401 unauthorized.
func Require(key []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		login, err := Verify(key, strings.TrimSpace(tok), time.Now())
		if !ok || err != nil {
			WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "missing or invalid broker token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, login)))
	})
}

// Login returns the login Require attached, or "" outside a protected handler.
func Login(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}

// WriteError writes the contract's {"error","message"} body with status.
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.Error{Error: code, Message: msg})
}
